package dbtest

import (
	"encoding/json/v2"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/transcript"
	"github.com/stretchr/testify/require"
)

func checkCloseTranscript(t *testing.T, store db.Storage) error {
	ctx := t.Context()
	project, err := store.CreateProject(ctx, "example-project")
	if err != nil {
		return err
	}
	ref := &transcript.Transcript{Agent: "claude", SessionID: "00000000-0000-4000-8000-000000000001", URL: "https://agentsview.example/sessions/00000000-0000-4000-8000-000000000001"}
	issue, err := createFixtureIssue(ctx, store, project.ID, "Completed example work", "worker", nil)
	if err != nil {
		return err
	}
	_, events, changed, err := store.CloseIssueGuarded(ctx, db.CloseIssueParams{IssueID: issue.ID, Reason: "done", Actor: "worker", Transcript: ref, Evidence: []db.Evidence{{Type: "test", Command: "go test ./internal/example"}}})
	if err != nil {
		return err
	}
	require.True(t, changed)
	require.NotEmpty(t, events)
	var payload struct {
		Transcript *transcript.Transcript `json:"transcript"`
	}
	require.NoError(t, json.Unmarshal([]byte(events[0].Payload), &payload))
	require.Equal(t, ref, payload.Transcript)
	return nil
}
