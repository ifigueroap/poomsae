package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/client"
	"github.com/ifigueroap/poomsae/internal/config"
	poomsaeclient "github.com/ifigueroap/poomsae/pkg/client"
	"github.com/ifigueroap/poomsae/pkg/client/generated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kitdaemon "go.kenn.io/kit/daemon"
)

func TestMCPRuntimeDirectorySelectsAttachedDaemon(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	t.Setenv("POOMSAE_SERVER", "http://127.0.0.1:1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ping" {
			_, _ = w.Write([]byte(`{"ok":true,"service":"poomsae","version":"test"}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	directory := t.TempDir()
	record := kitdaemon.NewRuntimeRecord("poomsae", "test", kitdaemon.Endpoint{Network: "tcp", Address: strings.TrimPrefix(server.URL, "http://")})
	_, err := (kitdaemon.RuntimeStore{Dir: directory}).Write(record)
	require.NoError(t, err)
	resolved, err := resolveMCPRuntime(t.Context(), directory)
	require.NoError(t, err)
	hc, err := httpClientForResolved(t.Context(), resolved)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, resolved.BaseURL+"/selected", nil)
	require.NoError(t, err)
	response, err := hc.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	assert.Equal(t, http.StatusAccepted, response.StatusCode)
}

func TestMCPRuntimeDirectoriesKeepTelemetryOptOutSeparate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("POOMSAE_HOME", home)
	t.Setenv("POOMSAE_SERVER", "")
	var gate client.TelemetryGate
	var posts atomic.Int32
	for _, name := range []string{"runtime-a", "runtime-b", ""} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/v1/ping" {
				_, _ = w.Write([]byte(`{"ok":true,"service":"poomsae","version":"test"}`))
				return
			}
			posts.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"disabled"}`))
		}))
		t.Cleanup(server.Close)
		directory := filepath.Join(home, name)
		if name == "" {
			var err error
			directory, err = config.RuntimeDir()
			require.NoError(t, err)
		}
		record := kitdaemon.NewRuntimeRecord("poomsae", "test", kitdaemon.Endpoint{Network: "tcp", Address: strings.TrimPrefix(server.URL, "http://")})
		_, err := (kitdaemon.RuntimeStore{Dir: directory}).Write(record)
		require.NoError(t, err)
		resolved, err := resolveMCPRuntime(t.Context(), directory)
		require.NoError(t, err)
		if name == "" {
			require.Equal(t, client.InspectTelemetryTarget(t.Context(), t.TempDir(), ""), client.TelemetryTargetKey(resolved), "explicit default runtime shares the regular CLI/TUI opt-out key")
		}
		hc, err := httpClientForResolved(t.Context(), resolved)
		require.NoError(t, err)
		api, err := poomsaeclient.NewWithHTTPClient(resolved.BaseURL, hc)
		require.NoError(t, err)
		_, err = gate.Report(t.Context(), client.TelemetryTargetKey(resolved), client.TelemetryDayMarkerPath(resolved, "agent-full-"), time.Now().UTC().Format(time.DateOnly), true, func(ctx context.Context) (*generated.CaptureTelemetryEventResponse, error) {
			return api.CaptureTelemetryEvent(ctx, &generated.CaptureTelemetryEventRequestOptions{Body: &generated.CaptureTelemetryEventBody{Event: "agent_active"}})
		})
		require.NoError(t, err)
	}
	require.EqualValues(t, 3, posts.Load(), "opting out one runtime must leave the others reportable")
}
