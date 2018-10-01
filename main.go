package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	Version string = "0.2.0"
)

func main() {
	app := kingpin.New("buildkite-signed-pipeline", "Signed pipeline uploads for Buildkite")
	app.Version(Version)

	sharedSecret := app.
		Flag("shared-secret", "A shared secret to use for signing").
		OverrideDefaultFromEnvar(`SIGNED_PIPELINE_SECRET`).
		String()

	awsSmSecretId := app.
		Flag("aws-sm-shared-secret-id", "AWS Secrets Manager (AWS SM) secret id (friendly name, or ARN) of a secret to use for signing").
		OverrideDefaultFromEnvar(`SIGNED_PIPELINE_AWS_SM_SECRET_ID`).
		String()

	uploadCommand := app.Command("upload", "Upload a pipeline.yml with signatures")
	uploadFile := uploadCommand.Arg("file", "The pipeline.yml to process").File()
	uploadDryRun := uploadCommand.Flag("dry-run", "Just show the pipeline that will be uploaded").Bool()

	verifyCommand := app.Command("verify", "Verify a job contains a signature")

	context := kingpin.MustParse(app.Parse(os.Args[1:]))
	if *sharedSecret == "" && *awsSmSecretId == "" {
		app.FatalUsage("Either --shared-secret or --aws-sm-shared-secret-id must be specified")
	}
	signer := NewSharedSecretSigner(*sharedSecret)
	switch context {
	case uploadCommand.FullCommand():
		upload(signer, *uploadFile, *uploadDryRun)
	case verifyCommand.FullCommand():
		verify(signer)
	}
}

func upload(signer *SharedSecretSigner, file *os.File, dryRun bool) error {
	// Exec `buildkite-agent pipeline upload <file> --dry-run`
	// Sign output
	// Exec `buildkite-agent pipeline upload with stdin`

	parsed, err := getPipelineFromBuildkiteAgent(file)
	if err != nil {
		log.Fatal(err)
	}

	signed, err := signer.Sign(parsed)
	if err != nil {
		log.Fatal(err)
	}

	outputJSON, err := json.Marshal(signed)
	if err != nil {
		log.Fatal(err)
	}

	// interpolation is disabled to avoid expanding variables twice
	uploadArgs := []string{"pipeline", "upload", "--no-interpolation"}

	if dryRun {
		uploadArgs = append(uploadArgs, "--dry-run")
	}

	cmd := exec.Command("buildkite-agent", uploadArgs...)
	cmd.Stdin = bytes.NewReader(outputJSON)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	return nil
}

func verify(signer *SharedSecretSigner) error {
	command := os.Getenv(`BUILDKITE_COMMAND`)
	pluginJSON := os.Getenv(`BUILDKITE_PLUGINS`)
	sig := os.Getenv(stepSignatureEnv)

	if command == "" && pluginJSON == "" {
		log.Println("No command or plugins set")
		return nil
	}

	err := signer.Verify(command, pluginJSON, Signature(sig))

	if err != nil {
		log.Fatalln(err)
	}

	log.Println("Signature matched")

	return nil
}

func getPipelineFromBuildkiteAgent(f *os.File) (interface{}, error) {
	args := []string{"pipeline", "upload", "--dry-run"}

	// handle an optional path to a pipeline.yml
	if f != nil {
		args = append(args, f.Name())
	}

	log.Printf("$ buildkite-agent %s", strings.Join(args, " "))

	// Run buildkite-agent the first time to get
	cmd := exec.Command("buildkite-agent", args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var parsed interface{}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return nil, err
	}

	return parsed, nil
}
