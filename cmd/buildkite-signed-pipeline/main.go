package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	Version string = "1.9.0"
)

func main() {
	app := kingpin.New("buildkite-signed-pipeline", "Signed pipeline uploads for Buildkite")
	app.Version(Version)

	var (
		sharedSecret      string
		awsSharedSecretId string
	)
	app.
		Flag("shared-secret", "A shared secret to use for signing").
		OverrideDefaultFromEnvar(`SIGNED_PIPELINE_SECRET`).
		StringVar(&sharedSecret)

	app.
		Flag("aws-sm-shared-secret-id", "A shared secret to use for signing").
		OverrideDefaultFromEnvar(`SIGNED_PIPELINE_AWS_SM_SECRET_ID`).
		StringVar(&awsSharedSecretId)

	uploadCommand := &uploadCommand{}
	uploadCommandClause := app.Command("upload", "Upload a pipeline.yml with signatures").Action(uploadCommand.run)
	uploadCommandClause.
		Arg("file", "The pipeline.yml to process").
		FileVar(&uploadCommand.File)

	uploadCommandClause.
		Flag("dry-run", "Just show the pipeline that will be uploaded").
		BoolVar(&uploadCommand.DryRun)

	uploadCommandClause.
		Flag("replace", "Replace the rest of the existing pipeline with the steps uploaded.").
		BoolVar(&uploadCommand.Replace)

	verifyCommand := &verifyCommand{}
	app.Command("verify", "Verify a job contains a signature").Action(verifyCommand.run)

	app.PreAction(func(_ *kingpin.ParseContext) error {
		if sharedSecret == "" && awsSharedSecretId == "" {
			return errors.New("One of --shared-secret or --aws-sm-shared-secret-id must be provided")
		}
		return nil
	})

	// This happens after parse, we need to create a signer object for all of our
	// commands.
	app.Action(func(_ *kingpin.ParseContext) error {
		signingSecret := sharedSecret

		if awsSharedSecretId != "" {
			log.Printf("Using secret from AWS SM %s", awsSharedSecretId)
			var err error
			signingSecret, err = GetAwsSmSecret(awsSharedSecretId)
			if err != nil {
				log.Fatal(err)
			}
		}

		uploadCommand.Signer = NewSharedSecretSigner(signingSecret)
		verifyCommand.Signer = NewSharedSecretSigner(signingSecret)
		return nil
	})

	kingpin.MustParse(app.Parse(os.Args[1:]))
}

type uploadCommand struct {
	Signer  *SharedSecretSigner
	File    *os.File
	DryRun  bool
	Replace bool
}

func (l *uploadCommand) run(c *kingpin.ParseContext) error {
	// Exec `buildkite-agent pipeline upload <file> --dry-run`
	// Sign output
	// Exec `buildkite-agent pipeline upload with stdin`

	parsed, err := getPipelineFromBuildkiteAgent(l.File)
	if err != nil {
		log.Fatal(err)
	}

	signed, err := l.Signer.Sign(parsed)
	if err != nil {
		log.Fatal(err)
	}

	outputJSON, err := json.Marshal(signed)
	if err != nil {
		log.Fatal(err)
	}

	// interpolation is disabled to avoid expanding variables twice
	uploadArgs := []string{"pipeline", "upload", "--no-interpolation"}

	if l.DryRun {
		uploadArgs = append(uploadArgs, "--dry-run")
	}

	if l.Replace {
		uploadArgs = append(uploadArgs, "--replace")
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

type verifyCommand struct {
	Signer *SharedSecretSigner
}

func (v *verifyCommand) run(c *kingpin.ParseContext) error {
	command := os.Getenv(`BUILDKITE_COMMAND`)
	pluginJSON := os.Getenv(`BUILDKITE_PLUGINS`)
	sig := os.Getenv(stepSignatureEnv)

	if command == "" && pluginJSON == "" {
		log.Println("No command or plugins set")
		return nil
	}

	err := v.Signer.Verify(command, pluginJSON, Signature(sig))
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
