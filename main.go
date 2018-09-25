package main

import (
	"fmt"
	"os"

	"gopkg.in/alecthomas/kingpin.v2"
)

type UploadCommand struct {
}

func (l *UploadCommand) run(c *kingpin.ParseContext) error {
	fmt.Printf("uploading\n")
	return nil
}

func configureUploadCommand(app *kingpin.Application) {
	c := &UploadCommand{}
	app.Command("upload", "Upload a pipeline.yml with signatures").Action(c.run)
	// ls.Flag("all", "List all files.").Short('a').BoolVar(&c.All)
}

type VerifyCommand struct {
}

func (v *VerifyCommand) run(c *kingpin.ParseContext) error {
	fmt.Printf("verifying\n")
	return nil
}

func configureVerifyCommand(app *kingpin.Application) {
	c := &VerifyCommand{}
	app.Command("verify", "Verify a job contains a signature").Action(c.run)
	// ls.Flag("all", "List all files.").Short('a').BoolVar(&c.All)
}

func main() {
	app := kingpin.New("buildkite-signed-pipeline", "Signed pipeline uploads for Buildkite")

	configureUploadCommand(app)
	configureVerifyCommand(app)

	kingpin.MustParse(app.Parse(os.Args[1:]))
}
