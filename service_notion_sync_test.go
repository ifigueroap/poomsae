package poomsae

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/githubsync"
	"github.com/ifigueroap/poomsae/internal/notionsync"
	"github.com/stretchr/testify/require"
)

type runtimeGitHubSourceFetcher struct {
	serviceGitHubFetcher
	started chan struct{}
}

func (f *runtimeGitHubSourceFetcher) Repository(ctx context.Context, _, _, _ string) (githubsync.Repository, error) {
	close(f.started)
	<-ctx.Done()
	return githubsync.Repository{}, ctx.Err()
}
func TestServiceNotionConfigRejectsInvalidSelector(t *testing.T) {
	_, err := New(t.Context(), Config{DSN: filepath.Join(t.TempDir(), "service.db"), Auth: AuthConfig{TrustCallerAuthentication: true}, NotionSync: NotionSyncConfig{TokenEnv: "INVALID=NAME"}})
	require.ErrorContains(t, err, "notion_sync.token_env")
}

const runtimeNotionSourceID = "11111111-1111-1111-1111-111111111111"
const runtimeNotionDatabaseID = "22222222-2222-2222-2222-222222222222"

type runtimeNotionFetcher struct {
	sourceStarted, releaseSource, contentStarted, releaseContent chan struct{}
	sourceOnce, contentOnce                                      sync.Once
}

func newRuntimeNotionFetcher() *runtimeNotionFetcher {
	return &runtimeNotionFetcher{sourceStarted: make(chan struct{}), releaseSource: make(chan struct{}), contentStarted: make(chan struct{}), releaseContent: make(chan struct{})}
}
func (f *runtimeNotionFetcher) ForRun(context.Context) (notionsync.Session, error) { return f, nil }
func (f *runtimeNotionFetcher) DataSource(ctx context.Context, id string) (notionsync.DataSource, error) {
	f.sourceOnce.Do(func() { close(f.sourceStarted) })
	select {
	case <-f.releaseSource:
	case <-ctx.Done():
		return notionsync.DataSource{}, ctx.Err()
	}
	return notionsync.DataSource{ID: id, DatabaseID: runtimeNotionDatabaseID, Name: "Tasks", Properties: []notionsync.Property{
		{ID: "title", Name: "Task", Type: "title"}, {ID: "status", Name: "State", Type: "status", Options: []notionsync.Option{{ID: "done", Name: "Delivered"}}}, {ID: "people", Name: "Owner", Type: "people"},
	}}, nil
}
func (*runtimeNotionFetcher) Database(context.Context, string) (notionsync.Database, error) {
	return notionsync.Database{ID: runtimeNotionDatabaseID, DataSources: []notionsync.Option{{ID: runtimeNotionSourceID, Name: "Tasks"}}}, nil
}
func (*runtimeNotionFetcher) Pages(context.Context, notionsync.Config, *time.Time) ([]notionsync.Page, error) {
	return []notionsync.Page{{ID: "33333333-3333-3333-3333-333333333333", URL: "https://www.notion.so/33333333333333333333333333333333", DataSourceID: runtimeNotionSourceID, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}}, nil
}
func (f *runtimeNotionFetcher) Content(ctx context.Context, _ notionsync.Config, p notionsync.Page) (notionsync.PageContent, error) {
	f.contentOnce.Do(func() { close(f.contentStarted) })
	select {
	case <-f.releaseContent:
	case <-ctx.Done():
		return notionsync.PageContent{}, ctx.Err()
	}
	return notionsync.PageContent{Page: p, Title: "Scheduled task", Markdown: "Body"}, nil
}
func runtimeNotionBinding(t *testing.T, store db.Storage, projectID int64) db.IssueSyncBinding {
	t.Helper()
	raw, err := notionsync.EncodeConfig(notionsync.Config{DataSourceID: runtimeNotionSourceID, DatabaseID: runtimeNotionDatabaseID, TitlePropertyID: "title", StatusPropertyID: "status", AssigneePropertyID: "people", DoneStatusIDs: []string{"done"}})
	require.NoError(t, err)
	binding, err := store.UpsertIssueSyncBinding(t.Context(), db.UpsertIssueSyncBindingParams{ProjectID: projectID, Provider: "notion", SourceKey: "notion:" + runtimeNotionSourceID, RemoteID: runtimeNotionSourceID, DisplayName: "Tasks", Config: raw, IntervalSeconds: 300})
	require.NoError(t, err)
	return binding
}

type runtimeNotionStatus struct {
	Status struct {
		State       string `json:"state"`
		LastCreated int    `json:"last_created"`
		Progress    *struct {
			Phase            string `json:"phase"`
			Completed, Total int
			StartedAt        time.Time `json:"started_at"`
		} `json:"progress"`
	} `json:"status"`
}

func readRuntimeNotionStatus(t *testing.T, handler http.Handler, projectID int64) runtimeNotionStatus {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/projects/%d/issue-sync/notion/status", projectID), nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var status runtimeNotionStatus
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	return status
}
func waitRuntimeNotion(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "scheduled Notion worker did not reach upstream read")
	}
}

func newRuntimeNotionPost(ctx context.Context, path string, body io.Reader) *http.Request {
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, path, body)
	request.Header.Set("Content-Type", "application/json")
	return request
}

// runtimeNotionRecordingStore exposes the real durable completion to tests.
// It delegates all product storage behavior before signaling the observer.
type runtimeNotionRecordingStore struct {
	db.Storage
	recorded chan db.IssueSyncStatus
}

func (s *runtimeNotionRecordingStore) RecordIssueSyncSuccess(ctx context.Context, params db.IssueSyncSuccessParams) (db.IssueSyncStatus, error) {
	status, err := s.Storage.RecordIssueSyncSuccess(ctx, params)
	if err == nil {
		s.recorded <- status
	}
	return status, err
}
func (s *runtimeNotionRecordingStore) RecordIssueSyncError(ctx context.Context, params db.IssueSyncErrorParams) (db.IssueSyncStatus, error) {
	status, err := s.Storage.RecordIssueSyncError(ctx, params)
	if err == nil {
		s.recorded <- status
	}
	return status, err
}
func waitRuntimeNotionRecord(t *testing.T, recorded <-chan db.IssueSyncStatus) db.IssueSyncStatus {
	t.Helper()
	select {
	case status := <-recorded:
		return status
	case <-time.After(5 * time.Second):
		require.FailNow(t, "scheduled Notion worker did not record durable completion")
	}
	return db.IssueSyncStatus{}
}
