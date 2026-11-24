//go:build !windows

package e2e_test

import (
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitForFederatedTitle(
	t *testing.T,
	store *sqlitestore.Store,
	title string,
	daemonStderr *safeBuffer,
	timeout time.Duration,
) db.Issue {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		var uid string
		err := store.QueryRowContext(t.Context(),
			`SELECT uid FROM issues WHERE title = ?`, title).Scan(&uid)
		if err == nil {
			issue, err := store.IssueByUID(t.Context(), uid, db.IncludeDeletedNo)
			require.NoError(t, err)
			return issue
		}
		lastErr = err
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("federated issue title %q was not materialized: %v\ndaemon stderr: %s",
		title, lastErr, daemonStderr.String())
	return db.Issue{}
}

func waitForFoldedProjectionMatch(
	t *testing.T,
	hub *sqlitestore.Store,
	spoke *sqlitestore.Store,
	hubProjectID int64,
	spokeProjectID int64,
	hubAfterID int64,
	daemonStderr *safeBuffer,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastHub, lastSpoke db.FoldProjection
	for time.Now().Before(deadline) {
		var err error
		lastHub, lastSpoke, err = foldedProjections(t, hub, spoke, hubProjectID, spokeProjectID, hubAfterID)
		require.NoError(t, err)
		if assert.ObjectsAreEqual(lastHub.Issues, lastSpoke.Issues) &&
			assert.ObjectsAreEqual(lastHub.Comments, lastSpoke.Comments) &&
			assert.ObjectsAreEqual(lastHub.Labels, lastSpoke.Labels) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.Equal(t, lastHub.Issues, lastSpoke.Issues)
	assert.Equal(t, lastHub.Comments, lastSpoke.Comments)
	assert.Equal(t, lastHub.Labels, lastSpoke.Labels)
	t.Fatalf("folded projections did not converge\ndaemon stderr: %s", daemonStderr.String())
}

func foldedProjections(
	t *testing.T,
	hub *sqlitestore.Store,
	spoke *sqlitestore.Store,
	hubProjectID int64,
	spokeProjectID int64,
	hubAfterID int64,
) (db.FoldProjection, db.FoldProjection, error) {
	t.Helper()
	ctx := t.Context()
	hubEvents, err := hub.EventsAfter(ctx, db.EventsAfterParams{
		ProjectID: hubProjectID,
		AfterID:   hubAfterID,
		Limit:     1000,
	})
	if err != nil {
		return db.FoldProjection{}, db.FoldProjection{}, err
	}
	spokeEvents, err := spoke.EventsAfter(ctx, db.EventsAfterParams{
		ProjectID: spokeProjectID,
		Limit:     1000,
	})
	if err != nil {
		return db.FoldProjection{}, db.FoldProjection{}, err
	}
	return db.FoldEvents(foldEvents(hubEvents)), db.FoldEvents(foldEvents(spokeEvents)), nil
}
