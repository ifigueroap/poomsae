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

	"github.com/ifigueroap/poomsae/internal/twentysync"
	"github.com/stretchr/testify/require"
)

type runtimeTwentyFetcher struct {
	started, release chan struct{}
	once             sync.Once
}

func newRuntimeTwentyFetcher() *runtimeTwentyFetcher {
	return &runtimeTwentyFetcher{started: make(chan struct{}), release: make(chan struct{})}
}
func (f *runtimeTwentyFetcher) ForRun(context.Context, twentysync.Config) (twentysync.Session, error) {
	return f, nil
}
func (f *runtimeTwentyFetcher) Workspace(ctx context.Context, _ twentysync.Config) (twentysync.Workspace, error) {
	return twentysync.Workspace{ID: "11111111-1111-4111-8111-111111111111", DisplayName: "Example tasks"}, ctx.Err()
}
func (f *runtimeTwentyFetcher) Schema(ctx context.Context, _ twentysync.Config) (twentysync.Schema, error) {
	return twentysync.Schema{StatusOptions: []string{"TODO", "IN_PROGRESS", "DONE"}}, ctx.Err()
}
func (f *runtimeTwentyFetcher) Tasks(ctx context.Context, _ twentysync.Config) ([]twentysync.Task, error) {
	f.once.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []twentysync.Task{{ID: "33333333-3333-4333-8333-333333333333", Title: "Scheduled task", CreatedAt: at, UpdatedAt: at}}, nil
}
func readRuntimeTwentyStatus(t *testing.T, h http.Handler, id int64) runtimeNotionStatus {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/projects/%d/issue-sync/twenty/status", id), nil))
	require.Equal(t, 200, w.Code, w.Body.String())
	var s runtimeNotionStatus
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	return s
}
func twentyRuntimePost(ctx context.Context, id int64, action string, body io.Reader) *http.Request {
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/issue-sync/twenty/%s", id, action), body)
	r.Header.Set("Content-Type", "application/json")
	return r
}
