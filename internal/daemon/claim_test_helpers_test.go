package daemon_test

import (
	"encoding/json/jsontext"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/ifigueroap/poomsae/internal/hooks"
	"github.com/ifigueroap/poomsae/internal/testenv"
)

type claimResponseBody struct {
	Granted    bool           `json:"granted"`
	Pending    bool           `json:"pending"`
	RequestUID string         `json:"request_uid"`
	Holder     claimPrincipal `json:"holder"`
	Claim      *claimOut      `json:"claim"`
	Event      *struct {
		Type    string `json:"type"`
		Actor   string `json:"actor"`
		Payload string `json:"payload"`
	} `json:"event,omitempty"`
}

type claimStatusBody struct {
	Held   bool           `json:"held"`
	Holder claimPrincipal `json:"holder"`
	Claim  *claimOut      `json:"claim"`
	HubNow time.Time      `json:"hub_now"`
}

type claimPrincipal struct {
	HolderInstanceUID string `json:"holder_instance_uid"`
	Holder            string `json:"holder"`
	ClientKind        string `json:"client_kind"`
}

type claimOut struct {
	ClaimUID          string     `json:"claim_uid"`
	IssueUID          string     `json:"issue_uid"`
	Holder            string     `json:"holder"`
	HolderInstanceUID string     `json:"holder_instance_uid"`
	ClientKind        string     `json:"client_kind"`
	Purpose           string     `json:"purpose"`
	ClaimKind         string     `json:"claim_kind"`
	ExpiresAt         *time.Time `json:"expires_at"`
	ReleasedAt        *time.Time `json:"released_at"`
	ReleaseReason     *string    `json:"release_reason"`
	Revision          int64      `json:"revision"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type recordingSink struct {
	mu     sync.Mutex
	events []db.Event
}

func (r *recordingSink) Enqueue(evt db.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, evt)
}

func (r *recordingSink) snapshot() []db.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]db.Event, len(r.events))
	copy(out, r.events)
	return out
}

var _ hooks.Sink = (*recordingSink)(nil)

func createFederatedHubProject(t *testing.T, env *testenv.Env, name string) db.Project {
	t.Helper()
	project, err := env.DB.CreateProject(t.Context(), name)
	require.NoError(t, err)
	_, err = env.DB.EnableProjectFederation(t.Context(), project.ID, "tester")
	require.NoError(t, err)
	return project
}

func createClaimHubIssue(t *testing.T, env *testenv.Env) (db.Project, db.Issue) {
	t.Helper()
	return createClaimHubIssueNamed(t, env, "hub")
}

func createClaimHubIssueNamed(t *testing.T, env *testenv.Env, name string) (db.Project, db.Issue) {
	t.Helper()
	project := createFederatedHubProject(t, env, name)
	issue, _, err := env.DB.CreateIssue(t.Context(), db.CreateIssueParams{
		ProjectID: project.ID,
		Title:     "claim target",
		Author:    "tester",
	})
	require.NoError(t, err)
	return project, issue
}

func createClaimHubIssueInDB(t *testing.T, store *sqlitestore.Store) (db.Project, db.Issue) {
	t.Helper()
	project, err := store.CreateProject(t.Context(), "hub")
	require.NoError(t, err)
	_, err = store.EnableProjectFederation(t.Context(), project.ID, "tester")
	require.NoError(t, err)
	issue, _, err := store.CreateIssue(t.Context(), db.CreateIssueParams{
		ProjectID: project.ID,
		Title:     "claim target",
		Author:    "tester",
	})
	require.NoError(t, err)
	return project, issue
}

func createClaimEnrollment(
	t *testing.T,
	env *testenv.Env,
	projectID int64,
	spokeUID string,
	capabilities string,
) db.CreatedFederationEnrollment {
	t.Helper()
	created, err := env.DB.CreateFederationEnrollment(t.Context(), db.CreateFederationEnrollmentParams{
		Token:            spokeUID + "-" + capabilities + "-token",
		SpokeInstanceUID: spokeUID,
		ProjectID:        &projectID,
		Capabilities:     capabilities,
		Actor:            "tester",
	})
	require.NoError(t, err)
	return created
}

func createClaimForwardingPair(
	t *testing.T,
	capabilities string,
	hubOptions ...testenv.Option,
) (*testenv.Env, *testenv.Env, db.Project, db.Project, db.Issue, string) {
	t.Helper()
	ctx := t.Context()
	hub := testenv.New(t, hubOptions...)
	hubProject, issue := createClaimHubIssue(t, hub)
	spoke := testenv.New(t)
	spokeProject, err := spoke.DB.CreateProjectWithUID(ctx, hubProject.Name, hubProject.UID)
	require.NoError(t, err)
	_, err = spoke.DB.UpsertFederationBinding(ctx, db.FederationBinding{
		ProjectID:            spokeProject.ID,
		Role:                 db.FederationRoleSpoke,
		HubURL:               hub.URL,
		HubProjectID:         hubProject.ID,
		HubProjectUID:        hubProject.UID,
		ReplayHorizonEventID: 1,
		Actor:                "tester",
		Enabled:              true,
		PushEnabled:          true,
	})
	require.NoError(t, err)
	hubCreated, err := hub.DB.EventsAfter(ctx, db.EventsAfterParams{
		ProjectID: hubProject.ID, IssueUID: issue.UID, Types: []string{"issue.created"}, Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, hubCreated, 1)
	created := hubCreated[0]
	inserted, err := spoke.DB.InsertRemoteEvent(ctx, spokeProject.ID, db.RemoteEvent{
		EventUID: created.UID, OriginInstanceUID: created.OriginInstanceUID,
		ProjectUID: created.ProjectUID, ProjectName: created.ProjectName,
		IssueUID: created.IssueUID, RelatedIssueUID: created.RelatedIssueUID,
		Type: created.Type, Actor: created.Actor,
		HLCPhysicalMS: created.HLCPhysicalMS, HLCCounter: created.HLCCounter,
		ContentHash: created.ContentHash, Payload: jsontext.Value(created.Payload), CreatedAt: created.CreatedAt,
	})
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, spoke.DB.MaterializeFederatedProject(ctx, spokeProject.ID))
	enrollment := createClaimEnrollment(t, hub, hubProject.ID, spoke.DB.InstanceUID(), capabilities)
	return hub, spoke, hubProject, spokeProject, issue, enrollment.Token
}

func claimActionPath(projectID int64, ref, action string) string {
	if action == "claim" {
		action = "acquire"
	}
	return issuePathRef(projectID, ref, "lease/actions/"+action)
}

func setupClaimGateIssue(t *testing.T, federated bool) (*testenv.Env, db.Project, db.Issue) {
	t.Helper()
	env := testenv.New(t)
	project, issue, _ := setupClaimGateProject(t, env, federated)
	return env, project, issue
}

func setupClaimGateProject(t *testing.T, env *testenv.Env, federated bool) (db.Project, db.Issue, db.Issue) {
	t.Helper()
	project, err := env.DB.CreateProject(t.Context(), "claim-gate-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	require.NoError(t, err)
	issue, _, err := env.DB.CreateIssue(t.Context(), db.CreateIssueParams{
		ProjectID: project.ID,
		Title:     "claim target",
		Author:    "tester",
	})
	require.NoError(t, err)
	peer, _, err := env.DB.CreateIssue(t.Context(), db.CreateIssueParams{
		ProjectID: project.ID,
		Title:     "claim peer",
		Author:    "tester",
	})
	require.NoError(t, err)
	if federated {
		_, err = env.DB.EnableProjectFederation(t.Context(), project.ID, "tester")
		require.NoError(t, err)
	}
	return project, issue, peer
}

func claimGatePrincipal(env *testenv.Env, actor string) db.ClaimPrincipal {
	return db.ClaimPrincipal{
		HolderInstanceUID: env.DB.InstanceUID(),
		Holder:            actor,
	}
}

func acquireClaimGateIssue(t *testing.T, env *testenv.Env, project db.Project, issue db.Issue, actor string) {
	t.Helper()
	_, err := env.DB.AcquireClaim(t.Context(), db.AcquireClaimParams{
		ProjectID: project.ID,
		IssueRef:  issue.ShortID,
		Principal: claimGatePrincipal(env, actor),
		ClaimKind: "hard",
		Now:       time.Now().UTC(),
	})
	require.NoError(t, err)
}

func createShowClaimSpokeProject(t *testing.T, env *testenv.Env) (db.Project, db.Issue) {
	t.Helper()
	project, err := env.DB.CreateProject(t.Context(), "spoke")
	require.NoError(t, err)
	issue, _, err := env.DB.CreateIssue(t.Context(), db.CreateIssueParams{
		ProjectID: project.ID,
		Title:     "spoke issue",
		Author:    "tester",
	})
	require.NoError(t, err)
	return project, issue
}

func writeShowClaimSpokeBinding(
	t *testing.T,
	env *testenv.Env,
	project db.Project,
	hubURL string,
	token string,
	capabilities string,
) {
	t.Helper()
	_, err := env.DB.UpsertFederationBinding(t.Context(), db.FederationBinding{
		ProjectID:     project.ID,
		Role:          db.FederationRoleSpoke,
		HubURL:        hubURL,
		HubProjectID:  42,
		HubProjectUID: "01HZNQ7VFPK1XGD8R5MABCD4AA",
		Enabled:       true,
	})
	require.NoError(t, err)
	require.NoError(t, config.WriteFederationCredential(project.UID, config.FederationCredential{
		HubURL:       hubURL,
		HubProjectID: 42,
		Token:        token,
		Capabilities: capabilities,
	}))
}
