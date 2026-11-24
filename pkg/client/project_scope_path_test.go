package client_test

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/daemon"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	poomsaeclient "github.com/ifigueroap/poomsae/pkg/client"
	"github.com/ifigueroap/poomsae/pkg/client/generated"
	"github.com/stretchr/testify/require"
)

func TestProjectScopeClientsPreserveQualifiedIssueReferences(t *testing.T) {
	store, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "poomsae.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	project, err := store.CreateProject(t.Context(), "spoke-project")
	require.NoError(t, err)
	issue, _, err := store.CreateIssue(t.Context(), db.CreateIssueParams{ProjectID: project.ID, Title: "Qualified reference", Author: "worker"})
	require.NoError(t, err)
	server := daemon.NewServer(daemon.ServerConfig{DB: store})
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client, err := poomsaeclient.NewWithHTTPClient(httpServer.URL, httpServer.Client())
	require.NoError(t, err)
	scoped, err := client.WithProjectScope([]string{project.UID})
	require.NoError(t, err)
	stacked, err := scoped.WithProjectScope([]string{project.UID})
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		client *poomsaeclient.Client
	}{{"original", client}, {"scoped", scoped}, {"stacked", stacked}} {
		t.Run(tc.name, func(t *testing.T) {
			shown, err := tc.client.ShowIssue(t.Context(), &generated.ShowIssueRequestOptions{PathParams: &generated.ShowIssuePath{ProjectID: project.ID, Ref: project.Name + "#" + issue.ShortID}})
			require.NoError(t, err, "an authorized qualified reference must reach the daemon with its literal # delimiter")
			require.Equal(t, issue.UID, shown.Issue.UID)
		})
	}
}
