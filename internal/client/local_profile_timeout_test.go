package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/daemonruntime"
	"github.com/stretchr/testify/require"
	kitdaemon "go.kenn.io/kit/daemon"
)

func TestLocalProfileIdentityProbeHonorsTimeout(t *testing.T) {
	t.Setenv("POOMSAE_HTTP_TIMEOUT", "50ms")
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer s.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- verifyLocalProfileInstance(ctx, ResolvedDaemon{BaseURL: s.URL, LocalProfile: &LocalProfileIdentity{InstanceUID: "01HZZZZZZZZZZZZZZZZZZZZZ01"}})
	}()
	select {
	case <-result:
	// Under DefaultHTTPTimeout, so only the 50ms setting returns the probe in time.
	case <-time.After(2 * time.Second):
		t.Fatal("profile identity probe exceeded configured request timeout")
	}
}

func TestNamedReadOnlyProfileDiscoveryDefersIdentityProbe(t *testing.T) {
	profile, _ := localProfileFixture(t)
	ns, err := daemonruntime.NewNamespaceForHome(profile.Home, profile.StorageID)
	require.NoError(t, err)
	require.NoError(t, ns.EnsureDirs())

	var identityCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/ping":
			_, _ = fmt.Fprint(w, `{"ok":true,"service":"poomsae","version":"test"}`)
		case "/api/v1/instance":
			identityCalls.Add(1)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	_, err = (kitdaemon.RuntimeStore{Dir: ns.DataDir}).Write(kitdaemon.RuntimeRecord{
		PID: os.Getpid(), Network: "tcp", Address: strings.TrimPrefix(server.URL, "http://"),
	})
	require.NoError(t, err)
	t.Setenv("POOMSAE_HTTP_TIMEOUT", "50ms")

	resolved, _, err := DiscoverSelected(t.Context(), DiscoverRequest{Name: "work", ReadOnly: true})
	require.NoError(t, err, "read-only named discovery defers profile identity checks to the caller's bounded client")
	require.Equal(t, profile.InstanceUID, resolved.LocalProfile.InstanceUID)
	require.Equal(t, server.URL, resolved.BaseURL)
	require.Zero(t, identityCalls.Load(), "discovery must not make an identity request with POOMSAE_HTTP_TIMEOUT")
}

func TestLocalProfileStorageProbeRejectsPostgresWithoutLeakingSecrets(t *testing.T) {
	profile := config.LocalProfileConfig{DSN: "postgres://profile_test:synthetic-secret@127.0.0.1:1/example_db?sslmode=disable", Config: &config.DaemonConfig{}} //nolint:gosec // synthetic secret; the test asserts it does not leak
	_, err := InspectLocalProfileStorage(t.Context(), profile)
	require.ErrorIs(t, err, ErrProfileStorageUnavailable)
	require.Contains(t, err.Error(), "not available")
	require.NotContains(t, err.Error(), "synthetic-secret")
}

func TestLocalProfileIdentityProbeDistinguishesAuthenticationFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "response-body-secret", status)
			}))
			t.Cleanup(server.Close)
			err := verifyLocalProfileInstance(t.Context(), ResolvedDaemon{BaseURL: server.URL, LocalProfile: &LocalProfileIdentity{InstanceUID: "01HZZZZZZZZZZZZZZZZZZZZZ01"}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "response-body-secret")
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				require.Contains(t, err.Error(), "authentication failed")
			} else {
				require.NotContains(t, err.Error(), "authentication failed")
				require.Contains(t, err.Error(), "identity unavailable")
			}
		})
	}
}
