package poomsae

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMountedServiceOmitsUnsupportedFederationRebindRoute(t *testing.T) {
	service, err := New(t.Context(), Config{
		DSN:  filepath.Join(t.TempDir(), "service.db"),
		Auth: AuthConfig{TrustCallerAuthentication: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)

	req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/v1/federation/replicas/1/actions/rebind", bytes.NewBufferString(`{"hub_catalog":"primary-hub"}`))
	require.NoError(t, reqErr)
	req.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func serviceFederationStatuses(t *testing.T, service *Service) []api.FederationProjectStatus {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/federation/status", nil)
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var body api.FederationStatusBody
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body.Statuses
}

type serviceFederationCredentialStore struct {
	mu      sync.Mutex
	entries map[string]FederationCredential
}

var _ FederationCredentialStore = (*serviceFederationCredentialStore)(nil)
var _ config.FederationManagedCredentialStore = serviceCredentialStoreAdapter{}

func newServiceFederationCredentialStore() *serviceFederationCredentialStore {
	return &serviceFederationCredentialStore{entries: make(map[string]FederationCredential)}
}

func (s *serviceFederationCredentialStore) FederationCredential(
	_ context.Context, projectUID string,
) (FederationCredential, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, ok := s.entries[projectUID]
	return credential, ok, nil
}

func (s *serviceFederationCredentialStore) StoreFederationCredential(
	_ context.Context, projectUID string, credential FederationCredential,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[projectUID] = credential
	return nil
}

func (s *serviceFederationCredentialStore) DeleteFederationCredential(
	_ context.Context, projectUID string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, projectUID)
	return nil
}

// recordingHookSink is a hooks.Sink that keeps every enqueued event so a test
// can assert which events reached webhook delivery.
type recordingHookSink struct {
	mu     sync.Mutex
	events []db.Event
}

func (s *recordingHookSink) Enqueue(evt db.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, evt)
}

func (s *recordingHookSink) snapshot() []db.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]db.Event(nil), s.events...)
}

// TestServiceRunEnqueuesHooksForFederationPulledEvents pins that a mounted
// service delivers hooks for events it pulled from a hub, matching the
// standalone daemon's federation callback. Before the fix the mounted service
// only broadcast pulled events over SSE, so webhooks never fired for them.
