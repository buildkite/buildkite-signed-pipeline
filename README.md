# buildkite-signed-pipeline

This is a tool that adds some extra security guarantees around Buildkite's jobs. Buildkite [security best practices](https://buildkite.com/docs/agent/v3/securing) suggest using `--no-command-eval` which will only allow local scripts in a checked out repository to be run, preventing arbitrary commands being injected by an intermediary.

The downside of that approach is that it also comes with the recommendation of disabling plugins, or allow listing specifically what plugins and parameters are allowed. This tool is a collaboration between SEEK and Buildkite that attempts to bridge this gap and allow uploaded steps to be signed with a secret shared by all agents, so that plugins can run without any concerns of tampering by third-parties.

## Example

### Uploading a pipeline with signatures

Upload is a thin wrapper around [`buildkite-agent pipeline upload`](https://buildkite.com/docs/agent/v3/cli-pipeline#uploading-pipelines) that adds the required signatures. It behaves much like the command it wraps.

```bash
export SIGNED_PIPELINE_SECRET='my secret'

buildkite-signed-pipeline upload
```

### Verifying a pipeline signature

In a global `environment` hook, you can include the following to ensure that all jobs that are handed to an agent contain the correct signatures:

```bash
# Allow the upload command to be unsigned, as it typically comes from the Buildkite UI and not your agents
if [[ "${BUILDKITE_COMMAND}" == "buildkite-signed-pipeline upload" ]]; then
  echo "Allowing pipeline upload"
  exit 0
fi

export SIGNED_PIPELINE_SECRET='my secret'

if ! buildkite-signed-pipeline verify ; then
  echo "Step verification failed"
  exit 1
fi
```

This step will fail if the provided signatures aren't in the environment.

## How it works

When the tool receives a pipeline for upload, it follows these steps:

* Iterates through each step of a JSON pipeline
* Extracts the `command` or `commands` block
* Trims whitespace on resulting command
* Calculates `HMAC(SHA256, command, shared-secret)`
* Add `STEP_SIGNATURE={hash}` to the step `environment` block
* Pipes the modified JSON pipeline to `buildkite-agent pipeline upload`

When the tool is verifying a pipeline:

* Calculates `HMAC(SHA256, BUILDKITE_COMMAND, shared-secret)`
* Compare result with `STEP_SIGNATURE`
* Fail if they don't match

## Development

This is using Golang's 1.11 modules.

```
export GO111MODULE=on
go run .
```
