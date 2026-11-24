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

	"github.com/ifigueroap/poomsae/internal/todoistsync"
	"github.com/stretchr/testify/require"
)

type runtimeTodoistFetcher struct {
	started, release chan struct{}
	once             sync.Once
}

func newRuntimeTodoistFetcher() *runtimeTodoistFetcher {
	return &runtimeTodoistFetcher{started: make(chan struct{}), release: make(chan struct{})}
}
func (f *runtimeTodoistFetcher) Account(context.Context) (string, error) { return "1234567", nil }
func (f *runtimeTodoistFetcher) ForRun(context.Context, todoistsync.Config) (todoistsync.Session, error) {
	return f, nil
}
func (f *runtimeTodoistFetcher) Project(ctx context.Context, c todoistsync.Config) (todoistsync.Project, error) {
	return todoistsync.Project{ID: c.ProjectID, Name: "Example tasks"}, ctx.Err()
}
func (f *runtimeTodoistFetcher) Tasks(ctx context.Context, c todoistsync.Config, _, _ time.Time) ([]todoistsync.Task, error) {
	f.once.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []todoistsync.Task{{ID: "33333333-3333-4333-8333-333333333333", ProjectID: c.ProjectID, Content: "Scheduled task", Priority: 1, AddedAt: at, UpdatedAt: at}}, nil
}
func readRuntimeTodoistStatus(t *testing.T, h http.Handler, id int64) runtimeNotionStatus {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/projects/%d/issue-sync/todoist/status", id), nil))
	require.Equal(t, 200, w.Code, w.Body.String())
	var s runtimeNotionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	return s
}
func todoistRuntimePost(ctx context.Context, id int64, action string, body io.Reader) *http.Request {
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/issue-sync/todoist/%s", id, action), body)
	r.Header.Set("Content-Type", "application/json")
	return r
}
