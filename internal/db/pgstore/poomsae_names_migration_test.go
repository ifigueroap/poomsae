package pgstore_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/pgstore"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoomsaeNamesMigrationUpgradesVersion54(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}
	ctx := t.Context()
	dsn, cleanup := testenv.NewPostgresContainer(t, ctx)
	t.Cleanup(cleanup)
	const schema = "poomsae_names_upgrade"
	store, err := pgstore.OpenWithConfig(ctx, dsn, pgstore.Config{Schema: schema, SchemaMode: pgstore.SchemaModeBootstrap})
	require.NoError(t, err)
	project, err := store.CreateProject(ctx, "example-project")
	require.NoError(t, err)
	_, _, err = store.CreateIssue(ctx, db.CreateIssueParams{ProjectID: project.ID, Title: "Searchable title", Author: "agent"})
	require.NoError(t, err)
	require.NoError(t, store.Close())
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	restoreV54Names(ctx, t, admin, schema)
	_, err = admin.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %[1]s;
INSERT INTO external_field_mappings(connector_instance, kata_field, external_field_id, external_field_name,
  accepted_kinds_json, nullable, writable, schema_revision, created_at, updated_at)
VALUES('example-connector', 'deadline_on', 'due', 'Due', '[]', 1, 1, 'r1', now(), now());
INSERT INTO federation_embedding_artifacts(project_uid, digest, issue_uid, input_hash, recipe_fingerprint,
  vector_byte_size, manifest, artifact)
VALUES('%[2]s', 'digest', 'issue', 'input', 'recipe', 0, '{}', '{}');
UPDATE meta SET value='54' WHERE key='schema_version'`, schema, project.UID)) // #nosec G201 -- Fixed test schema and generated UID.
	require.NoError(t, err)

	upgraded, err := pgstore.OpenWithConfig(ctx, dsn, pgstore.Config{Schema: schema, SchemaMode: pgstore.SchemaModeBootstrap})
	require.NoError(t, err)
	t.Cleanup(func() { _ = upgraded.Close() })
	version, err := upgraded.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, db.CurrentSchemaVersion(), version)

	var field string
	require.NoError(t, admin.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT poomsae_field FROM %s.external_field_mappings WHERE connector_instance='example-connector'`, schema)).Scan(&field)) // #nosec G201 -- Fixed test schema.
	assert.Equal(t, "deadline_on", field)
	var artifacts int
	require.NoError(t, admin.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.federation_embedding_artifacts`, schema)).Scan(&artifacts)) // #nosec G201 -- Fixed test schema.
	assert.Zero(t, artifacts)
	results, err := upgraded.SearchFTS(ctx, db.SearchFTSParams{ProjectID: project.ID, Query: "searchable", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// restoreV54Names gives a current schema the version 54 names so tests
// can start an upgrade from that version.
func restoreV54Names(ctx context.Context, t *testing.T, admin *sql.DB, schema string) {
	t.Helper()
	_, err := admin.ExecContext(ctx, fmt.Sprintf(`SET search_path TO %s;
ALTER TABLE external_field_mappings RENAME COLUMN poomsae_field TO kata_field;
ALTER TABLE external_field_mappings
  RENAME CONSTRAINT external_field_mappings_poomsae_field_check TO external_field_mappings_kata_field_check;
ALTER TABLE external_field_states RENAME COLUMN conflict_poomsae TO conflict_kata;
ALTER TABLE external_field_states
  RENAME CONSTRAINT external_field_states_conflict_poomsae_check TO external_field_states_conflict_kata_check;
ALTER TEXT SEARCH CONFIGURATION poomsae_simple_unaccent RENAME TO kata_simple_unaccent;
CREATE OR REPLACE FUNCTION rebuild_issue_search(p_issue_id BIGINT) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO issues_search (issue_id, tsv)
    SELECT id, to_tsvector('kata_simple_unaccent', title || ' ' || coalesce(body, ''))
    FROM issues WHERE id = p_issue_id
    ON CONFLICT (issue_id) DO UPDATE SET tsv = EXCLUDED.tsv;
END $$;
RESET search_path`, schema)) // #nosec G201 -- Fixed test schema.
	require.NoError(t, err)
}
