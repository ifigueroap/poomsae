package main

import (
	"encoding/json/v2"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ifigueroap/poomsae/internal/mcpdiscovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPStatusJSONReportsPublishedListener(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	cleanup, err := mcpdiscovery.Publish(filepath.Join(home, "mcp"), "127.0.0.1:9876", "", "http://127.0.0.1:4321")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	for _, args := range [][]string{
		{"--json", "mcp", "status"},
		{"mcp", "status", "--json"},
		{"--format=json", "mcp", "status"},
		{"mcp", "status", "--format=json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output := executeRoot(t, newRootCmd(), args...)
			var rows []mcpdiscovery.Endpoint
			require.NoError(t, json.Unmarshal(output, &rows))
			require.Len(t, rows, 1)
			assert.Equal(t, "http://127.0.0.1:9876/mcp", rows[0].URL)
			assert.Equal(t, "http://127.0.0.1:4321", rows[0].BackendURL)
		})
	}
}

func TestMCPStatusRejectsConflictingOutputModes(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	_, _, err := executeRootCapture(t, t.Context(), "mcp", "status", "--json", "--format=human")
	var cli *cliError
	require.ErrorAs(t, err, &cli)
	assert.Equal(t, ExitUsage, cli.ExitCode)
}
