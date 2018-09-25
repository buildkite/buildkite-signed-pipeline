package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	var sharedSecret string

	app := kingpin.New("buildkite-signed-pipeline", "Signed pipeline uploads for Buildkite")
	app.
		Flag("shared-secret", "A shared secret to use for signing").
		OverrideDefaultFromEnvar(`SIGNED_PIPELINE_SECRET`).
		Required().
		StringVar(&sharedSecret)

	uploadCommand := &uploadCommand{}
	uploadCommandClause := app.Command("upload", "Upload a pipeline.yml with signatures").Action(uploadCommand.run)
	uploadCommandClause.
		Arg("file", "The pipeline.yml to process").
		FileVar(&uploadCommand.File)

	uploadCommandClause.
		Flag("dry-run", "Just show the pipeline that will be uploaded").
		BoolVar(&uploadCommand.DryRun)

	verifyCommand := &verifyCommand{}
	app.Command("verify", "Verify a job contains a signature").Action(verifyCommand.run)

	// This happens after parse, we need to create a signer object for all of our
	// commands.
	app.Action(kingpin.Action(func(c *kingpin.ParseContext) error {
		uploadCommand.Signer = NewSharedSecretSigner(sharedSecret)
		verifyCommand.Signer = NewSharedSecretSigner(sharedSecret)
		return nil
	}))

	kingpin.MustParse(app.Parse(os.Args[1:]))
}

type uploadCommand struct {
	Signer *SharedSecretSigner
	File   *os.File
	DryRun bool
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

	uploadArgs := []string{"pipeline", "upload"}

	if l.DryRun {
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

type verifyCommand struct {
	Signer *SharedSecretSigner
}

func (v *verifyCommand) run(c *kingpin.ParseContext) error {
	fmt.Printf("verifying\n")
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
