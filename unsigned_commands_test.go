package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsignedCommandValidation(t *testing.T) {
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
			"buildkite-signed-pipeline upload",
			true,
		},
		{
			"Upload with existing file argument",
			"buildkite-signed-pipeline upload go.mod",
			true,
		},
		{
			"Upload with not found file argument",
			"buildkite-signed-pipeline upload missing-file.yaml",
			false,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			validator := UnsignedCommandValidator{}
			isAllowed, err := validator.Allowed(tc.Command)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, isAllowed, tc.Expected)
		})
	}
}
