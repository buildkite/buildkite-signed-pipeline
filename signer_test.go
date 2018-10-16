package main

import (
	"encoding/json"
	"fmt"
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
	assert.Equal(t, `{"steps":[{"command":"echo Hello \"Fred\"","env":{"STEP_SIGNATURE":"sha256:a3ea512c6a88aa490d50879ef7ad7e3bc27c6f286435a9660fb662960e63592c"}}]}`, string(j))
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
					"seek-oss/custom-plugin": map[string]interface{} {
						"a-setting": true,
					},
				},
			},
		},
	}

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Equal(t, "my command", command)
		assert.Contains(t, plugins, "github.com/buildkite-plugins/my-plugin-buildkite-plugin")
		assert.Contains(t, plugins, "github.com/seek-oss/custom-plugin-buildkite-plugin")
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
			`{"steps":[{"command":"echo Hello \"Fred\"","env":{"STEP_SIGNATURE":"signature(echo Hello \"Fred\",)"}}]}`,
		},
		{
			"Pipeline with top level env",
			`{"env":{"GLOBAL_ENV":"wow"},"steps":[{"command":"echo Hello \"Fred\""}]}`,
			`{"env":{"GLOBAL_ENV":"wow"},"steps":[{"command":"echo Hello \"Fred\"","env":{"STEP_SIGNATURE":"signature(echo Hello \"Fred\",)"}}]}`,
		},
		{
			"Command with existing env",
			`{"steps":[{"command":"echo Hello \"Fred\"", "env":{"EXISTING": "existing-value"}}]}`,
			`{"steps":[{"command":"echo Hello \"Fred\"","env":{"EXISTING":"existing-value","STEP_SIGNATURE":"signature(echo Hello \"Fred\",)"}}]}`,
		},
		{
			"Pipeline with multiple commands",
			`{"steps":[{"command":["echo Hello World", "echo Foo Bar"]}]}`,
			`{"steps":[{"command":["echo Hello World","echo Foo Bar"],"env":{"STEP_SIGNATURE":"signature(echo Hello World\necho Foo Bar,)"}}]}`,
		},
		{
			"Step with no command",
			`{"steps":[{"label":"I have no commands"}]}`,
			`{"steps":[{"label":"I have no commands"}]}`,
		},
		{
			"Step plugins, but no commands",
			`{"steps":[{"label":"I have no commands","plugins":[{"docker#v1.4.0":{"image":"node:7"}}]}]}`,
			`{"steps":[{"env":{"STEP_SIGNATURE":"signature(,[{\"github.com/buildkite-plugins/docker-buildkite-plugin#v1.4.0\":{\"image\":\"node:7\"}}])"},"label":"I have no commands","plugins":[{"docker#v1.4.0":{"image":"node:7"}}]}]}`,
		},
		{
			"Pipeline with multiple steps",
			`{"steps":[{"command":"echo hello"},{"commands":["echo world", "echo foo"]}]}`,
			`{"steps":[{"command":"echo hello","env":{"STEP_SIGNATURE":"signature(echo hello,)"}},{"commands":["echo world","echo foo"],"env":{"STEP_SIGNATURE":"signature(echo world\necho foo,)"}}]}`,
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
			`{"steps":[{"block":"Does this work?","prompt":"Yes"},"wait",{"command":"echo done","env":{"STEP_SIGNATURE":"signature(echo done,)"}}]}`,
		},
		{
			"Step with plugins",
			`{"steps":[{"command":"echo Hello World","plugins":[{"docker#v0.0.4":{"image":"foo"}}]}]}`,
			`{"steps":[{"command":"echo Hello World","env":{"STEP_SIGNATURE":"signature(echo Hello World,[{\"github.com/buildkite-plugins/docker-buildkite-plugin#v0.0.4\":{\"image\":\"foo\"}}])"},"plugins":[{"docker#v0.0.4":{"image":"foo"}}]}]}`,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			signer := NewSharedSecretSigner("secret-llamas")
			signer.signerFunc = func(command, plugins string) (Signature, error) {
				return Signature(fmt.Sprintf("signature(%s,%s)", command, plugins)), nil
			}
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

func TestVerifyCommand(t *testing.T) {
	const expectedPluginJSON = ""
	const expectedCommand = `echo hello world`
	const expectedSignature = Signature("llamas")

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Equal(t, expectedCommand, command)
		assert.Equal(t, expectedPluginJSON, plugins)
		return expectedSignature, nil
	}

	err := signer.Verify(expectedCommand, expectedPluginJSON, expectedSignature)
	assert.Nil(t, err)
}

func TestVerifyCommandAndPlugins(t *testing.T) {
	const expectedPluginJSON = `[{"github.com/buildkite-plugins/docker-buildkite-plugin#v123":{"image":"node8"}}]`
	const expectedCommand = `echo hello world`
	const expectedSignature = Signature("llamas")

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Equal(t, expectedCommand, command)
		assert.Equal(t, expectedPluginJSON, plugins)
		return expectedSignature, nil
	}

	err := signer.Verify(expectedCommand, expectedPluginJSON, expectedSignature)
	assert.Nil(t, err)
}

func TestVerifyCommandAndPluginsRejectsSignature(t *testing.T) {
	const expectedPluginJSON = `[{"github.com/buildkite-plugins/docker-buildkite-plugin#v123":{"image":"node8"}}]`
	const expectedCommand = `echo hello world`

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Equal(t, expectedCommand, command)
		assert.Equal(t, expectedPluginJSON, plugins)
		return Signature("llamas"), nil
	}

	err := signer.Verify(expectedCommand, expectedPluginJSON, "oh no")
	assert.NotNil(t, err)
}

func TestVerifyPluginsAndNoCommand(t *testing.T) {
	const expectedPluginJSON = `[{"github.com/buildkite-plugins/docker-buildkite-plugin#v123":{"image":"node8"}}]`
	const expectedCommand = ""
	const expectedSignature = Signature("llamas")

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Equal(t, expectedCommand, command)
		assert.Equal(t, expectedPluginJSON, plugins)
		return expectedSignature, nil
	}
	signer.unsignedCommandValidatorFunc = func(command string) (bool, error) {
		assert.Fail(t, "Unsigned command validation should not be called")
		return true, nil
	}

	err := signer.Verify(expectedCommand, expectedPluginJSON, expectedSignature)
	assert.Nil(t, err)
}

func TestVerifyAllowsUnsignedCommand(t *testing.T) {
	const expectedPluginJSON = ""
	const expectedCommand = "buildkite-signed-pipeline upload"

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Fail(t, "Signer should not be called")
		return "", nil
	}
	signer.unsignedCommandValidatorFunc = func(command string) (bool, error) {
		assert.Equal(t, expectedCommand, command)
		return true, nil
	}

	err := signer.Verify(expectedCommand, expectedPluginJSON, "")
	assert.Nil(t, err)
}

func TestVerifyRejectsUnsignedCommand(t *testing.T) {
	const expectedPluginJSON = ""
	const expectedCommand = "something-naughty"

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Fail(t, "Signer should not be called")
		return "", nil
	}
	signer.unsignedCommandValidatorFunc = func(command string) (bool, error) {
		assert.Equal(t, expectedCommand, command)
		return false, nil
	}

	err := signer.Verify(expectedCommand, expectedPluginJSON, "")
	assert.NotNil(t, err)
}

func TestVerifyRejectsUnsignedCommandWithPlugins(t *testing.T) {
	const expectedPluginJSON = `[{"github.com/buildkite-plugins/docker-buildkite-plugin#v123":{"image":"node8"}}]`
	const expectedCommand = "buildkite-signed-pipeline upload"

	signer := NewSharedSecretSigner("secret-llamas")
	signer.signerFunc = func(command, plugins string) (Signature, error) {
		assert.Equal(t, expectedCommand, command)
		assert.Equal(t, expectedPluginJSON, plugins)
		return Signature("not the signature"), nil
	}
	signer.unsignedCommandValidatorFunc = func(command string) (bool, error) {
		assert.Fail(t, "Unsigned command validation should not be called")
		return true, nil
	}

	err := signer.Verify(expectedCommand, expectedPluginJSON, "")
	assert.NotNil(t, err)
}
