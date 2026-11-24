package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ifigueroap/poomsae/internal/db"
)

// TestSelfAuthenticatedRouteMatcherMatchesOnlyTransportRoutes uses literal
// requests and expectations independent of the rule and route tables that
// generate the matcher. Removing or misrouting any self-authenticating
// operation, or broadening a route to the wrong method, makes this test fail.

func TestSelfAuthenticatedRouteMatcherZeroValueMatchesNothing(t *testing.T) {
	var matcher selfAuthenticatedRouteMatcher
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/projects/1/federation/events", nil)
	assert.False(t, matcher.matches(request))
}

// TestBearerMiddlewareRequiresExactMethodForSelfAuthenticatedGETRoutes catches
// net/http's GET-pattern behavior broadening the bypass to HEAD. The deleted
// parsers required exact GET, so HEAD must continue to require the daemon token.
func TestBearerMiddlewareRequiresExactMethodForSelfAuthenticatedGETRoutes(t *testing.T) {
	srv := NewServer(ServerConfig{})
	t.Cleanup(func() { _ = srv.Close() })
	policy := authPolicy{
		Token: "daemon-token",
		SelfAuthenticatedRoutes: newSelfAuthenticatedRouteMatcher(
			selfAuthenticatedRoutes(srv.API().OpenAPI())),
	}
	handler := requireBearer(policy)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }))

	for _, tc := range []struct {
		name string
		path string
	}{
		{"lease status", "/api/v1/projects/7/issues/abc4/lease"},
		{"federation poll", "/api/v1/projects/7/federation/events"},
		{"federation metadata", "/api/v1/projects/7/federation/metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodHead, tc.path, nil))
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}

// TestBearerMiddlewareRequiresTokenForUnusableProjectID pins the guard the
// deleted digit scans provided: a handler can only authenticate a request
// against a project it can identify, so an unusable {project_id} keeps
// requiring the daemon's bearer token.

// TestHostAccessRejectsFederationBearerOnForceReleaseOnly is the HTTP-level
// half of the force-release asymmetry: the bearer middleware defers to the
// handler for both routes, but the host-access middleware only dispatches an
// unattributed bearer for routes that accept an enrollment credential.

type allowAllHostAccess struct{}

func (allowAllHostAccess) Authorize(
	context.Context,
	HostAccessRequest,
) (HostAccessDecision, error) {
	return HostAccessDecision{
		Revalidate:       func(context.Context) error { return nil },
		TransactionFence: func(context.Context, db.Transaction) error { return nil },
	}, nil
}
