package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsignedCommandValidation(t *testing.T) {
	thisTool := filepath.Base(os.Args[0])

	for _, tc := range []struct {
		Name     string
		Command  string
		Expected bool
	}{
		{
			"Normal buildkite upload",
			"buildkite-agent pipeline upload",
			true,
		},
		{
			"Normal buildkite upload with file argument",
			"buildkite-agent pipeline upload .buildkite/pipeline.yml",
			true,
		},
		{
			"Normal buildkite upload with quoted file argument",
			`buildkite-agent pipeline upload ".buildkite/pipeline.yml"`,
			false,
		},
		{
			"Simple signed upload",
			fmt.Sprintf("%s upload", thisTool),
			true,
		},
		{
			"Upload with special shell characters",
			fmt.Sprintf("%s upload `rm -rf /`", thisTool),
			false,
		},
		{
			"Upload with shell variable",
			fmt.Sprintf(`%s upload "$PWD"`, thisTool),
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
