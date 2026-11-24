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

	"github.com/ifigueroap/poomsae/internal/planesync"
	"github.com/stretchr/testify/require"
)

const runtimePlaneStateID = "22222222-2222-4222-8222-222222222222"

type runtimePlaneFetcher struct {
	started, release chan struct{}
	once             sync.Once
}

func newRuntimePlaneFetcher() *runtimePlaneFetcher {
	return &runtimePlaneFetcher{started: make(chan struct{}), release: make(chan struct{})}
}
func (f *runtimePlaneFetcher) ForRun(context.Context, planesync.Config) (planesync.Session, error) {
	return f, nil
}
func (f *runtimePlaneFetcher) Project(ctx context.Context, c planesync.Config) (planesync.Project, error) {
	return planesync.Project{ID: c.ProjectID, Name: "Example tasks", Identifier: "EX"}, ctx.Err()
}
func (f *runtimePlaneFetcher) States(ctx context.Context, _ planesync.Config) ([]planesync.State, error) {
	return []planesync.State{{ID: runtimePlaneStateID, Group: "started"}}, ctx.Err()
}
func (f *runtimePlaneFetcher) WorkItems(ctx context.Context, c planesync.Config) ([]planesync.WorkItem, error) {
	f.once.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []planesync.WorkItem{{ID: "33333333-3333-4333-8333-333333333333", ProjectID: c.ProjectID, StateID: runtimePlaneStateID, SequenceID: 1, Name: "Scheduled task", CreatedAt: at, UpdatedAt: at}}, nil
}
func readRuntimePlaneStatus(t *testing.T, h http.Handler, id int64) runtimeNotionStatus {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/projects/%d/issue-sync/plane/status", id), nil))
	require.Equal(t, 200, w.Code, w.Body.String())
	var s runtimeNotionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	return s
}
func planeRuntimePost(ctx context.Context, id int64, action string, body io.Reader) *http.Request {
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/issue-sync/plane/%s", id, action), body)
	r.Header.Set("Content-Type", "application/json")
	return r
}
