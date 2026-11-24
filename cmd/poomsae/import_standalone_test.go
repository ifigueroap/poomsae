package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImportAsStandaloneRejectsReplacementAndMerge(t *testing.T) {
	setupKataEnv(t)
	for _, incompatible := range []string{"--force", "--merge"} {
		t.Run(incompatible, func(t *testing.T) {
			_, err := runCmdOutput(t, nil, "import", "--as-standalone", incompatible,
				"--input", "source.jsonl", "--target", "copy.db")
			ce := requireCLIError(t, err, ExitValidation)
			assert.Contains(t, ce.Message, "--as-standalone cannot be combined with "+incompatible)
		})
	}
}
