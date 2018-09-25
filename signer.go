package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
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
			return copy, nil
		}
	}

	commandSignature, err := s.signCommand(rawCommand)
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

	env[stepSignatureEnv] = commandSignature
	copy["env"] = env

	return copy, nil
}

func (s SharedSecretSigner) signCommand(command interface{}) (string, error) {
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

	h := hmac.New(sha256.New, []byte(s.secret))
	h.Write([]byte(strings.TrimSpace(strings.Join(commandStrings, ""))))

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
