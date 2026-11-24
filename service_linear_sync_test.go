package poomsae

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/linearsync"
	"github.com/stretchr/testify/require"
)

const runtimeLinearStateID = "22222222-2222-4222-8222-222222222222"

type runtimeLinearFetcher struct {
	started, release chan struct{}
	once             sync.Once
}

func newRuntimeLinearFetcher() *runtimeLinearFetcher {
	return &runtimeLinearFetcher{started: make(chan struct{}), release: make(chan struct{})}
}
func (f *runtimeLinearFetcher) ForRun(context.Context, linearsync.Config) (linearsync.Session, error) {
	return f, nil
}
func (f *runtimeLinearFetcher) Scope(ctx context.Context, c linearsync.Config) (linearsync.Scope, error) {
	return linearsync.Scope{WorkspaceID: c.WorkspaceID, TeamID: c.TeamID, ProjectID: c.ProjectID, Name: "Example tasks"}, ctx.Err()
}
func (f *runtimeLinearFetcher) States(ctx context.Context, _ linearsync.Config) ([]linearsync.State, error) {
	return []linearsync.State{{ID: runtimeLinearStateID, Type: "started"}}, ctx.Err()
}
func (f *runtimeLinearFetcher) Comments(ctx context.Context, _ linearsync.Config, _ []string) ([]linearsync.Comment, error) {
	return nil, ctx.Err()
}
func (f *runtimeLinearFetcher) Issues(ctx context.Context, c linearsync.Config) ([]linearsync.Issue, error) {
	f.once.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []linearsync.Issue{{ID: "33333333-3333-4333-8333-333333333333", TeamID: c.TeamID, ProjectID: c.ProjectID, StateID: runtimeLinearStateID, Identifier: "EX-1", Title: "Scheduled task", URL: "https://linear.app/example-workspace/issue/EX-1/scheduled-task", CreatedAt: at, UpdatedAt: at}}, nil
}
func readRuntimeLinearStatus(t *testing.T, h http.Handler, id int64) runtimeNotionStatus {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/projects/%d/issue-sync/linear/status", id), nil))
	require.Equal(t, 200, w.Code, w.Body.String())
	var s runtimeNotionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	return s
}
func linearRuntimePost(ctx context.Context, id int64, action string, body io.Reader) *http.Request {
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/issue-sync/linear/%s", id, action), body)
	r.Header.Set("Content-Type", "application/json")
	return r
}
