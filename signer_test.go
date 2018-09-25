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

func TestPipelines(t *testing.T) {
	for _, tc := range []struct {
		Name         string
		PipelineJSON string
		Expected     string
	}{
		{
			"Simple pipeline",
			`{"steps":[{"command":"echo Hello \"Fred\""}]}`,
			`{"steps":[{"command":"echo Hello \"Fred\"","env":{"STEP_SIGNATURE":"a3ea512c6a88aa490d50879ef7ad7e3bc27c6f286435a9660fb662960e63592c"}}]}`,
		},
		{
			"Pipeline with multiple commands",
			`{"steps":[{"command":["echo Hello World", "echo Foo Bar"]}]}`,
			`{"steps":[{"command":["echo Hello World","echo Foo Bar"],"env":{"STEP_SIGNATURE":"3a2ce177522b03ff8146aff9b26c0e552728619d496e1e0870532c8d5257a42b"}}]}`,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			signer := NewSharedSecretSigner("secret-llamas")
			var pipeline interface{}
			err := json.Unmarshal([]byte(tc.PipelineJSON), &pipeline)
			if err != nil {
				t.Fatal(err)
			}
			signed, err := signer.Sign(pipeline)
			if err != nil {
				t.Fatal(err)
			}
			signedJSON, err := json.Marshal(signed)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, string(signedJSON), tc.Expected)
		})
	}
}
