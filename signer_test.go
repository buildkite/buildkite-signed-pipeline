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

func TestSigningCommandWithPlugins(t *testing.T) {
	var pipeline = map[string]interface{}{
		"steps": []interface{}{
			map[string]interface{}{
				"command": "my command",
				"plugins": map[string]interface{}{
					"my-plugin": map[string]interface{}{
						"my-setting": true,
					},
				},
			},
		},
	}

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Equal(t, "my command", command)
		return Signature("llamas"), nil
	}

	signed, err := signer.Sign(pipeline)
	if err != nil {
		t.Fatal(err)
	}

	var result struct{
		Steps []struct{
			Env map[string]string
		}
	}

	if err := mapInto(&result, signed); err != nil {
		t.Fatal(err)
	}

	sig, ok := result.Steps[0].Env["STEP_SIGNATURE"]
	if !ok {
		t.Fatal("No STEP_SIGNATURE env present")
	}

	assert.Equal(t, "llamas", sig)
}

func mapInto(dest interface{}, source interface{}) error {
	jsonBytes, err := json.Marshal(source)
	if err != nil {
		return err
	}

	return json.Unmarshal(jsonBytes, dest)
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
			"Pipeline with top level env",
			`{"env":{"GLOBAL_ENV":"wow"},"steps":[{"command":"echo Hello \"Fred\""}]}`,
			`{"env":{"GLOBAL_ENV":"wow"},"steps":[{"command":"echo Hello \"Fred\"","env":{"STEP_SIGNATURE":"a3ea512c6a88aa490d50879ef7ad7e3bc27c6f286435a9660fb662960e63592c"}}]}`,
		},
		{
			"Command with existing env",
			`{"steps":[{"command":"echo Hello \"Fred\"", "env":{"EXISTING": "existing-value"}}]}`,
			`{"steps":[{"command":"echo Hello \"Fred\"","env":{"EXISTING":"existing-value","STEP_SIGNATURE":"a3ea512c6a88aa490d50879ef7ad7e3bc27c6f286435a9660fb662960e63592c"}}]}`,
		},
		{
			"Pipeline with multiple commands",
			`{"steps":[{"command":["echo Hello World", "echo Foo Bar"]}]}`,
			`{"steps":[{"command":["echo Hello World","echo Foo Bar"],"env":{"STEP_SIGNATURE":"3a2ce177522b03ff8146aff9b26c0e552728619d496e1e0870532c8d5257a42b"}}]}`,
		},
		{
			"Step with no command",
			`{"steps":[{"label":"I have no commands"}]}`,
			`{"steps":[{"label":"I have no commands"}]}`,
		},
		{
			"Step plugins, but no commands",
			`{"steps":[{"label":"I have no commands","plugins":[{"docker#v1.4.0":{"image":"node:7"}}]}]}`,
			`{"steps":[{"env":{"STEP_SIGNATURE":"acee5ed57eea7fb6388e3349677f1ec85ce55e131c7ba56b7093218f5be24a6b"},"label":"I have no commands","plugins":[{"docker#v1.4.0":{"image":"node:7"}}]}]}`,
		},
		{
			"Pipeline with multiple steps",
			`{"steps":[{"command":"echo hello"},{"commands":["echo world", "echo foo"]}]}`,
			`{"steps":[{"command":"echo hello","env":{"STEP_SIGNATURE":"bc6d93682b086f836db67c98551c95079e6cd0b64f59abc590b5e076956759e0"}},{"commands":["echo world","echo foo"],"env":{"STEP_SIGNATURE":"b5a1828030d5bb9577b9d29ace3f0f5a2c1ede4e9d357cc30296565da9636eba"}}]}`,
		},
		{
			"Empty command",
			`{"steps":[{"command":""}]}`,
			`{"steps":[{"command":""}]}`,
		},
		{
			"Wait step",
			`{"steps":["wait"]}`,
			`{"steps":["wait"]}`,
		},
		{
			"Block step",
			`{"steps":[{"block":"Does this work?","prompt":"Yes"}]}`,
			`{"steps":[{"block":"Does this work?","prompt":"Yes"}]}`,
		},
		{
			"Wait with steps",
			`{"steps":[{"block":"Does this work?","prompt":"Yes"},"wait",{"command":"echo done"}]}`,
			`{"steps":[{"block":"Does this work?","prompt":"Yes"},"wait",{"command":"echo done","env":{"STEP_SIGNATURE":"7314596562367a9a0fe297ea47d32416d9039b064e14f39aed84170bdc4c6574"}}]}`,
		},
		{
			"Step with plugins",
			"{\"steps\":[{\"command\":\"echo Hello World\",\"plugins\":[{\"github.com/seek-oss/snyk-buildkite-plugin#v0.0.4\":{\"path\":\"package.json\",\"block\":true}},{\"github.com/seek-oss/aws-sm-buildkite-plugin#v0.0.5\":{\"env\":{\"XX\":\"name\"}}}]}]}",
			"{\"steps\":[{\"command\":\"echo Hello World\",\"env\":{\"STEP_SIGNATURE\":\"c6b6e6344b52b2ce57bd13ba05e0b63ef48364dda962c300bb210cd5be3898ef\"},\"plugins\":[{\"github.com/seek-oss/snyk-buildkite-plugin#v0.0.4\":{\"path\":\"package.json\",\"block\":true}},{\"github.com/seek-oss/aws-sm-buildkite-plugin#v0.0.5\":{\"env\":{\"XX\":\"name\"}}}]}]}",
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
