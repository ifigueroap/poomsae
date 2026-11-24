package poomsae_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingAccessController struct {
	mu                   sync.Mutex
	requests             []poomsae.AccessRequest
	err                  error
	lease                poomsae.AccessLease
	transactionFence     poomsae.TransactionFence
	omitTransactionFence bool
	authorize            func(poomsae.AccessRequest) error
}

func (c *recordingAccessController) Authorize(
	_ context.Context,
	request poomsae.AccessRequest,
) (poomsae.AccessDecision, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, request)
	decision := poomsae.AccessDecision{}
	decision.TransactionFence = c.transactionFence
	if decision.TransactionFence == nil && !c.omitTransactionFence {
		decision.TransactionFence = func(context.Context, poomsae.Transaction) error { return nil }
	}
	if request.Operation.ID == "streamEvents" {
		decision.Lease = c.lease
	}
	if c.authorize != nil {
		return decision, c.authorize(request)
	}
	return decision, c.err
}

func TestServiceMutationFailsClosedWithoutTransactionFence(t *testing.T) {
	controller := &recordingAccessController{omitTransactionFence: true}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	project, err := service.EnsureProject(t.Context(), poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "example-project",
	})
	require.NoError(t, err)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/projects/"+
		strconv.FormatInt(project.Project.ID, 10)+"/issues",
		bytes.NewBufferString(`{"actor":"ignored","title":"must not be stored"}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), poomsae.Principal{
		Subject: "user-123", Actor: "Example User",
	}))
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t,
		`{"status":503,"error":{"code":"access_unavailable","message":"transaction access decision unavailable"}}`,
		response.Body.String())

	listRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/projects/"+
		strconv.FormatInt(project.Project.ID, 10)+"/issues", nil)
	listRequest = listRequest.WithContext(poomsae.WithPrincipal(listRequest.Context(), poomsae.Principal{
		Subject: "user-123", Actor: "Example User",
	}))
	listed := httptest.NewRecorder()
	service.Handler().ServeHTTP(listed, listRequest)
	require.Equal(t, http.StatusOK, listed.Code)
	assert.NotContains(t, listed.Body.String(), "must not be stored")
}

func TestServiceTransactionFenceSharesMutationCommitAndRollback(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "service.db")
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var attempts atomic.Int64
	controller := &recordingAccessController{}
	controller.transactionFence = func(ctx context.Context, tx poomsae.Transaction) error {
		attempt := attempts.Add(1)
		_, err := tx.ExecContext(ctx, `INSERT INTO fence_markers(attempt) VALUES(?)`, attempt)
		if err != nil {
			return err
		}
		if attempt == 1 {
			entered <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return poomsae.ErrAccessDenied
	}
	service, err := poomsae.New(t.Context(), poomsae.Config{DSN: databasePath, Access: controller})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	project, err := service.EnsureProject(t.Context(), poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "example-project",
	})
	require.NoError(t, err)

	inspection, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, inspection.Close()) })
	_, err = inspection.ExecContext(t.Context(), `CREATE TABLE fence_markers (attempt INTEGER NOT NULL)`)
	require.NoError(t, err)

	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.Handler().ServeHTTP(w, r.WithContext(poomsae.WithPrincipal(r.Context(), principal)))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	create := func(title string) (*http.Response, error) {
		body := bytes.NewBufferString(`{"actor":"ignored","title":` + strconv.Quote(title) + `}`)
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/projects/"+
			strconv.FormatInt(project.Project.ID, 10)+"/issues", body)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		return http.DefaultClient.Do(request)
	}

	firstResponse := make(chan *http.Response, 1)
	firstError := make(chan error, 1)
	go func() {
		response, requestErr := create("committed after fence")
		firstResponse <- response
		firstError <- requestErr
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "transaction fence was not reached")
	}
	var issuesBeforeRelease int
	require.NoError(t, inspection.QueryRowContext(t.Context(),
		`SELECT count(*) FROM issues WHERE title = ?`, "committed after fence").Scan(&issuesBeforeRelease))
	assert.Zero(t, issuesBeforeRelease, "domain write must not run before the transaction fence")
	close(release)
	require.NoError(t, <-firstError)
	committed := <-firstResponse
	require.NotNil(t, committed)
	defer func() { _ = committed.Body.Close() }()
	require.Equal(t, http.StatusOK, committed.StatusCode)

	rejected, err := create("rolled back with fence")
	require.NoError(t, err)
	defer func() { _ = rejected.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, rejected.StatusCode)

	var markerCount, committedIssues, rejectedIssues int
	require.NoError(t, inspection.QueryRowContext(t.Context(), `SELECT count(*) FROM fence_markers`).Scan(&markerCount))
	require.NoError(t, inspection.QueryRowContext(t.Context(),
		`SELECT count(*) FROM issues WHERE title = ?`, "committed after fence").Scan(&committedIssues))
	require.NoError(t, inspection.QueryRowContext(t.Context(),
		`SELECT count(*) FROM issues WHERE title = ?`, "rolled back with fence").Scan(&rejectedIssues))
	assert.Equal(t, 1, markerCount, "rejected fence work must roll back with the domain transaction")
	assert.Equal(t, 1, committedIssues)
	assert.Zero(t, rejectedIssues)
}

type revocableAccessLease struct {
	revoked atomic.Bool
}

func (l *revocableAccessLease) Revalidate(context.Context) error {
	if l.revoked.Load() {
		return poomsae.ErrAccessDenied
	}
	return nil
}

func (c *recordingAccessController) snapshot() []poomsae.AccessRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]poomsae.AccessRequest(nil), c.requests...)
}

func TestServiceAccessControllerRequiresAnInProcessPrincipal(t *testing.T) {
	controller := &recordingAccessController{}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:    filepath.Join(t.TempDir(), "service.db"),
		Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/projects", nil))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.JSONEq(t, `{"status":401,"error":{"code":"authentication_required","message":"authentication required"}}`,
		response.Body.String())
	assert.Empty(t, controller.snapshot(), "authorization must not run without an authenticated principal")
}

func TestServiceAccessControllerDeniesWithoutDisclosingProjectData(t *testing.T) {
	controller := &recordingAccessController{err: poomsae.ErrAccessDenied}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:    filepath.Join(t.TempDir(), "service.db"),
		Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/projects/42", nil)
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), poomsae.Principal{
		Subject: "user-123",
		Actor:   "Example User",
	}))
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.JSONEq(t, `{"status":404,"error":{"code":"not_found","message":"resource not found"}}`,
		response.Body.String())
	require.Len(t, controller.snapshot(), 1)
	assert.Equal(t, poomsae.Operation{
		ID:         "showProject",
		Method:     http.MethodGet,
		Path:       "/api/v1/projects/{project_id}",
		PathParams: map[string]string{"project_id": "42"},
		Policy: poomsae.OperationPolicy{
			Kind: poomsae.OperationProjectRead, Capability: poomsae.CapabilityRead,
		},
		ProjectIDs: []int64{42},
	}, controller.snapshot()[0].Operation)
}

func TestNewRejectsUnknownEmbeddingProfile(t *testing.T) {
	_, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:     filepath.Join(t.TempDir(), "service.db"),
		Auth:    poomsae.AuthConfig{TrustCallerAuthentication: true},
		Profile: poomsae.EmbeddingProfile("unknown"),
	})
	require.EqualError(t, err, `poomsae: unknown embedding profile "unknown"`)
}

func TestServiceAccessControllerSuppliesTheMutationActor(t *testing.T) {
	controller := &recordingAccessController{}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:    filepath.Join(t.TempDir(), "service.db"),
		Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.Handler().ServeHTTP(w, r.WithContext(poomsae.WithPrincipal(r.Context(), principal)))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	project := createProject(t, server.URL, "example-project")
	require.NotZero(t, project.Project.ID)

	body, err := json.Marshal(map[string]string{
		"actor": "caller supplied actor",
		"title": "Host-authorized change",
	})
	require.NoError(t, err)
	req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/projects/"+strconv.FormatInt(project.Project.ID, 10)+"/issues", bytes.NewReader(body))
	require.NoError(t, reqErr)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)

	var created struct {
		Issue struct {
			Author string `json:"author"`
		} `json:"issue"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	assert.Equal(t, "Example User", created.Issue.Author)

	requests := controller.snapshot()
	require.GreaterOrEqual(t, len(requests), 2)
	issueRequest := requests[len(requests)-1]
	assert.Equal(t, principal, issueRequest.Principal)
	assert.Equal(t, "createIssue", issueRequest.Operation.ID)
	assert.Equal(t, strconv.FormatInt(project.Project.ID, 10), issueRequest.Operation.PathParams["project_id"])
}

func TestServiceAccessControllerKeepsUnexpectedFailuresDistinctFromDenials(t *testing.T) {
	controller := &recordingAccessController{err: errors.New("policy store unavailable")}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:    filepath.Join(t.TempDir(), "service.db"),
		Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/projects", nil)
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), poomsae.Principal{
		Subject: "user-123",
		Actor:   "Example User",
	}))
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"status":503,"error":{"code":"access_unavailable","message":"access decision unavailable"}}`,
		response.Body.String())
}

func TestServiceAccessLeaseStopsAStreamBeforeTheNextEvent(t *testing.T) {
	lease := &revocableAccessLease{}
	controller := &recordingAccessController{lease: lease}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:    filepath.Join(t.TempDir(), "service.db"),
		Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.Handler().ServeHTTP(w, r.WithContext(poomsae.WithPrincipal(r.Context(), principal)))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	project := createProject(t, server.URL, "example-project")
	streamRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		server.URL+"/api/v1/events/stream?after_id=0&project_id="+
			strconv.FormatInt(project.Project.ID, 10), nil)
	require.NoError(t, err)
	streamRequest.Header.Set("Accept", "text/event-stream")
	streamResponse, err := server.Client().Do(streamRequest)
	require.NoError(t, err)
	defer func() { _ = streamResponse.Body.Close() }()
	require.Equal(t, http.StatusOK, streamResponse.StatusCode)

	reader := bufio.NewReader(streamResponse.Body)
	connected, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, ": connected\n", connected)
	_, err = reader.ReadString('\n')
	require.NoError(t, err)

	lease.revoked.Store(true)
	body := bytes.NewBufferString(`{"actor":"untrusted","title":"must not reach revoked stream"}`)
	req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/projects/"+
		strconv.FormatInt(project.Project.ID, 10)+"/issues", body)
	require.NoError(t, reqErr)
	req.Header.Set("Content-Type", "application/json")
	mutation, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = mutation.Body.Close() }()
	require.Equal(t, http.StatusOK, mutation.StatusCode)

	streamResult := make(chan error, 1)
	go func() {
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				streamResult <- nil
				return
			}
			if line == "event: issue.created\n" {
				streamResult <- errors.New("revoked stream received issue.created")
				return
			}
		}
	}()
	select {
	case streamErr := <-streamResult:
		require.NoError(t, streamErr)
	case <-time.After(2 * time.Second):
		require.Fail(t, "revoked event stream did not close")
	}
}

func TestServiceAccessControllerRequiresALeaseForEventStreams(t *testing.T) {
	controller := &recordingAccessController{}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:    filepath.Join(t.TempDir(), "service.db"),
		Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/events/stream?after_id=0", nil)
	request.Header.Set("Accept", "text/event-stream")
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), poomsae.Principal{
		Subject: "user-123",
		Actor:   "Example User",
	}))
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.JSONEq(t, `{"status":503,"error":{"code":"access_lease_required","message":"long-lived access lease required"}}`,
		response.Body.String())
}

func TestNewRejectsHostAccessCombinedWithAnotherAuthenticationPolicy(t *testing.T) {
	controller := &recordingAccessController{}
	for _, auth := range []poomsae.AuthConfig{
		{Token: "service-token"},
		{TrustCallerAuthentication: true},
	} {
		service, err := poomsae.New(t.Context(), poomsae.Config{
			DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller, Auth: auth,
		})
		assert.Nil(t, service)
		assert.EqualError(t, err,
			"poomsae: auth token, trusted caller authentication, and host access are mutually exclusive")
	}
}

func TestWithPrincipalDoesNotChangeTrustedCallerMode(t *testing.T) {
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:  filepath.Join(t.TempDir(), "service.db"),
		Auth: poomsae.AuthConfig{TrustCallerAuthentication: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := poomsae.WithPrincipal(r.Context(), poomsae.Principal{
			Subject: "must-be-ignored", Actor: "must-be-ignored",
		})
		service.Handler().ServeHTTP(w, r.WithContext(ctx))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	project := createProject(t, server.URL, "example-project")

	body := bytes.NewBufferString(`{"actor":"request actor","title":"ordinary trusted mutation"}`)
	req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/projects/"+
		strconv.FormatInt(project.Project.ID, 10)+"/issues", body)
	require.NoError(t, reqErr)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)

	var created struct {
		Issue struct {
			Author string `json:"author"`
		} `json:"issue"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	assert.Equal(t, "request actor", created.Issue.Author)
}

func TestServiceAccessControllerProtectsTheOpenAPIDocument(t *testing.T) {
	controller := &recordingAccessController{}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:    filepath.Join(t.TempDir(), "service.db"),
		Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	missing := httptest.NewRecorder()
	service.Handler().ServeHTTP(missing,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/openapi.yaml", nil))
	assert.Equal(t, http.StatusUnauthorized, missing.Code)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/openapi.yaml", nil)
	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), principal))
	allowed := httptest.NewRecorder()
	service.Handler().ServeHTTP(allowed, request)
	require.Equal(t, http.StatusOK, allowed.Code)

	requests := controller.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, principal, requests[0].Principal)
	assert.Equal(t, poomsae.Operation{
		ID: "openAPI", Method: http.MethodGet, Path: "/openapi.yaml",
		PathParams: map[string]string{},
		Policy: poomsae.OperationPolicy{
			Kind: poomsae.OperationServiceRead, Capability: poomsae.CapabilityRead,
		},
	}, requests[0].Operation)
}

func TestServiceAccessControllerAuthorizesBothProjectsBeforeMerge(t *testing.T) {
	controller := &recordingAccessController{}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.Handler().ServeHTTP(w, r.WithContext(poomsae.WithPrincipal(r.Context(), principal)))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	target := createProject(t, server.URL, "target-project")
	source := createProject(t, server.URL, "source-project")
	controller.authorize = func(request poomsae.AccessRequest) error {
		if request.Operation.ID == "mergeProject" &&
			containsInt64(request.Operation.ProjectIDs, source.Project.ID) {
			return poomsae.ErrAccessDenied
		}
		return nil
	}

	body := bytes.NewBufferString(`{"source_project_id":` +
		strconv.FormatInt(source.Project.ID, 10) + `}`)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/projects/"+
		strconv.FormatInt(target.Project.ID, 10)+"/merge", body)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, response.StatusCode)

	requests := controller.snapshot()
	mergeRequest := requests[len(requests)-1]
	assert.Equal(t, "mergeProject", mergeRequest.Operation.ID)
	assert.ElementsMatch(t, []int64{target.Project.ID, source.Project.ID}, mergeRequest.Operation.ProjectIDs)
}

func TestServiceAccessControllerAuthorizesMoveDestination(t *testing.T) {
	controller := &recordingAccessController{}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.Handler().ServeHTTP(w, r.WithContext(poomsae.WithPrincipal(r.Context(), principal)))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	source := createProject(t, server.URL, "source-project")
	target := createProject(t, server.URL, "target-project")
	issueBody := bytes.NewBufferString(`{"actor":"ignored","title":"stays in source"}`)
	req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/projects/"+
		strconv.FormatInt(source.Project.ID, 10)+"/issues", issueBody)
	require.NoError(t, reqErr)
	req.Header.Set("Content-Type", "application/json")
	issueResponse, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = issueResponse.Body.Close() }()
	require.Equal(t, http.StatusOK, issueResponse.StatusCode)
	var created struct {
		Issue struct {
			ShortID  string `json:"short_id"`
			Revision int64  `json:"revision"`
		} `json:"issue"`
	}
	require.NoError(t, json.NewDecoder(issueResponse.Body).Decode(&created))

	controller.authorize = func(request poomsae.AccessRequest) error {
		if request.Operation.ID == "moveIssue" &&
			containsString(request.Operation.ProjectUIDs, target.Project.UID) {
			return poomsae.ErrAccessDenied
		}
		return nil
	}
	moveBody := bytes.NewBufferString(`{"actor":"ignored","to_project_uid":"` + target.Project.UID + `"}`)
	moveRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/projects/"+
		strconv.FormatInt(source.Project.ID, 10)+"/issues/"+created.Issue.ShortID+"/actions/move", moveBody)
	require.NoError(t, err)
	moveRequest.Header.Set("Content-Type", "application/json")
	moveRequest.Header.Set("If-Match", `"rev-`+strconv.FormatInt(created.Issue.Revision, 10)+`"`)
	moveResponse, err := server.Client().Do(moveRequest)
	require.NoError(t, err)
	defer func() { _ = moveResponse.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, moveResponse.StatusCode)

	requests := controller.snapshot()
	request := requests[len(requests)-1]
	assert.Equal(t, "moveIssue", request.Operation.ID)
	assert.Equal(t, []int64{source.Project.ID}, request.Operation.ProjectIDs)
	assert.Equal(t, []string{target.Project.UID}, request.Operation.ProjectUIDs)
}

func TestServiceAccessControllerRequiresCompleteEventStreamScope(t *testing.T) {
	controller := &recordingAccessController{lease: &revocableAccessLease{}}
	controller.authorize = func(request poomsae.AccessRequest) error {
		if request.Operation.ID == "streamEvents" {
			return poomsae.ErrAccessDenied
		}
		return nil
	}
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.Handler().ServeHTTP(w, r.WithContext(poomsae.WithPrincipal(r.Context(), principal)))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	project := createProject(t, server.URL, "example-project")

	for _, path := range []string{
		"/api/v1/events/stream?project_id=" + strconv.FormatInt(project.Project.ID, 10),
		"/api/v1/events/stream",
	} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
		require.NoError(t, err)
		request.Header.Set("Accept", "text/event-stream")
		response, err := server.Client().Do(request)
		require.NoError(t, err)
		_ = response.Body.Close()
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	}

	requests := controller.snapshot()
	require.GreaterOrEqual(t, len(requests), 3)
	projectStream := requests[len(requests)-2].Operation
	globalStream := requests[len(requests)-1].Operation
	assert.Empty(t, projectStream.ProjectIDs)
	assert.True(t, projectStream.AllProjects)
	assert.Empty(t, globalStream.ProjectIDs)
	assert.True(t, globalStream.AllProjects)
}

func postJSON(t *testing.T, client *http.Client, url, body string, out any) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusOK, response.StatusCode)
	if out != nil {
		require.NoError(t, json.NewDecoder(response.Body).Decode(out))
	}
}

func containsInt64(values []int64, want int64) bool {
	return slices.Contains(values, want)
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func TestServiceTelemetryCaptureIsUnavailableToEmbeddedHosts(t *testing.T) {
	principal := poomsae.Principal{Subject: "user-123", Actor: "Example User"}
	serve := func(service *poomsae.Service) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/ui/telemetry",
			bytes.NewBufferString(`{"event":"app_opened"}`))
		request.Header.Set("Content-Type", "application/json")
		request = request.WithContext(poomsae.WithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()
		service.Handler().ServeHTTP(response, request)
		return response
	}

	t.Run("full profile", func(t *testing.T) {
		controller := &recordingAccessController{}
		service, err := poomsae.New(t.Context(), poomsae.Config{
			DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller,
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, service.Close()) })

		response := serve(service)

		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
		assert.JSONEq(t, `{"status":503,"error":{"code":"telemetry_unavailable","message":"telemetry capture is unavailable"}}`,
			response.Body.String())
		requests := controller.snapshot()
		require.Len(t, requests, 1)
		assert.Equal(t, poomsae.Operation{
			ID: "captureTelemetryEvent", Method: http.MethodPost, Path: "/api/v1/ui/telemetry",
			PathParams: map[string]string{},
			Policy: poomsae.OperationPolicy{
				Kind: poomsae.OperationServiceMutation, Capability: poomsae.CapabilityWrite, Mutation: true,
			},
		}, requests[0].Operation)
		assert.Empty(t, requests[0].Operation.ProjectIDs)
		assert.False(t, requests[0].Operation.AllProjects)
	})

	t.Run("restricted profile", func(t *testing.T) {
		controller := &recordingAccessController{}
		service, err := poomsae.New(t.Context(), poomsae.Config{
			DSN: filepath.Join(t.TempDir(), "service.db"), Access: controller,
			Profile: poomsae.EmbeddingProfileRestricted,
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, service.Close()) })

		response := serve(service)

		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Empty(t, controller.snapshot(), "restricted operations must fail before host authorization")
	})
}
