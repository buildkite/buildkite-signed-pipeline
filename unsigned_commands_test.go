package main

import (
	"fmt"
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
			false,
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
