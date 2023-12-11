package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
)

const (
	stepSignatureEnv    = `STEP_SIGNATURE`
	buildkiteBuildIDEnv = `BUILDKITE_BUILD_ID`
)

func NewSharedSecretSigner(secret string) *SharedSecretSigner {
	return &SharedSecretSigner{
		secret: secret,
	}
}

type SharedSecretSigner struct {
	secret string
	// Allow the signature function to be overriden in tests
	signerFunc func(string, string) (Signature, error)
	// Allow the unsigned command validation to be overriden in tests
	unsignedCommandValidatorFunc func(string) (bool, error)
}

func (s SharedSecretSigner) Sign(pipeline any) (any, error) {
	original := reflect.ValueOf(pipeline)

	// only process pipelines that are either a single complex step (not "wait") or a collection of steps
	if original.Kind() != reflect.Map {
		return pipeline, nil
	}

	copy := reflect.MakeMap(original.Type())

	// TODO handle pipelines of single commands (e.g. `command: foo`)
	// Iterate over the top level map (where keys are things like, steps, agents, env)
	for _, mk := range original.MapKeys() {
		keyName := mk.String()
		item := original.MapIndex(mk)

		elem, err := s.maybeSignElements(keyName, item)
		if err != nil {
			return nil, fmt.Errorf("signing pipeline element %s: %w", keyName, err)
		}

		copy.SetMapIndex(mk, elem)
	}

	return copy.Interface(), nil
}

func (s SharedSecretSigner) maybeSignElements(keyName string, item reflect.Value) (reflect.Value, error) {
	// We only care about "steps" at the top level, so return it unchanged if it's not that
	if !strings.EqualFold(keyName, "steps") {
		return item, nil
	}

	unwrapped := item.Elem()
	if unwrapped.Kind() != reflect.Slice {
		return item, nil
	}

	// newSteps will replace the existing steps. they will be built up with the signature added
	newSteps := make([]any, 0, unwrapped.Len())
	for i := 0; i < unwrapped.Len(); i += 1 {
		stepItem := unwrapped.Index(i)

		if stepItem.Elem().Kind() == reflect.String {
			// The current stepItem is a plain string (like just `wait` or `block`) so add it without modification
			newSteps = append(newSteps, stepItem.Interface())
			continue
		}

		// Otherwise, it's (probably?) a step object, so sign it
		signedStep, err := s.signStep(stepItem)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("signing step: %w", err)
		}

		newSteps = append(newSteps, signedStep)
	}

	return reflect.ValueOf(newSteps), nil
}

func addSignature(env any, signature Signature) (any, error) {
	// if there's no env, default to the map format
	if env == nil {
		env = make(map[string]any)
	}

	switch i := env.(type) {
	case []any: // key=value environment variables
		envCopy := make([]any, len(i))
		copy(envCopy, i)

		envCopy = append(envCopy, fmt.Sprintf("%s=%s", stepSignatureEnv, signature))
		return envCopy, nil

	case map[string]any: // map of environment variables
		envCopy := make(map[string]any)
		reflectedEnv := reflect.ValueOf(i)

		for _, key := range reflectedEnv.MapKeys() {
			envCopy[key.String()] = reflectedEnv.MapIndex(key).Interface()
		}

		envCopy[stepSignatureEnv] = signature
		return envCopy, nil
	}

	return nil, fmt.Errorf("unknown environment type %T", env)
}

func (s SharedSecretSigner) signStep(step reflect.Value) (any, error) {
	original := step.Elem()

	// Check to make sure the interface isn't nil
	if !original.IsValid() {
		return nil, errors.New("nil interface provided")
	}

	// Create a new object
	copy := make(map[string]any)
	for _, key := range original.MapKeys() {
		copy[key.String()] = original.MapIndex(key).Interface()
	}

	rawCommand, hasCommand := copy["command"]
	if !hasCommand {
		// treat commands as an alias of command
		var hasCommands bool
		rawCommand, hasCommands = copy["commands"]
		if !hasCommands {
			// no commands to sign
			rawCommand = ""
		}
	}

	// if the step is a `group` we need to recurse to calculate the signature of nested command steps
	if _, hasGroup := copy["group"]; hasGroup {
		pipeline := make(map[string]any)
		pipeline["steps"] = copy["steps"]
		signedGroup, err := s.Sign(pipeline)
		copy["steps"] = signedGroup.(map[string]any)["steps"]
		return copy, err
	}

	// extract the plugin declaration for signing
	extractedPlugins := ""
	var err error
	if plugins, hasPlugins := copy["plugins"]; hasPlugins {
		extractedPlugins, err = s.extractPlugins(plugins)
		if err != nil {
			return nil, err
		}

		log.Printf("Signing canonicalised plugins %s", extractedPlugins)
	}

	// no plugins or commands -- nothing to do
	if rawCommand == "" && extractedPlugins == "" {
		return copy, nil
	}

	extractedCommand, err := s.extractCommand(rawCommand)
	if err != nil {
		return nil, err
	}

	// allow signerFunc to be overwritten in tests
	signerFunc := s.signerFunc
	if signerFunc == nil {
		signerFunc = s.signData
	}

	signature, err := signerFunc(extractedCommand, extractedPlugins)
	if err != nil {
		return nil, err
	}

	existingEnv := copy["env"]
	if copy["env"], err = addSignature(existingEnv, signature); err != nil {
		return nil, err
	}

	return copy, nil
}

func (s SharedSecretSigner) extractPlugins(plugins any) (string, error) {
	var parsed []Plugin

	switch t := plugins.(type) {
	/*
	 handles array syntax for referencing plugins, e.g.
	 plugins:
	  - foo#v1.2.3
	  - bar#v1.2.3:
	  - another#v1.2.3:
	    a-parameter: true
	*/
	case []any:
		for _, item := range t {
			plugin, err := NewPluginFromReference(item)
			if err != nil {
				return "", err
			}
			parsed = append(parsed, *plugin)
		}
	/*
	 handles object syntax for referencing plugins, e.g.
	 plugins:
	  bar#v1.2.3:
	  another#v1.2.3:
	    a-parameter: true
	*/
	case map[string]any:
		for k, v := range t {
			// convert to a single map so it can be treated the same as the array syntax
			plugin, err := NewPluginFromReference(map[string]any{k: v})
			if err != nil {
				return "", err
			}
			parsed = append(parsed, *plugin)
		}
	default:
		return "", fmt.Errorf("unknown plugin type %T", t)
	}

	pluginJSON, err := marshalPlugins(parsed)
	if err != nil {
		return "", err
	}

	// ensure the same plugin form (ordering, etc) is used as the verify step
	canonicalJSON, err := canonicalisePluginJSON(pluginJSON)
	if err != nil {
		return "", err
	}

	return canonicalJSON, err
}

func (s SharedSecretSigner) extractCommand(command any) (string, error) {
	value := reflect.ValueOf(command)

	// expand into simple list of commands
	var commandStrings []string
	if value.Kind() == reflect.Slice {
		for i := 0; i < value.Len(); i += 1 {
			commandStrings = append(commandStrings, value.Index(i).Elem().String())
		}
	} else if value.Kind() == reflect.String {
		commandStrings = append(commandStrings, value.String())
	} else {
		return "", fmt.Errorf("unexpected type for command: %T", command)
	}

	return strings.Join(commandStrings, "\n"), nil
}

type Signature string

func (s SharedSecretSigner) signData(command string, pluginJSON string) (Signature, error) {
	h := hmac.New(sha256.New, []byte(s.secret))
	h.Write([]byte(strings.TrimSpace(command)))
	h.Write([]byte(os.Getenv(buildkiteBuildIDEnv)))
	h.Write([]byte(pluginJSON))
	return Signature(fmt.Sprintf("sha256:%x", h.Sum(nil))), nil
}

func (s SharedSecretSigner) Verify(command string, pluginJSON string, expected Signature) error {
	// step with just a command (no plugins) isn't signed
	if expected == "" && pluginJSON == "" && command != "" {
		log.Printf("⚠️ Command is unsigned, checking if it's allow-listed")

		// allow a custom validator func to be provided in tests
		validatorFunc := s.unsignedCommandValidatorFunc
		if validatorFunc == nil {
			validatorFunc = IsUnsignedCommandOk
		}

		isAllowed, err := validatorFunc(command)
		if err != nil {
			return err
		}
		if isAllowed {
			log.Printf("Allowing unsigned command")
			return nil
		}
		return errors.New("🚨 Signature missing. The provided command is not permitted to be unsigned")
	}

	if pluginJSON != "" {
		var err error
		pluginJSON, err = canonicalisePluginJSON(pluginJSON)
		if err != nil {
			return err
		}
	}

	// allow signerFunc to be overwritten in tests
	signerFunc := s.signerFunc
	if signerFunc == nil {
		signerFunc = s.signData
	}
	signature, err := signerFunc(command, pluginJSON)

	if err != nil {
		return err
	}

	if signature != expected {
		return errors.New("🚨 Signature mismatch. " +
			"Perhaps check the shared secret is the same across agents?")
	}

	return nil
}
