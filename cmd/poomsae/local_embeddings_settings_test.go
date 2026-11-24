package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/config"
	"github.com/ifigueroap/poomsae/internal/daemon"
	"github.com/ifigueroap/poomsae/internal/daemonruntime"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/require"
	"go.kenn.io/kit/embedlocal"
)

func TestLocalEmbeddingsSetupStatus(t *testing.T) {
	home := clearLocalEmbeddingsEnv(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".kenn"), []byte("unrelated file"), 0o600))
	for _, tc := range []struct{ provider, url string }{
		{"ollama", "http://127.0.0.1:11434/v1"},
		{"other", "https://embeddings.example/v1"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cfg, err := config.ParseSearchConfig(fmt.Appendf(nil, "[search.embeddings]\nbase_url = %q\nmodel = \"nomic-embed-text\"\ndims = 768\n", tc.url))
			require.NoError(t, err)
			setup := &localEmbeddingsSetup{embeddings: cfg.Embeddings}
			status, err := setup.Status(t.Context())
			require.NoError(t, err, "configured external search does not use the shared local installation")
			require.Equal(t, tc.provider, status.Provider)
			require.False(t, status.CanChange)
		})
	}
}

func TestLocalEmbeddingsRestart(t *testing.T) {
	previous := localEmbeddingsInspect
	localEmbeddingsInspect = func(context.Context, embedlocal.Options) (embedlocal.Status, error) { return embedlocal.Status{}, nil }
	t.Cleanup(func() { localEmbeddingsInspect = previous })
	startErr := errors.New("helper could not start")
	exitErr := errors.New("replacement refused")
	for _, tc := range []struct {
		name              string
		callback          bool
		startErr, exitErr error
		attempts, calls   int
		wantErr           error
		canRestart        bool
	}{
		{name: "repeated request", callback: true, attempts: 2, calls: 1},
		{name: "missing callback", attempts: 1, wantErr: daemon.ErrLocalEmbeddingsBusy},
		{name: "start failure", callback: true, startErr: startErr, attempts: 1, calls: 1, wantErr: startErr, canRestart: true},
		{name: "helper exit", callback: true, attempts: 1, calls: 1, exitErr: exitErr, canRestart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			setup := &localEmbeddingsSetup{job: &api.LocalEmbeddingsJob{State: "restart"}}
			if tc.callback {
				setup.restart = func() error { calls++; return tc.startErr }
			}
			status, err := setup.Status(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.callback, status.CanRestart)
			for range tc.attempts {
				require.ErrorIs(t, setup.Restart(t.Context()), tc.wantErr)
			}
			if tc.exitErr != nil {
				setup.restartExited(tc.exitErr)
			}
			status, err = setup.Status(t.Context())
			require.NoError(t, err)
			require.Equal(t, "restart", status.Job.State)
			require.Equal(t, tc.canRestart, status.CanRestart)
			require.Equal(t, tc.calls, calls)
			if tc.startErr != nil {
				require.Contains(t, status.Job.Error, tc.startErr.Error())
			}
			if tc.exitErr != nil {
				require.Contains(t, status.Job.Error, tc.exitErr.Error())
			}
		})
	}
	t.Run("status during restart start", func(t *testing.T) {
		started, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		setup := &localEmbeddingsSetup{job: &api.LocalEmbeddingsJob{State: "restart"}, restart: func() error {
			close(started)
			<-release
			return nil
		}}
		go func() { finished <- setup.Restart(t.Context()) }()
		defer func() { close(release); <-finished }()
		<-started
		statusDone := make(chan api.LocalEmbeddingsStatus, 1)
		go func() { status, _ := setup.Status(t.Context()); statusDone <- status }()
		select {
		case status := <-statusDone:
			require.Equal(t, "restart", status.Job.State)
			require.False(t, status.CanRestart)
			require.True(t, status.RestartInProgress)
		case <-time.After(time.Second):
			t.Fatal("status blocked while restart was starting")
		}
	})
}

func TestLocalEmbeddingsTurnOnPreservesProviderSavedAfterStartup(t *testing.T) {
	clearLocalEmbeddingsEnv(t)
	home := setupKataEnv(t)
	original := []byte("[search.embeddings]\nbase_url = \"http://127.0.0.1:11434/v1\"\nmodel = \"nomic-embed-text\"\ndims = 768\n")
	path := filepath.Join(home, "config.toml")
	require.NoError(t, os.WriteFile(path, original, 0o600))
	var run func(context.Context)
	setup := &localEmbeddingsSetup{start: func(work func(context.Context)) bool { run = work; return true }}
	require.NoError(t, setup.TurnOn(t.Context()))
	require.ErrorIs(t, setup.TurnOn(t.Context()), daemon.ErrLocalEmbeddingsBusy)
	run(t.Context())
	require.Equal(t, "restart", setup.job.State)
	got, err := os.ReadFile(path) // #nosec G304 -- configuration in the isolated test home
	require.NoError(t, err)
	require.Equal(t, original, got, "a page loaded while search was off cannot switch a provider saved later")
}

func TestSharedChoiceDaemonStartup(t *testing.T) {
	for _, scenario := range []string{"vector path directory", "preflight", "enabled"} {
		t.Run(scenario, func(t *testing.T) {
			clearLocalEmbeddingsEnv(t)
			resetFlags(t)
			home := setupKataEnv(t)
			t.Setenv("PORT", "")
			t.Setenv("POOMSAE_DSN", "")
			t.Setenv("POOMSAE_AUTH_TOKEN", "")
			t.Setenv(daemonruntime.AutoStartMarkerEnv, "1")
			require.NoError(t, embedlocal.RecordChoice())
			switch scenario {
			case "vector path directory":
				path, err := vectorsPathForDSN(filepath.Join(home, "poomsae.db"))
				require.NoError(t, err)
				require.NoError(t, os.Mkdir(path, 0o700))
			case "preflight":
				require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte("[search.embeddings]\nmodel_context_tokens=32000\nmax_batch_tokens=31999\n"), 0o600))
			}
			if scenario == "enabled" {
				provider := testenv.NewEmbeddingServer(t, func(string) []float32 {
					v := make([]float32, 768)
					v[0] = 1
					return v
				})
				oldEndpoint := localEmbeddingsEndpoint
				t.Cleanup(func() { localEmbeddingsEndpoint = oldEndpoint })
				localEmbeddingsEndpoint = func(context.Context, embedlocal.Options) (string, error) { return provider.URL, nil }
			}
			baseURL := launchDaemon(t)
			status := getJSON[api.LocalEmbeddingsStatus](t, baseURL+"/api/v1/embeddings/local")
			if scenario == "enabled" {
				require.Equal(t, "built_in", status.Provider)
			} else {
				require.Equal(t, "none", status.Provider)
			}
			project := postJSON[struct {
				Project struct {
					ID int64 `json:"id"`
				} `json:"project"`
			}](t, baseURL+"/api/v1/projects", map[string]string{"name": "example-project", "actor": "user-a"})
			projectURL := fmt.Sprintf("%s/api/v1/projects/%d", baseURL, project.Project.ID)
			postJSONOK(t, projectURL+"/issues", map[string]string{"title": "Repair bicycle", "actor": "user-a"})
			result := getJSON[struct {
				Mode    string          `json:"mode"`
				Results []api.SearchHit `json:"results"`
			}](t, projectURL+"/search?q=bicycle")
			if scenario == "enabled" {
				require.Equal(t, "hybrid", result.Mode)
			} else {
				require.Equal(t, "lexical", result.Mode)
			}
			require.Len(t, result.Results, 1)
			require.Equal(t, "Repair bicycle", result.Results[0].Issue.Title)
		})
	}
}
