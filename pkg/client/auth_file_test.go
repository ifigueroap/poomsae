package client_test

import (
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/pkg/client"
	"github.com/stretchr/testify/require"
)

// Selected auth-file failures stop every SDK transport before a request.
func TestNewWithGlobalAuthUnixRejectsBrokenSelectedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DB", filepath.Join(home, "poomsae.db"))
	t.Setenv("POOMSAE_AUTH_TOKEN", "")
	t.Setenv("POOMSAE_AUTH_TOKEN_FILE", filepath.Join(home, "missing-token"))
	_, err := client.NewWithGlobalAuth(t.Context(), "unix:///tmp/example-daemon.sock")
	require.Error(t, err)
}
