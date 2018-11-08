package main

import (
	"fmt"
	"io/ioutil"
	"testing"
	"os"
	"path/filepath"

	"github.com/stretchr/testify/assert"
)

func TestUnsignedCommandValidation(t *testing.T) {
	thisTool := filepath.Base(os.Args[0])

	for _, tc := range []struct {
		Name         string
		Command      string
		Expected     bool
	}{
		{
			"Normal buildkite upload",
			"buildkite-agent pipeline upload",
			true,
		},
		{
			"Normal buildkite upload with file argument",
			"buildkite-agent pipeline upload go.mod",
			true,
		},
		{
			"Simple signed upload",
			fmt.Sprintf("%s upload", thisTool),
			true,
		},
		{
			"Upload with existing file argument",
			fmt.Sprintf("%s upload go.mod", thisTool),
			true,
		},
		{
			"Upload with relative argument inside working directory",
			fmt.Sprintf("%s upload .git/../go.mod", thisTool),
			true,
		},
		{
			"Upload with not found file argument",
			fmt.Sprintf("%s upload missing-file.yaml", thisTool),
			false,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			isAllowed, err := IsUnsignedCommandOk(tc.Command)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, isAllowed, tc.Expected)
		})
	}
}

func TestUnsignedCommandWithOutsideFile(t *testing.T) {
	thisTool := filepath.Base(os.Args[0])

	workingDirectory, err := os.Getwd()
	assert.Nil(t, err)

	tempFile, err := ioutil.TempFile("", "outside")
	assert.Nil(t, err)
	defer os.Remove(tempFile.Name())

	// create a path relative to the outside file, this will be something like ../../../tmp/foo
	relPath, err := filepath.Rel(workingDirectory, tempFile.Name())

	command := fmt.Sprintf("%s upload %s", thisTool, relPath)
	isAllowed, err := IsUnsignedCommandOk(command)
	assert.Nil(t, err)
	assert.False(t, isAllowed)
}
