package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKataDSN_PrefersKataDSNOverKataDB(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "postgres://db.example.com/poomsae")
	t.Setenv("POOMSAE_DB", "/tmp/should-be-shadowed.db")

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "postgres://db.example.com/poomsae", got)
}

func TestKataDSN_FallsBackToKataDBWhenKataDSNUnset(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "/tmp/from-env.db")

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "/tmp/from-env.db", got)
}

func TestKataDSN_DefaultsToHomeKataDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "poomsae.db"), got)
}

func TestKataDSN_TrimsEnvWhitespace(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "  postgres://h/db  ")
	t.Setenv("POOMSAE_DB", "")

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "postgres://h/db", got)
}

func TestKataDSN_RejectsUnknownScheme(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "mysql://h/db")
	t.Setenv("POOMSAE_DB", "")

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported dsn scheme")
	assert.Contains(t, err.Error(), "mysql")
}

func TestKataDSN_RejectsPGOnlyQueryParamsOnSQLiteDSN(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "sqlite:///tmp/k.db?sslmode=require")
	t.Setenv("POOMSAE_DB", "")

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sqlite")
	assert.Contains(t, err.Error(), "sslmode")
}

func TestKataDSN_RejectsPGOnlyQueryParamsOnBarePath(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "/tmp/k.db?pool_max_conns=8")
	t.Setenv("POOMSAE_DB", "")

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pool_max_conns")
}

func TestKataDSN_RejectsSchemeLessLibpqKeywordDSNFromEnv(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "host=db user=poomsae password=SECRET dbname=poomsae") //nolint:gosec // fixture verifies credential-free rejection
	t.Setenv("POOMSAE_DB", "")

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "libpq keyword")
	assert.NotContains(t, err.Error(), "SECRET")
	assert.NotContains(t, err.Error(), "password=SECRET")
}

func TestKataDSN_RejectsSchemeLessLibpqKeywordDSNWithSpacedEqualsFromEnv(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "host = db user = poomsae password = SECRET dbname = poomsae") //nolint:gosec // fixture verifies credential-free rejection
	t.Setenv("POOMSAE_DB", "")

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "libpq keyword")
	assert.NotContains(t, err.Error(), "SECRET")
	assert.NotContains(t, err.Error(), "password = SECRET")
}

func TestKataDSN_RejectsSchemeLessLibpqKeywordDSNFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	body := "[storage]\ndsn = \"host=db user=poomsae sslpassword=SECRET dbname=poomsae\"\n" //nolint:gosec // fixture verifies credential-free rejection
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0o600))

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "libpq keyword")
	assert.NotContains(t, err.Error(), "SECRET")
	assert.NotContains(t, err.Error(), "sslpassword=SECRET")
}

func TestKataDSN_RejectsSchemeLessLibpqKeywordDSNWithSpacedEqualsFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	body := "[storage]\ndsn = \"host = db user = poomsae sslpassword = SECRET dbname = poomsae\"\n" //nolint:gosec // fixture verifies credential-free rejection
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0o600))

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "libpq keyword")
	assert.NotContains(t, err.Error(), "SECRET")
	assert.NotContains(t, err.Error(), "sslpassword = SECRET")
}

func TestKataDSN_RejectsAmbiguousPostgresCredentials(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "postgres://user:p://w@host/db")
	t.Setenv("POOMSAE_DB", "")

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	// The error must NOT echo the embedded credential.
	assert.NotContains(t, err.Error(), "p://w")
}

func TestKataDSN_ValidationErrorOnPostgresOmitsPassword(t *testing.T) {
	// A bad-shape postgres DSN with a password — validateDSN runs the
	// CanonicalDSNIdentity probe which never echoes the input. Ensure the
	// password does not leak.
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "postgres://user:SECRET@host:badport/db") //nolint:gosec // fixture
	t.Setenv("POOMSAE_DB", "")

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET")
}

func TestKataDSN_AcceptsPostgresDSN(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "postgres://user:pw@host:5432/poomsae?sslmode=require")
	t.Setenv("POOMSAE_DB", "")

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "postgres://user:pw@host:5432/poomsae?sslmode=require", got)
}

func TestKataDSN_AcceptsSQLiteDSN(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "sqlite:///var/lib/poomsae/poomsae.db")
	t.Setenv("POOMSAE_DB", "")

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "sqlite:///var/lib/poomsae/poomsae.db", got)
}

func TestKataDSN_AcceptsBarePath(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_DSN", "/var/lib/poomsae/poomsae.db")
	t.Setenv("POOMSAE_DB", "")

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/poomsae/poomsae.db", got)
}

func TestKataDSN_ReadsStorageDSNFromConfigToml(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[storage]\ndsn = \"postgres://h/db\"\n"), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "postgres://h/db", got)
}

func TestKataDSN_KataDBOverridesStorageDSN(t *testing.T) {
	// User-specified precedence: POOMSAE_DSN > POOMSAE_DB > [storage].dsn > default.
	// Env vars beat the config file so a user's existing shell (with POOMSAE_DB
	// exported) keeps pointing at the same database after [storage].dsn is
	// added to config.toml — otherwise an absent-from-the-shell config-file
	// knob would silently redirect long-running scripts to a different DB.
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "/tmp/from-env.db")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[storage]\ndsn = \"postgres://from-toml/poomsae\"\n"), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "/tmp/from-env.db", got)
}

func TestKataDSN_StorageDSNUsedWhenKataDBUnset(t *testing.T) {
	// With env vars unset, the config file's [storage].dsn is picked up.
	// Confirms the TOML branch is reachable in the new precedence order.
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[storage]\ndsn = \"postgres://from-toml/poomsae\"\n"), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "postgres://from-toml/poomsae", got)
}

func TestKataDSN_KataDSNOverridesStorageDSN(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "postgres://from-env/poomsae")
	t.Setenv("POOMSAE_DB", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[storage]\ndsn = \"postgres://from-toml/poomsae\"\n"), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "postgres://from-env/poomsae", got)
}

func TestKataDSN_ValidatesStorageDSN(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[storage]\ndsn = \"mysql://h/db\"\n"), 0o600))

	_, err := config.PoomsaeDSN(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported dsn scheme")
	assert.Contains(t, err.Error(), "mysql")
}

func TestKataDSN_EmptyStorageDSNFallsThroughToKataDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "/tmp/from-env.db")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[storage]\ndsn = \"\"\n"), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "/tmp/from-env.db", got)
}

func TestKataDSN_EmptyStorageDSNFallsThroughToDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[storage]\ndsn = \"\"\n"), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "poomsae.db"), got)
}

// TestKataDSN_TolerantOfMalformedAuthSection is the scope-narrowing invariant:
// a parse error in an unrelated section (auth, listen, close, ...) must not
// block PoomsaeDSN from resolving its [storage].dsn. Legacy POOMSAE_DB callers
// were never gated on the daemon config; the new TOML branch must not
// regress that.
func TestKataDSN_TolerantOfMalformedAuthSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	// A genuinely broken auth section that ReadDaemonConfig would reject.
	body := "[storage]\ndsn = \"postgres://h/db\"\n[auth]\ntoken =\n"
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte(body), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err, "unrelated-section parse errors must not block PoomsaeDSN")
	assert.Equal(t, "postgres://h/db", got)
}

func TestKataDSN_ReadsCommentedStorageHeadingAndStopsAtCommentedPeer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	body := "[storage] # primary database\ndsn = \"postgres://h/db\"\n[auth] # unrelated\ntoken =\n"
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "postgres://h/db", got)
}

func TestKataDSN_RejectsUnknownStorageKeys(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		wantKey string
	}{
		{
			name:    "storage",
			body:    "[storage]\ndsn = \"postgres://h/db\"\nscheam = \"wrong\"\n",
			wantKey: "storage.scheam",
		},
		{
			name:    "postgres",
			body:    "[storage]\ndsn = \"postgres://h/db\"\n[storage.postgres]\nscheam = \"wrong\"\n[auth]\ntoken =\n",
			wantKey: "storage.postgres.scheam",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("POOMSAE_HOME", home)
			t.Setenv("POOMSAE_DSN", "")
			t.Setenv("POOMSAE_DB", "")
			require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte(test.body), 0o600))

			_, err := config.PoomsaeDSN(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantKey)
		})
	}
}

// TestKataDSN_TolerantOfMissingStorageSection confirms that a config.toml
// with no [storage] section at all is harmless — PoomsaeDSN falls through to
// POOMSAE_DB / default without complaint.
func TestKataDSN_TolerantOfMissingStorageSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_DSN", "")
	t.Setenv("POOMSAE_DB", "")
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("listen = \"127.0.0.1:7777\"\n"), 0o600))

	got, err := config.PoomsaeDSN(t.Context())
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "poomsae.db"), got)
}
