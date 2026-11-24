package daemon_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/cron"
	"github.com/ifigueroap/poomsae/internal/daemon"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/require"
)

func nativeCronEnrollmentFixture(t *testing.T, options ...testenv.Option) (*testenv.Env, db.Project, db.Issue, cron.JobDefinition) {
	t.Helper()
	env := testenv.New(t, options...)
	project := seedProject(t, env, "hub-project")
	parent, _, err := env.DB.CreateIssue(t.Context(), db.CreateIssueParams{ProjectID: project.ID, Title: "Periodic work", Author: "tester"})
	require.NoError(t, err)
	_, err = env.DB.EnableProjectFederation(t.Context(), project.ID, "tester")
	require.NoError(t, err)
	definition := cron.JobDefinition{Version: 1, Kind: "job", Enabled: true, Trigger: cron.Trigger{Kind: "interval", IntervalSeconds: 7200}, Action: cron.Action{Kind: "notify", Recipient: "current-owner-or-author"}, Issue: &cron.IssuePolicy{Kind: "per-run", ParentUID: parent.UID, Title: "Review service"}, Overlap: "allow", Catchup: "all"}
	return env, project, parent, definition
}

// The push enrollment must reach the real job transaction and retain the
// original progress anchor through replacement, archive, and restore.

// Cron accepts direct scoped push authority only. Every rejection must leave
// the project history untouched, including on an otherwise keyless hub.

type beforeCronJobStore struct {
	db.Storage
	before func() error
}

func (s beforeCronJobStore) PutCronJob(ctx context.Context, in db.PutCronJob) (db.CronJob, []db.Event, error) {
	if err := s.before(); err != nil {
		return db.CronJob{}, nil, err
	}
	return s.Storage.PutCronJob(ctx, in)
}

// Authentication before dispatch cannot outlive a revoked credential or
// removed project membership by the time the job transaction commits.
func TestCronJobWriteRevalidatesAuthority(t *testing.T) {
	for _, kind := range []string{"enrollment", "identity", "membership"} {
		t.Run(kind, func(t *testing.T) {
			env, project, _, definition := nativeCronEnrollmentFixture(t)
			enrollment := createClaimEnrollment(t, env, project.ID, cronHTTPUID(t), "push")
			actorToken, _, err := env.DB.CreateAPIToken(t.Context(), db.CreateAPITokenParams{PlaintextToken: "actor-token", Actor: "tester", AdminActor: "admin"})
			require.NoError(t, err)
			token := enrollment.Token
			if kind == "identity" {
				token = "actor-token"
			}
			store := beforeCronJobStore{Storage: env.DB, before: func() error {
				switch kind {
				case "enrollment":
					return env.DB.RevokeFederationEnrollment(t.Context(), enrollment.Enrollment.ID)
				case "identity":
					_, _, err := env.DB.RevokeAPIToken(t.Context(), actorToken.ID, "admin")
					return err
				default:
					_, _, err := env.DB.SetProjectAccessPolicy(t.Context(), db.ProjectAccessPolicy{ProjectUID: project.UID, Visibility: "teams"}, "admin")
					return err
				}
			}}
			server := daemon.NewServer(daemon.ServerConfig{DB: store, Auth: config.AuthConfig{Token: "owner-token", RequireTokenIdentity: true}})
			t.Cleanup(func() { require.NoError(t, server.Close()) })
			endpoint := httptest.NewServer(server.Handler())
			t.Cleanup(endpoint.Close)
			target := &testenv.Env{URL: endpoint.URL, HTTP: endpoint.Client()}
			response, raw := envDoRaw(t, target, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/cron/jobs", project.ID), map[string]any{"actor": "tester", "name": "Review", "definition": definition}, map[string]string{"Authorization": "Bearer " + token})
			want := http.StatusForbidden
			if kind == "identity" {
				want = http.StatusUnauthorized
			}
			require.Equalf(t, want, response.StatusCode, "%s", raw)
			jobs, err := env.DB.ListCronJobs(t.Context(), db.CronList{ProjectID: project.ID})
			require.NoError(t, err)
			require.Empty(t, jobs)
		})
	}
}

func TestCronKeylessOrdinaryWrite(t *testing.T) {
	env, project, _, definition := nativeCronEnrollmentFixture(t)
	response, raw := envDoRaw(t, env, http.MethodPost, fmt.Sprintf("/api/v1/projects/%d/cron/jobs", project.ID), map[string]any{"actor": "tester", "name": "Review", "definition": definition}, nil)
	require.Equalf(t, http.StatusCreated, response.StatusCode, "%s", raw)
}

// Push enrollment manages native recurring jobs only. An adapter job must
// never be convertible in either direction: replacing it with a native
// definition would irreversibly discard the adapter configuration, and
// replacing a native job with an adapter definition would abandon native
// ownership. Every rejection must leave the definition and history untouched.

// Cron routes resolve credentials inside the handler, after the root
// attribution middleware has already run. Remote enrollment push writes and
// static-token writes (forwarding headers mark the request as proxied, so it
// is not owner-local) must still commit root receipts, or the hub loses
// certified acceptance for its cron history.
