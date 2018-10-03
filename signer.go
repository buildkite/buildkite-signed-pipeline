package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"log"
	"fmt"
	"reflect"
	"strings"
	"path/filepath"
	"os"
)

const (
	stepSignatureEnv = `STEP_SIGNATURE`
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
	case []interface{}:
		for _, item := range t {
			for name, settings := range item.(map[string]interface{}) {
				parsed = append(parsed, Plugin{name, settings.(map[string]interface{})})
			}
		}
	case map[string]interface{}:
		for name, settings := range t {
			parsed = append(parsed, Plugin{name, settings.(map[string]interface{})})
		}
	default:
		return "", fmt.Errorf("Unknown plugin type %T", t)
	}

	return marshalPlugins(parsed)
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
	h.Write([]byte(pluginJSON))
	return Signature(fmt.Sprintf("sha256:%x", h.Sum(nil))), nil
}

func allowCommand(command string) (bool, error) {
	fileName := strings.Replace(command, "\n", "", -1)
	isLocalFile, err := isWorkingDirectoryFile(fileName) 
	if err != nil {
		return false, err
	}
	return isLocalFile, nil
}

func (s SharedSecretSigner) Verify(command string, pluginJSON string, allowedUnsignedCommands []string, expected Signature) error {
	// Allow any command on the unsigned list to be verified provided it has no signature and plugins.
	// Checking there's no signature is important to ensure plugins aren't being injected with a
	// command on the built-in allow list
	if contains(allowedUnsignedCommands, command) {
		log.Println("Command is on the allowed list of unsigned commands")
		if expected == "" && pluginJSON == "" {
			log.Println("✅ Allowing allow-listed command as no signature is specified, and no plugins are referenced")
			return nil
		}
		log.Println("❗️Forcing signature check as pstep has an expected signature or referenced plugins")
	}

	forceAllow, err := allowCommand(command)
	if err != nil {
		return err
	}
	if forceAllow {
		log.Println("Allowing command without signature")
		return nil
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

func contains(hayStack []string, needle string) bool {
	for _, item := range hayStack {
		if item == needle {
			return true
		}
	}
	return false
}

func isWorkingDirectoryFile(fileName string) (bool, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return false, err
	}
	pathToFile, err := filepath.Abs(filepath.Join(workingDirectory, fileName))
	return err == nil && fileExists(pathToFile), nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
