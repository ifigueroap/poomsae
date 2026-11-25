package sqlitestore_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/ifigueroap/poomsae/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_AppliesPragmas(t *testing.T) { //nolint:paralleltest // sets POOMSAE_TEST_FAST_SQLITE
	t.Setenv("POOMSAE_TEST_FAST_SQLITE", "")

	d := openTestDB(t)

	var fk int
	require.NoError(t, d.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&fk))
	assert.Equal(t, 1, fk)

	var mode string
	require.NoError(t, d.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", mode)
}

// TestOpen_OnFreshDBBootstrapsSchema confirms the new Open contract: a fresh
// path returns a handle whose meta table and schema_version row are already
// in place, courtesy of the bootstrap-on-Open transaction.
func TestOpen_OnFreshDBBootstrapsSchema(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "poomsae.db")
	d, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	var n int
	require.NoError(t, d.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&n))
	assert.Equal(t, 1, n)

	v, err := d.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, db.CurrentSchemaVersion(), v)
}

// TestOpen_IsIdempotentAfterBootstrap confirms that re-opening an
// already-bootstrapped DB succeeds and reports the same schema_version and
// instance_uid.
func TestOpen_IsIdempotentAfterBootstrap(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "poomsae.db")

	d1, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	uid1 := d1.InstanceUID()
	require.NoError(t, d1.Close())

	d2, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d2.Close() })

	v, err := d2.SchemaVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, db.CurrentSchemaVersion(), v)
	assert.Equal(t, uid1, d2.InstanceUID())
}

// Version 55 renames the version 54 columns in place and clears embedding
// artifacts, which all predate the poomsae digest and recipe names.
func TestPoomsaeNamesMigrationUpgradesVersion54(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "poomsae.db")
	d, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	project, err := d.CreateProject(ctx, "example-project")
	require.NoError(t, err)
	_, err = d.ExecContext(ctx, `
ALTER TABLE external_field_mappings RENAME COLUMN poomsae_field TO kata_field;
ALTER TABLE external_field_states RENAME COLUMN conflict_poomsae TO conflict_kata;
INSERT INTO external_field_mappings(connector_instance, kata_field, external_field_id, external_field_name,
  accepted_kinds_json, nullable, writable, schema_revision, created_at, updated_at)
VALUES('example-connector', 'deadline_on', 'due', 'Due', '[]', 1, 1, 'r1', '2026-11-25T00:00:00Z', '2026-11-25T00:00:00Z');
INSERT INTO federation_embedding_artifacts(project_uid, digest, issue_uid, input_hash, recipe_fingerprint,
  vector_byte_size, manifest, artifact)
VALUES(?, 'digest', 'issue', 'input', 'recipe', 0, '{}', '{}');
UPDATE meta SET value='54' WHERE key='schema_version'`, project.UID)
	require.NoError(t, err)
	require.NoError(t, d.Close())

	require.NoError(t, sqlitestore.UpgradeInPlace(ctx, path))
	upgraded, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = upgraded.Close() })
	version, err := upgraded.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, db.CurrentSchemaVersion(), version)

	var field string
	require.NoError(t, upgraded.QueryRowContext(ctx,
		`SELECT poomsae_field FROM external_field_mappings WHERE connector_instance='example-connector'`).Scan(&field))
	assert.Equal(t, "deadline_on", field)
	var artifacts int
	require.NoError(t, upgraded.QueryRowContext(ctx, `SELECT COUNT(*) FROM federation_embedding_artifacts`).Scan(&artifacts))
	assert.Zero(t, artifacts)

	fresh := openTestDB(t)
	const catalog = `SELECT name, sql FROM sqlite_master
WHERE tbl_name IN ('external_field_mappings', 'external_field_states') AND sql IS NOT NULL ORDER BY name`
	assert.Equal(t, sqliteCatalogRows(t, fresh, catalog), sqliteCatalogRows(t, upgraded, catalog))
}

func sqliteCatalogRows(t *testing.T, store *sqlitestore.Store, query string) [][2]string {
	t.Helper()
	rows, err := store.QueryContext(t.Context(), query)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out [][2]string
	for rows.Next() {
		var row [2]string
		require.NoError(t, rows.Scan(&row[0], &row[1]))
		out = append(out, row)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestFederationCursorFenceMigrationUpgradesVersion53(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "poomsae.db")
	d, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	_, err = d.ExecContext(ctx, `ALTER TABLE external_field_mappings RENAME COLUMN poomsae_field TO kata_field;
ALTER TABLE external_field_states RENAME COLUMN conflict_poomsae TO conflict_kata;
UPDATE meta SET value='53' WHERE key='schema_version'`)
	require.NoError(t, err)
	require.NoError(t, d.Close())
	require.NoError(t, sqlitestore.UpgradeInPlace(ctx, path))
	d, err = sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	version, err := d.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, db.CurrentSchemaVersion(), version)
}

func TestOpen_RejectsVersionZeroExistingTables(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "poomsae.db")

	d, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	_, err = d.ExecContext(ctx, `UPDATE meta SET value='0' WHERE key='schema_version'`)
	require.NoError(t, err)
	require.NoError(t, d.Close())

	reopened, err := sqlitestore.Open(ctx, path)
	assert.Nil(t, reopened)
	require.Error(t, err)
	assert.ErrorIs(t, err, sqlitestore.ErrSchemaCutoverRequired)
}

func TestSchema_IssuesHasShortIDColumn(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)
	var typ string
	err := d.QueryRowContext(t.Context(),
		`SELECT type FROM pragma_table_info('issues') WHERE name='short_id'`,
	).Scan(&typ)
	require.NoError(t, err)
	assert.Equal(t, "TEXT", typ)
}

func TestSchema_IssuesNumberColumnGone(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)
	var n int
	err := d.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_table_info('issues') WHERE name='number'`,
	).Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestSchema_ProjectsNextIssueNumberGone(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)
	var n int
	err := d.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='next_issue_number'`,
	).Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestSchema_EventsIssueNumberGone(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)
	var n int
	err := d.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_table_info('events') WHERE name='issue_number'`,
	).Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestSchema_PurgeLogIssueNumberGone(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)
	var n int
	err := d.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_table_info('purge_log') WHERE name='issue_number'`,
	).Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestSchema_ProjectNameRejectsHash(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)
	_, err := d.ExecContext(t.Context(),
		`INSERT INTO projects(uid, name) VALUES('01HZNQ7VFPK1XGD8R5MABCD4EX', 'has#hash')`,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK")
}

func TestSchemaVersion(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)
	ctx := t.Context()
	v, err := d.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, db.CurrentSchemaVersion(), v)
}

func TestOpen_TimestampColumnsScanIntoTime(t *testing.T) {
	t.Parallel()
	d := openTestDB(t)

	projectUID, err := uid.New()
	require.NoError(t, err)
	_, err = d.ExecContext(t.Context(), `INSERT INTO projects(uid, name) VALUES(?,'x')`, projectUID)
	require.NoError(t, err)

	rows, err := d.QueryContext(t.Context(), `SELECT created_at FROM projects`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var ts any
	require.NoError(t, rows.Scan(&ts))
	// modernc.org/sqlite returns time.Time for DATETIME columns
	_, ok := ts.(interface{ Year() int })
	assert.True(t, ok, "expected time.Time, got %T", ts)
}

func TestCheckpointTruncatesWAL(t *testing.T) { //nolint:paralleltest // sets POOMSAE_TEST_FAST_SQLITE
	t.Setenv("POOMSAE_TEST_FAST_SQLITE", "")

	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "poomsae.db")
	d, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	d.SetMaxOpenConns(1)

	_, err = d.ExecContext(ctx, `PRAGMA wal_autocheckpoint=0`)
	require.NoError(t, err)
	_, err = d.ExecContext(ctx, `CREATE TABLE checkpoint_noise(id INTEGER PRIMARY KEY, body BLOB)`)
	require.NoError(t, err)
	_, err = d.ExecContext(ctx, `
		WITH RECURSIVE seq(x) AS (
			SELECT 1
			UNION ALL
			SELECT x + 1 FROM seq WHERE x < 128
		)
		INSERT INTO checkpoint_noise(body)
		SELECT randomblob(4096) FROM seq`)
	require.NoError(t, err)

	before, err := os.Stat(path + "-wal")
	require.NoError(t, err)
	require.Positive(t, before.Size(), "test setup must create WAL frames")

	require.NoError(t, d.Checkpoint(ctx))

	after, err := os.Stat(path + "-wal")
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	require.NoError(t, err)
	assert.Zero(t, after.Size(), "TRUNCATE checkpoint should leave no WAL bytes")
}

func TestOpenUsesFastSQLitePragmasWhenTestHarnessRequestsIt(t *testing.T) { //nolint:paralleltest // sets POOMSAE_TEST_FAST_SQLITE
	t.Setenv("POOMSAE_TEST_FAST_SQLITE", "1")

	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "poomsae.db")
	d, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	var mode string
	require.NoError(t, d.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", mode)

	var sync int
	require.NoError(t, d.QueryRowContext(t.Context(), "PRAGMA synchronous").Scan(&sync))
	assert.Zero(t, sync)

	var tempStore int
	require.NoError(t, d.QueryRowContext(t.Context(), "PRAGMA temp_store").Scan(&tempStore))
	assert.Equal(t, 2, tempStore)
}

func TestOpenURICharactersUseExactPath(t *testing.T) {
	ctx := t.Context()
	t.Setenv("POOMSAE_HOME", t.TempDir())
	names := []string{"example#database.db", "example%2Fdatabase.db"}
	if runtime.GOOS != "windows" {
		names = append(names, "example?mode=ro.db")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			store, err := sqlitestore.Open(ctx, path)
			require.NoError(t, err)
			project, err := store.CreateProject(ctx, "example-project")
			require.NoError(t, err)
			instance := store.InstanceUID()
			require.NoError(t, store.Close())
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Positive(t, info.Size(), "the exact path, including URI punctuation, holds the database")
			store, err = sqlitestore.Open(ctx, path, db.ReadOnly())
			require.NoError(t, err)
			defer func() { require.NoError(t, store.Close()) }()
			restored, err := store.ProjectByID(ctx, project.ID)
			require.NoError(t, err)
			require.Equal(t, project.UID, restored.UID)
			var restoredInstance string
			require.NoError(t, store.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='instance_uid'`).Scan(&restoredInstance))
			require.Equal(t, instance, restoredInstance)
		})
	}
}
