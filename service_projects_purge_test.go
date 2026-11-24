package poomsae_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func purgeTestIssueStatus(t *testing.T, service *poomsae.Service, method string, projectID int64, suffix, body string) int {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method,
		"/api/v1/projects/"+strconv.FormatInt(projectID, 10)+"/issues"+suffix, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	return response.Code
}

// checkServicePurgeArchivedHubProject covers the host workflow after a hub
// project is replaced: archive it, then purge it so its identity and tasks no
// longer occupy the hub.
func checkServicePurgeArchivedHubProject(t *testing.T, service *poomsae.Service) {
	ctx := t.Context()
	spec := poomsae.ProjectSpec{UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "hub-project"}
	ensured, err := service.EnsureProject(ctx, spec)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, purgeTestIssueStatus(t, service, http.MethodPost,
		ensured.Project.ID, "", `{"title":"held task","actor":"user-a"}`))
	// Creating an enrollment makes the project a federation hub.
	enrollment, err := service.CreateFederationEnrollment(ctx, poomsae.FederationEnrollmentSpec{
		ProjectUID: spec.UID, SpokeInstanceUID: "01HZNQ7VFPK1XGD8R5MABCD4EA",
		Capabilities: "pull,push", Actor: "Example Operator",
	})
	require.NoError(t, err)
	assertFederationTokenStatus(t, service, ensured.Project.ID, enrollment.Token, http.StatusOK)

	_, err = service.PurgeArchivedProject(ctx, spec.UID, "Example Operator", "replaced")
	require.ErrorIs(t, err, poomsae.ErrProjectNotArchived, "an active project is refused")
	_, err = service.PurgeArchivedProject(ctx, "01HZNQ7VFPK1XGD8R5MABCD4EY", "Example Operator", "")
	require.ErrorIs(t, err, poomsae.ErrProjectNotFound)

	_, err = service.ArchiveProject(ctx, spec.UID, "Example Operator")
	require.NoError(t, err)
	// The project is now purgeable, so only the missing actor refuses it.
	_, err = service.PurgeArchivedProject(ctx, spec.UID, " ", "replaced")
	require.ErrorIs(t, err, poomsae.ErrProjectPurgeActorRequired, "the purge log names who purged")
	purged, err := service.PurgeArchivedProject(ctx, spec.UID, "Example Operator", "replaced")
	require.NoError(t, err)
	assert.Equal(t, spec.UID, purged.Project.UID)
	assert.Equal(t, spec.Name, purged.Project.Name)
	assert.Equal(t, ensured.Project.ID, purged.Project.ID)
	assert.NotEmpty(t, purged.PurgeLogUID)
	assert.EqualValues(t, 1, purged.IssueCount)
	assert.Equal(t, 1, purged.RemovedEnrollments)
	assert.Equal(t, "replaced", purged.Reason)
	assert.False(t, purged.PurgedAt.IsZero())

	// The enrollment is gone with the project, so its token no longer works.
	request := httptest.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/"+strconv.FormatInt(ensured.Project.ID, 10)+"/federation/metadata", nil)
	request.Header.Set("Authorization", "Bearer "+enrollment.Token)
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	assert.NotEqual(t, http.StatusOK, response.Code)
	_, err = service.ListFederationEnrollments(ctx, spec.UID)
	require.ErrorIs(t, err, poomsae.ErrProjectNotFound)

	_, err = service.PurgeArchivedProject(ctx, spec.UID, "Example Operator", "replaced")
	require.ErrorIs(t, err, poomsae.ErrProjectNotFound, "a repeated purge finds nothing")

	// The identity is free: the host can create the project again from scratch.
	recreated, err := service.EnsureProject(ctx, spec)
	require.NoError(t, err)
	assert.True(t, recreated.Created)
	assert.NotEqual(t, ensured.Project.ID, recreated.Project.ID)
	assert.Equal(t, http.StatusOK, purgeTestIssueStatus(t, service, http.MethodGet, recreated.Project.ID, "", ""))
}

func TestServicePurgeArchivedHubProject(t *testing.T) {
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:  filepath.Join(t.TempDir(), "service.db"),
		Auth: poomsae.AuthConfig{TrustCallerAuthentication: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	checkServicePurgeArchivedHubProject(t, service)
}

func TestServicePostgresPurgeArchivedHubProject(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	dsn, cleanup := testenv.NewPostgresContainer(t, ctx)
	t.Cleanup(cleanup)
	service, err := poomsae.New(ctx, poomsae.Config{
		DSN: dsn, Auth: poomsae.AuthConfig{TrustCallerAuthentication: true},
		Postgres: poomsae.PostgresConfig{Schema: "poomsae", SchemaMode: poomsae.PostgresSchemaBootstrap},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	checkServicePurgeArchivedHubProject(t, service)
}
