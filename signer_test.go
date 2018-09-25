package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSigningCommand(t *testing.T) {
	jsonPipeline := `{"steps":[{"command":"echo Hello \"Fred\""}]}`

	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonPipeline), &parsed); err != nil {
		t.Fatal(err)
	}

	signer := NewSharedSecretSigner("secret-llamas")

	signed, err := signer.Sign(parsed)
	if err != nil {
		t.Fatal(err)
	}

	j, err := json.Marshal(signed)
	assert.Equal(t, `{"steps":[{"command":"echo Hello \"Fred\"","env":{"STEP_SIGNATURE":"a3ea512c6a88aa490d50879ef7ad7e3bc27c6f286435a9660fb662960e63592c"}}]}`, string(j))
}

func TestSigningMultipleCommands(t *testing.T) {
	jsonPipeline := `{"steps":[{"command":["echo Hello World", "echo Foo Bar"]}]}`

	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonPipeline), &parsed); err != nil {
		t.Fatal(err)
	}

	signer := NewSharedSecretSigner("secret-llamas")

	signed, err := signer.Sign(parsed)
	if err != nil {
		t.Fatal(err)
	}

	j, err := json.Marshal(signed)
	assert.Equal(t, `{"steps":[{"command":["echo Hello World","echo Foo Bar"],"env":{"STEP_SIGNATURE":"3a2ce177522b03ff8146aff9b26c0e552728619d496e1e0870532c8d5257a42b"}}]}`, string(j))
}

func TestSigningCommandWithEnv(t *testing.T) {
	jsonPipeline := `{"steps":[{"command":"echo Hello \"Fred\"", "env":{"EXISTING": "existing-value"}}]}`

	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonPipeline), &parsed); err != nil {
		t.Fatal(err)
	}

	signer := NewSharedSecretSigner("secret-llamas")

	signed, err := signer.Sign(parsed)
	if err != nil {
		t.Fatal(err)
	}

	j, err := json.Marshal(signed)
	assert.Equal(t, `{"steps":[{"command":"echo Hello \"Fred\"","env":{"EXISTING":"existing-value","STEP_SIGNATURE":"a3ea512c6a88aa490d50879ef7ad7e3bc27c6f286435a9660fb662960e63592c"}}]}`, string(j))
}
