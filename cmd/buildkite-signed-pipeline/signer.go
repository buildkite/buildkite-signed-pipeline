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
	stepSignatureEnv  = `STEP_SIGNATURE`
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

func (s SharedSecretSigner) Sign(pipeline interface{}) (interface{}, error) {
	original := reflect.ValueOf(pipeline)

	// only process pipelines that are either a single complex step (not "wait") or a collection of steps
	if original.Kind() != reflect.Map {
		return pipeline, nil
	}

	copy := reflect.MakeMap(original.Type())

	// Copy values to new map
	// TODO handle pipelines of single commands (e.g. `command: foo`)
	for _, mk := range original.MapKeys() {
		keyName := mk.String()
		item := original.MapIndex(mk)

		// references many steps
		if strings.EqualFold(keyName, "steps") {
			unwrapped := item.Elem()
			if unwrapped.Kind() == reflect.Slice {
				var newSteps []interface{}
				for i := 0; i < unwrapped.Len(); i += 1 {
					stepItem := unwrapped.Index(i)
					if stepItem.Elem().Kind() != reflect.String {
						signedStep, err := s.signStep(stepItem)
						if err != nil {
							return nil, err
						}
						newSteps = append(newSteps, signedStep)
					} else {
						newSteps = append(newSteps, stepItem.Interface())
					}
				}
				item = reflect.ValueOf(newSteps)
			}
		}
		copy.SetMapIndex(mk, item)
	}

	return copy.Interface(), nil
}

func (s SharedSecretSigner) signStep(step reflect.Value) (interface{}, error) {
	original := step.Elem()

	// Check to make sure the interface isn't nil
	if !original.IsValid() {
		return nil, errors.New("Nil interface provided")
	}

	// Create a new object
	copy := make(map[string]interface{})
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

	env := make(map[string]interface{})
	existingEnv, hasEnv := copy["env"]
	if hasEnv {
		reflectedEnv := reflect.ValueOf(existingEnv)
		for _, key := range reflectedEnv.MapKeys() {
			env[key.String()] = reflectedEnv.MapIndex(key).Interface()
		}
	}

	env[stepSignatureEnv] = signature
	copy["env"] = env

	return copy, nil
}

func (s SharedSecretSigner) extractPlugins(plugins interface{}) (string, error) {
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
	case []interface{}:
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
	case map[string]interface{}:
		for k, v := range t {
			// convert to a single map so it can be treated the same as the array syntax
			plugin, err := NewPluginFromReference(map[string]interface{}{k:v})
			if err != nil {
				return "", err
			}
			parsed = append(parsed, *plugin)
		}
	default:
		return "", fmt.Errorf("Unknown plugin type %T", t)
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

func (s SharedSecretSigner) extractCommand(command interface{}) (string, error) {
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
		return "", fmt.Errorf("Unexpected type for command: %T", command)
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
		return errors.New("🚨 Signature missing. The provided command is not permitted to be unsigned.")
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
