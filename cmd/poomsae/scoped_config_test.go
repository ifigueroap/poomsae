package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const untrustedWebConfig = "[web]\npublic_origin = \"http://daemon.example\"\n"

func TestDaemonStartRejectsInvalidServerConfig(t *testing.T) {
	home := setupKataEnv(t)
	t.Setenv("POOMSAE_TRUST_PRIVATE_NETWORK", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(untrustedWebConfig), 0600))
	_, err := preflightDaemonStartup(t.Context(), "", false, false)
	require.ErrorContains(t, err, "web.public_origin")
}
