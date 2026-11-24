//go:build !windows

package e2e_test

import (
	"encoding/json"
	"encoding/json/jsontext"
	"net/http"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodePOST(t *testing.T, client *http.Client, url string, body, out any) {
	t.Helper()
	resp := postJSON(t, client, url, body)
	defer func() { _ = resp.Body.Close() }()
	require.Equalf(t, http.StatusOK, resp.StatusCode, "POST %s", url)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
}

func waitForFederatedIssue(t *testing.T, store *sqlitestore.Store, issueUID string, daemonStderr *safeBuffer) db.Issue {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		issue, err := store.IssueByUID(t.Context(), issueUID, db.IncludeDeletedYes)
		if err == nil {
			return issue
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("federated issue %s was not materialized: %v\ndaemon stderr: %s",
		issueUID, lastErr, daemonStderr.String())
	return db.Issue{}
}

func assertFoldedProjectionMatch(t *testing.T, hub, spoke *sqlitestore.Store, hubProjectID, spokeProjectID, hubAfterID int64) {
	t.Helper()
	ctx := t.Context()
	hubEvents, err := hub.EventsAfter(ctx, db.EventsAfterParams{
		ProjectID: hubProjectID,
		AfterID:   hubAfterID,
		Limit:     1000,
	})
	require.NoError(t, err)
	spokeEvents, err := spoke.EventsAfter(ctx, db.EventsAfterParams{
		ProjectID: spokeProjectID,
		Limit:     1000,
	})
	require.NoError(t, err)
	hubFold := db.FoldEvents(foldEvents(hubEvents))
	spokeFold := db.FoldEvents(foldEvents(spokeEvents))
	assert.Equal(t, hubFold.Issues, spokeFold.Issues)
	assert.Equal(t, hubFold.Comments, spokeFold.Comments)
	assert.Equal(t, hubFold.Labels, spokeFold.Labels)
}

func foldEvents(events []db.Event) []db.FoldEvent {
	out := make([]db.FoldEvent, 0, len(events))
	for _, ev := range events {
		var issueUID string
		if ev.IssueUID != nil {
			issueUID = *ev.IssueUID
		}
		var relatedUID string
		if ev.RelatedIssueUID != nil {
			relatedUID = *ev.RelatedIssueUID
		}
		out = append(out, db.FoldEvent{
			UID:               ev.UID,
			OriginInstanceUID: ev.OriginInstanceUID,
			ProjectUID:        ev.ProjectUID,
			IssueUID:          issueUID,
			RelatedIssueUID:   relatedUID,
			Type:              ev.Type,
			Actor:             ev.Actor,
			HLCPhysicalMS:     ev.HLCPhysicalMS,
			HLCCounter:        ev.HLCCounter,
			CreatedAt:         ev.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			Payload:           jsontext.Value(ev.Payload),
		})
	}
	return out
}
