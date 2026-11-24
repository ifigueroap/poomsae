package mcpserver

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/daemon"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	poomsaeclient "github.com/ifigueroap/poomsae/pkg/client"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func teamAdministrationFixture(t *testing.T) (*sqlitestore.Store, *poomsaeclient.Client) {
	t.Helper()
	store, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "poomsae.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	server := daemon.NewServer(daemon.ServerConfig{DB: store, StartedAt: time.Now().UTC(), Auth: config.AuthConfig{Token: "bootstrap-test-token", RequireTokenIdentity: true}})
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client, err := poomsaeclient.NewWithBearer(t.Context(), httpServer.URL, "bootstrap-test-token")
	require.NoError(t, err)
	return store, client
}

// R9: team/policy MCP operations reuse the native owner capability and API.
func TestTeamsProjectAccessMCP(t *testing.T) {
	store, client := teamAdministrationFixture(t)
	project, err := store.CreateProject(t.Context(), "shared-mcp-project")
	require.NoError(t, err)
	session := connectRawTestServerWithOptions(t, Options{Client: client, Scope: NewAllScope(), EnableTokenAdmin: true, Actor: "owner", Version: "test"})
	callAdministrationTool(t, session, "poomsae.load_teams", map[string]any{})
	created := callAdministrationTool(t, session, "poomsae.team_create", map[string]any{"name": "engineering"})
	uid := created["team"].(map[string]any)["uid"].(string)
	callAdministrationTool(t, session, "poomsae.team_member_set", map[string]any{"team_uid": uid, "member_actor": "member", "present": true})
	members, err := store.TeamMembers(t.Context(), uid)
	require.NoError(t, err)
	require.Contains(t, members, "member")
	callAdministrationTool(t, session, "poomsae.project_access_set", map[string]any{"project": project.Name, "visibility": "teams", "team_uids": []string{uid}})
	policy, err := store.ProjectAccessPolicy(t.Context(), project.UID)
	require.NoError(t, err)
	require.Equal(t, "teams", policy.Visibility)
	require.Equal(t, []string{uid}, policy.TeamUIDs)
	shown := callAdministrationTool(t, session, "poomsae.project_access_show", map[string]any{"project": project.Name})
	require.Equal(t, "teams", shown["policy"].(map[string]any)["visibility"])
	callAdministrationTool(t, session, "poomsae.team_member_set", map[string]any{"team_uid": uid, "member_actor": "member", "present": false})
	callAdministrationTool(t, session, "poomsae.team_delete", map[string]any{"team_uid": uid})
	policy, err = store.ProjectAccessPolicy(t.Context(), project.UID)
	require.NoError(t, err)
	require.Equal(t, "teams", policy.Visibility)
	require.Empty(t, policy.TeamUIDs)
}
func TestTeamsMCPRequiresExplicitDaemonWideCapability(t *testing.T) {
	store, client := teamAdministrationFixture(t)
	project, err := store.CreateProject(t.Context(), "scoped-mcp-project")
	require.NoError(t, err)
	bound, err := NewBoundScope(ProjectIdentity{ID: project.ID, UID: project.UID, Name: project.Name})
	require.NoError(t, err)
	for _, options := range []Options{{Client: client, Scope: NewAllScope(), Actor: "owner", Version: "test"}, {Client: client, Scope: bound, EnableTokenAdmin: true, Actor: "owner", Version: "test"}} {
		session := connectRawTestServerWithOptions(t, options)
		output := callAdministrationTool(t, session, "poomsae.load_teams", map[string]any{})
		require.Equal(t, false, output["available"])
		result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "poomsae.team_create", Arguments: map[string]any{"name": "forbidden"}})
		require.True(t, err != nil || result.IsError, "unavailable administrative tool must not execute")
	}
}
func TestTokenCreateInitialTeamsMCP(t *testing.T) {
	store, client := teamAdministrationFixture(t)
	team, _, err := store.CreateTeam(t.Context(), "engineering", db.BootstrapActor)
	require.NoError(t, err)
	session := connectRawTestServerWithOptions(t, Options{Client: client, Scope: NewAllScope(), EnableTokenAdmin: true, Actor: "owner", Version: "test"})
	callAdministrationTool(t, session, "poomsae.load_tokens", map[string]any{})
	output := callAdministrationTool(t, session, "poomsae.token_create", map[string]any{"token_actor": "member", "team_uids": []string{team.UID}})
	require.NotEmpty(t, output["token"])
	members, err := store.TeamMembers(t.Context(), team.UID)
	require.NoError(t, err)
	require.Contains(t, members, "member")
}
