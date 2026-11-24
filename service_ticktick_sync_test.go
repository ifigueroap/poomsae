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

	"github.com/ifigueroap/poomsae/internal/tickticksync"
	"github.com/stretchr/testify/require"
)

type runtimeTickTickFetcher struct {
	started, release chan struct{}
	once             sync.Once
}

func newRuntimeTickTickFetcher() *runtimeTickTickFetcher {
	return &runtimeTickTickFetcher{started: make(chan struct{}), release: make(chan struct{})}
}
func (f *runtimeTickTickFetcher) ForRun(context.Context, tickticksync.Config) (tickticksync.Session, error) {
	return f, nil
}
func (f *runtimeTickTickFetcher) Project(ctx context.Context) (tickticksync.Project, error) {
	return tickticksync.Project{ID: "project-1", Name: "Example tasks", Kind: "TASK"}, ctx.Err()
}
func (f *runtimeTickTickFetcher) Data(ctx context.Context) (tickticksync.ProjectData, error) {
	f.once.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-ctx.Done():
		return tickticksync.ProjectData{}, ctx.Err()
	}
	return tickticksync.ProjectData{Project: tickticksync.Project{ID: "project-1", Name: "Example tasks", Kind: "TASK"}, Tasks: []tickticksync.Task{{ID: "task-1", ProjectID: "project-1", Title: "Scheduled task", Status: new(0)}}}, nil
}
func (f *runtimeTickTickFetcher) Task(ctx context.Context, id string) (tickticksync.Task, error) {
	return tickticksync.Task{ID: id, ProjectID: "project-1", Status: new(0)}, ctx.Err()
}
func readRuntimeTickTickStatus(t *testing.T, h http.Handler, id int64) runtimeNotionStatus {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/projects/%d/issue-sync/ticktick/status", id), nil))
	require.Equal(t, 200, w.Code, w.Body.String())
	var s runtimeNotionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	return s
}
func ticktickRuntimePost(ctx context.Context, id int64, action string, body io.Reader) *http.Request {
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/issue-sync/ticktick/%s", id, action), body)
	r.Header.Set("Content-Type", "application/json")
	return r
}
