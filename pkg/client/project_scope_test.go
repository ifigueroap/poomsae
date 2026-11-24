package client_test

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifigueroap/poomsae/internal/projectscope"
	poomsaeclient "github.com/ifigueroap/poomsae/pkg/client"
	"github.com/stretchr/testify/require"
)

func TestProjectScopeClientIsImmutableAndPreservesStackedLimits(t *testing.T) {
	type capture struct {
		scopes []string
		auth   string
	}
	requests := make(chan capture, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- capture{scopes: r.Header.Values(projectscope.Header), auth: r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":[]}`))
	}))
	t.Cleanup(server.Close)
	client, err := poomsaeclient.NewWithHTTPClient(server.URL, server.Client(), poomsaeclient.WithRequestEditor(func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer example-token")
		return nil
	}))
	require.NoError(t, err)
	source, target := "01HAAAAAAAAAAAAAAAAAAAAAAA", "01HBBBBBBBBBBBBBBBBBBBBBBB"
	refs := []string{source}
	scoped, err := client.WithProjectScope(refs)
	require.NoError(t, err)
	refs[0] = target
	stacked, err := scoped.WithProjectScope([]string{target})
	require.NoError(t, err)
	for _, c := range []*poomsaeclient.Client{client, scoped, stacked} {
		_, err := c.ListProjects(t.Context(), nil)
		require.NoError(t, err)
	}
	observed := make([][]string, 0, 3)
	for range 3 {
		request := <-requests
		require.Equal(t, "Bearer example-token", request.auth)
		observed = append(observed, request.scopes)
	}
	require.Empty(t, observed[0])
	require.Len(t, observed[1], 1)
	require.Len(t, observed[2], 2)
	for i, want := range []string{source, target} {
		var got []string
		require.NoError(t, json.Unmarshal([]byte(observed[2][i]), &got))
		require.Equal(t, []string{want}, got)
	}
	require.Equal(t, observed[1][0], observed[2][0], "a later binding cannot replace the original narrowing layer")
}
