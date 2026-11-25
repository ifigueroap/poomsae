package daemonruntime_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/daemonruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockEnv creates a temp dir and points $POOMSAE_HOME and $POOMSAE_DB at it
// for the test's lifetime. Returns the temp dir so callers can construct
// dbhash-derived paths or layer additional env vars on top before calling
// daemonruntime.NewNamespace().
func setupMockEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("POOMSAE_HOME", tmp)
	t.Setenv("POOMSAE_DB", filepath.Join(tmp, "poomsae.db"))
	return tmp
}

func TestNamespace_DataDirIsPoomsaeHomeRuntime(t *testing.T) {
	tmp := setupMockEnv(t)

	ns, err := daemonruntime.NewNamespace()
	require.NoError(t, err)
	hash := config.DBHash(filepath.Join(tmp, "poomsae.db"))
	assert.Equal(t, filepath.Join(tmp, "runtime", hash), ns.DataDir)
	assert.Equal(t, hash, ns.DBHash)
}

func TestNamespace_SocketDirHonorsXDGRuntimeDir(t *testing.T) {
	setupMockEnv(t)
	xdg := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	ns, err := daemonruntime.NewNamespace()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(xdg, "poomsae", ns.DBHash), ns.SocketDir)
}

func TestNamespaceSeparatesPostgresSchemas(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "postgres://user@db.example/poomsae?sslmode=verify-full")
	t.Setenv("POOMSAE_POSTGRES_SCHEMA", "alpha")
	alpha, err := daemonruntime.NewNamespace()
	require.NoError(t, err)

	t.Setenv("POOMSAE_POSTGRES_SCHEMA", "beta")
	beta, err := daemonruntime.NewNamespace()
	require.NoError(t, err)

	assert.NotEqual(t, alpha.DBHash, beta.DBHash)
	assert.NotEqual(t, alpha.DataDir, beta.DataDir)
	assert.NotEqual(t, alpha.SocketDir, beta.SocketDir)
}

func TestNamespace_SocketDirFallsBackToTmpDir(t *testing.T) {
	setupMockEnv(t)
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", "/var/folders/xy")

	ns, err := daemonruntime.NewNamespace()
	require.NoError(t, err)
	assert.Contains(t, ns.SocketDir, "poomsae-")
	assert.Contains(t, ns.SocketDir, ns.DBHash)
}

func TestNamespaceForHomeUsesSelectedHomeWithoutCreatingDirs(t *testing.T) {
	setupMockEnv(t)
	home := t.TempDir()
	hash := config.DBHash(filepath.Join(home, "poomsae.db"))
	ns, err := daemonruntime.NewNamespaceForHome(home, hash)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "runtime", hash), ns.DataDir)
	assert.Equal(t, hash, ns.DBHash)
	_, err = os.Stat(ns.DataDir)
	assert.ErrorIs(t, err, os.ErrNotExist)
	for _, invalid := range []string{"../outside", "", "ABCDEF123456"} {
		_, err = daemonruntime.NewNamespaceForHome(home, invalid)
		require.Error(t, err)
	}
}
