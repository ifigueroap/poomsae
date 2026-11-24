package katamigrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kitdaemon "go.kenn.io/kit/daemon"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
)

func TestMigrateCopiesDataAndLeavesOriginal(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "poomsae-home")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "attachments"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "backups"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(src, "attachments", "note.txt"), []byte("attachment-bytes"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "backups", "night.tar"), []byte("backup-bytes"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "config.toml"), []byte("dsn = \""+filepath.Join(src, "kata.db")+"\"\n"), 0o600))

	store, err := sqlitestore.Open(t.Context(), filepath.Join(src, "kata.db"))
	require.NoError(t, err)
	version, err := store.SchemaVersion(t.Context())
	require.NoError(t, err)
	require.Equal(t, db.CurrentSchemaVersion(), version)
	require.NoError(t, store.Close())

	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, ".kata.toml"), []byte("version = 1\n\n[project]\nname = \"example\"\n"), 0o600)) //nolint:gosec // fixture under t.TempDir
	require.NoError(t, os.WriteFile(filepath.Join(workspace, ".kata.local.toml"), []byte("version = 1\n\n[server]\nurl = \"http://127.0.0.1:9\"\n"), 0o600))

	t.Setenv("KATA_HOME", src)
	t.Setenv("KATA_DB", "")
	t.Setenv("KATA_DSN", "")
	t.Setenv("POOMSAE_HOME", "")
	t.Setenv("POOMSAE_DB", "")
	t.Setenv("POOMSAE_DSN", "")

	result, err := Migrate(t.Context(), Options{Destination: dst, Sources: []string{src}, Workspace: workspace, Explicit: true})
	require.NoError(t, err)
	assert.True(t, result.Copied)
	assert.Equal(t, src, result.Source)
	assert.Equal(t, db.CurrentSchemaVersion(), result.Schema)
	assert.Equal(t, filepath.Join(dst, "poomsae.db"), result.Database)

	assert.Equal(t, "attachment-bytes", mustRead(t, filepath.Join(dst, "attachments", "note.txt")))
	assert.Equal(t, "backup-bytes", mustRead(t, filepath.Join(dst, "backups", "night.tar")))
	assert.Contains(t, mustRead(t, filepath.Join(dst, "config.toml")), filepath.Join(dst, "poomsae.db"))
	assert.Equal(t, "attachment-bytes", mustRead(t, filepath.Join(src, "attachments", "note.txt")))
	assert.FileExists(t, filepath.Join(src, "kata.db"))
	assert.FileExists(t, filepath.Join(dst, "kata.db"))

	project, err := os.ReadFile(filepath.Join(workspace, ".poomsae.toml")) //nolint:gosec // fixture under t.TempDir
	require.NoError(t, err)
	assert.Equal(t, "version = 1\n\n[project]\nname = \"example\"\n", string(project))
	original, err := os.ReadFile(filepath.Join(workspace, ".kata.toml")) //nolint:gosec // fixture under t.TempDir
	require.NoError(t, err)
	assert.Equal(t, string(project), string(original))

	again, err := Migrate(t.Context(), Options{Destination: dst, Sources: []string{src}, Workspace: workspace, Explicit: true})
	require.NoError(t, err)
	assert.False(t, again.Copied)
	assert.True(t, again.Already)
	assert.Empty(t, again.Projects)
	assert.Equal(t, "attachment-bytes", mustRead(t, filepath.Join(src, "attachments", "note.txt")))
}

func TestPromoteDatabaseCopiesSidecars(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kata.db"), []byte("db-bytes"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kata.db-wal"), []byte("wal-bytes"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kata.db-shm"), []byte("shm-bytes"), 0o600))
	require.NoError(t, promoteDatabase(dir))
	assert.Equal(t, "db-bytes", mustRead(t, filepath.Join(dir, "poomsae.db")))
	assert.Equal(t, "wal-bytes", mustRead(t, filepath.Join(dir, "poomsae.db-wal")))
	assert.Equal(t, "shm-bytes", mustRead(t, filepath.Join(dir, "poomsae.db-shm")))
	assert.Equal(t, "db-bytes", mustRead(t, filepath.Join(dir, "kata.db")))
	require.NoError(t, promoteDatabase(dir))
	assert.Equal(t, "db-bytes", mustRead(t, filepath.Join(dir, "poomsae.db")))
}

func TestMigrateRefusesToOverwriteDestination(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "kata.db"), []byte("original-db"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "keep.txt"), []byte("keep"), 0o600))
	result, err := Migrate(t.Context(), Options{Destination: dst, Sources: []string{src}})
	require.NoError(t, err)
	assert.True(t, result.Already)
	assert.False(t, result.Copied)
	assert.Equal(t, "keep", mustRead(t, filepath.Join(dst, "keep.txt")))
	_, err = os.Stat(filepath.Join(dst, "kata.db"))
	assert.Error(t, err)
}

func TestMigrateStopsKataDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal stop is covered by the unix daemon runtime")
	}
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dest")
	cmd := exec.CommandContext(t.Context(), "sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	runtimeDir := filepath.Join(src, "runtime", "abcdef012345")
	require.NoError(t, os.MkdirAll(runtimeDir, 0o700))
	_, err := (kitdaemon.RuntimeStore{Dir: runtimeDir}).Write(kitdaemon.RuntimeRecord{
		PID:       cmd.Process.Pid,
		Network:   "unix",
		Address:   filepath.Join(src, "daemon.sock"),
		StartedAt: time.Now().Add(-time.Second),
	})
	require.NoError(t, err)

	result, err := Migrate(t.Context(), Options{Destination: dst, Sources: []string{src}})
	require.NoError(t, err)
	assert.Contains(t, result.Stopped, cmd.Process.Pid)
	assert.False(t, kitdaemon.ProcessAlive(cmd.Process.Pid))
}

func TestSourceDirsPreferKataHomeThenPlatformDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", "")
	t.Setenv("LOCALAPPDATA", "")
	kataHome := t.TempDir()
	t.Setenv("KATA_HOME", kataHome)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".kata"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".local", "share", "kata"), 0o700))
	got := SourceDirs()
	require.NotEmpty(t, got)
	assert.Equal(t, kataHome, got[0])
	assert.Contains(t, got, filepath.Join(home, ".kata"))

	t.Setenv("KATA_HOME", "")
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	require.NoError(t, os.RemoveAll(filepath.Join(home, ".kata")))
	require.NoError(t, os.MkdirAll(filepath.Join(xdg, "kata"), 0o700))
	got = SourceDirs()
	assert.Equal(t, []string{filepath.Join(xdg, "kata")}, got)
}

func TestApplyLegacyKataEnvDoesNotOverridePoomsae(t *testing.T) {
	t.Setenv("KATA_AUTHOR", "from-kata")
	t.Setenv("POOMSAE_AUTHOR", "from-poomsae")
	t.Setenv("KATA_COLOR_MODE", "dark")
	unsetEnv(t, "POOMSAE_COLOR_MODE")
	mapped := config.ApplyLegacyKataEnv()
	assert.Equal(t, "from-poomsae", os.Getenv("POOMSAE_AUTHOR"))
	assert.Equal(t, "dark", os.Getenv("POOMSAE_COLOR_MODE"))
	assert.Contains(t, mapped, "KATA_COLOR_MODE -> POOMSAE_COLOR_MODE")
	assert.NotContains(t, mapped, "KATA_AUTHOR -> POOMSAE_AUTHOR")
	again := config.ApplyLegacyKataEnv()
	assert.Empty(t, again)
}

func TestAdoptLegacyProjectFilesIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	body := []byte("version = 1\n\n[project]\nname = \"example\"\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".kata.toml"), body, 0o600)) //nolint:gosec // fixture under t.TempDir
	copied, err := config.AdoptLegacyProjectFiles(dir)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(dir, ".poomsae.toml")}, copied)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".poomsae.toml"), []byte("keep\n"), 0o600)) //nolint:gosec // fixture under t.TempDir
	again, err := config.AdoptLegacyProjectFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, again)
	assert.Equal(t, "keep\n", mustRead(t, filepath.Join(dir, ".poomsae.toml")))
	assert.Equal(t, string(body), mustRead(t, filepath.Join(dir, ".kata.toml")))
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, existed := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // fixture under t.TempDir
	require.NoError(t, err)
	return string(data)
}
