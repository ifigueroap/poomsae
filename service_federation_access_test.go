package poomsae_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/json/jsontext"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/stretchr/testify/require"
)

type recordingFederationAccessController struct {
	mu       sync.Mutex
	requests []poomsae.FederationAccessRequest
	decide   func(poomsae.FederationAccessRequest) (poomsae.FederationAccessDecision, error)
}

func (c *recordingFederationAccessController) AuthorizeFederation(
	_ context.Context,
	request poomsae.FederationAccessRequest,
) (poomsae.FederationAccessDecision, error) {
	c.mu.Lock()
	c.requests = append(c.requests, request)
	c.mu.Unlock()
	if c.decide != nil {
		return c.decide(request)
	}
	return poomsae.FederationAccessDecision{}, nil
}

func (c *recordingFederationAccessController) snapshot() []poomsae.FederationAccessRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]poomsae.FederationAccessRequest(nil), c.requests...)
}

type readTrackingBody struct {
	content []byte
	reads   int
}

func (b *readTrackingBody) Read(p []byte) (int, error) {
	b.reads++
	if len(b.content) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b.content)
	b.content = b.content[n:]
	return n, nil
}

func allowFederationMutation(request poomsae.FederationAccessRequest) (poomsae.FederationAccessDecision, error) {
	decision := poomsae.FederationAccessDecision{}
	if request.Operation.Mutation {
		decision.TransactionFence = func(context.Context, poomsae.Transaction) error { return nil }
	}
	return decision, nil
}

type revokeEnrollmentOnRead struct {
	once   sync.Once
	reader *bytes.Reader
	revoke func() error
	err    error
}

func (b *revokeEnrollmentOnRead) Read(p []byte) (int, error) {
	b.once.Do(func() { b.err = b.revoke() })
	return b.reader.Read(p)
}

func ingestFederationIssueAsPrincipal(
	t *testing.T,
	service *poomsae.Service,
	project poomsae.Project,
	token string,
) *httptest.ResponseRecorder {
	t.Helper()
	body := federationIssueIngestBody(t, project)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/projects/"+
		strconv.FormatInt(project.ID, 10)+"/federation/events:ingest", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), poomsae.Principal{
		Subject: "user-123", Actor: "Example User",
	}))
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	return response
}

func federationIssueIngestBody(t *testing.T, project poomsae.Project) []byte {
	t.Helper()
	createdAt := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	issueUID := "01HZNQ7VFPK1XGD8R5MABCD4EC"
	payload := jsontext.Value(`{"uid":"01HZNQ7VFPK1XGD8R5MABCD4EC","short_id":"cd4ec","title":"remote work","body":"","author":"Example Operator","status":"open","metadata":{},"created_at":"2026-07-22T12:00:00.000Z"}`)
	hash, err := db.EventContentHash(db.EventHashInput{
		UID:               "01HZNQ7VFPK1XGD8R5MABCD4EB",
		OriginInstanceUID: "01HZNQ7VFPK1XGD8R5MABCD4EA",
		ProjectUID:        project.UID,
		ProjectName:       project.Name,
		IssueUID:          &issueUID,
		Type:              "issue.created",
		Actor:             "Example Operator",
		HLCPhysicalMS:     1,
		CreatedAt:         "2026-07-22T12:00:00.000Z",
		Payload:           payload,
	})
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{
		"schema_version": db.CurrentSchemaVersion(),
		"events": []map[string]any{{
			"event_id": 17, "event_uid": "01HZNQ7VFPK1XGD8R5MABCD4EB",
			"origin_instance_uid": "01HZNQ7VFPK1XGD8R5MABCD4EA",
			"project_uid":         project.UID, "project_name": project.Name,
			"issue_uid": issueUID, "type": "issue.created", "actor": "Example Operator",
			"hlc_physical_ms": 1, "hlc_counter": 0, "content_hash": hash,
			"payload": jsontext.Value(payload), "created_at": createdAt,
		}},
	})
	require.NoError(t, err)
	return body
}

func storedFederationIssueCount(t *testing.T, databasePath string) int {
	t.Helper()
	inspection, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, inspection.Close()) })
	var count int
	require.NoError(t, inspection.QueryRowContext(t.Context(),
		`SELECT count(*) FROM issues WHERE uid = ?`, "01HZNQ7VFPK1XGD8R5MABCD4EC",
	).Scan(&count))
	return count
}

func forceFederationBaselineRefresh(t *testing.T, databasePath string, project poomsae.Project) {
	t.Helper()
	inspection, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, inspection.Close()) })
	_, err = inspection.ExecContext(t.Context(), `
		INSERT INTO purge_log(
			uid, origin_instance_uid, project_id, purged_issue_id,
			project_uid, project_name, issue_title, issue_author,
			comment_count, link_count, label_count, event_count,
			purge_reset_after_event_id, short_id, actor
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, ?, ?, ?)`,
		"01HZNQ7VFPK1XGD8R5MABCD4EB", "01HZNQ7VFPK1XGD8R5MABCD4EC",
		project.ID, 999, project.UID, project.Name, "purged issue", "Example User",
		999999, "d4eb", "Example User")
	require.NoError(t, err)
}

func getFederationMetadataAsPrincipal(
	t *testing.T,
	service *poomsae.Service,
	projectID int64,
	token string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/projects/"+
		strconv.FormatInt(projectID, 10)+"/federation/metadata", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), poomsae.Principal{
		Subject: "user-123", Actor: "Example User",
	}))
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	return response
}

func acquireFederationAccessClaim(
	t *testing.T,
	service *poomsae.Service,
	projectID int64,
	issue string,
	token string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/projects/"+
		strconv.FormatInt(projectID, 10)+"/issues/"+issue+"/lease/actions/acquire",
		bytes.NewBufferString(`{"holder":"worker-1","claim_kind":"timed","ttl_seconds":300}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	return response
}

func createFederationAccessIssue(t *testing.T, service *poomsae.Service, projectID int64) string {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/projects/"+
		strconv.FormatInt(projectID, 10)+"/issues",
		bytes.NewBufferString(`{"actor":"Example Operator","title":"claim fence"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Issue struct {
			ShortID string `json:"short_id"`
		} `json:"issue"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotEmpty(t, body.Issue.ShortID)
	return body.Issue.ShortID
}
