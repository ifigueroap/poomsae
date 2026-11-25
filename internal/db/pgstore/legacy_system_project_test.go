package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/pgstore"
	"github.com/ifigueroap/poomsae/internal/db/pgstore/pgstoretest"
)

// A database created before the kata -> poomsae rename holds the system
// project as .kata-system. Reopening it must rename that row in place, and its
// token events must carry the new name because import replay requires system
// events to name SystemProjectName.
func TestOpenRenamesLegacySystemProject(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	dsn, cleanup := pgstoretest.NewDatabase(t, ctx)
	t.Cleanup(cleanup)

	store, err := pgstore.Open(ctx, dsn)
	require.NoError(t, err)
	_, created, err := store.CreateAPIToken(ctx, db.CreateAPITokenParams{
		PlaintextToken: "legacy-token",
		Actor:          "automation",
		AdminActor:     db.BootstrapActor,
	})
	require.NoError(t, err)
	_, err = store.ExecContext(ctx, `UPDATE projects SET name = $1 WHERE uid = $2`,
		db.LegacySystemProjectName, db.SystemProjectUID)
	require.NoError(t, err)
	_, err = store.ExecContext(ctx, `UPDATE events SET project_name = $1 WHERE id = $2`,
		db.LegacySystemProjectName, created.ID)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	reopened, err := pgstore.Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })

	sys, err := reopened.SystemProject(ctx)
	require.NoError(t, err)
	assert.Equal(t, db.SystemProjectUID, sys.UID)
	assert.Equal(t, created.ProjectID, sys.ID)
	var eventProject string
	require.NoError(t, reopened.QueryRowContext(ctx,
		`SELECT project_name FROM events WHERE id = $1`, created.ID).Scan(&eventProject))
	assert.Equal(t, db.SystemProjectName, eventProject)
	var legacyRows int
	require.NoError(t, reopened.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM projects WHERE name = $1`, db.LegacySystemProjectName).Scan(&legacyRows))
	assert.Zero(t, legacyRows)
}
