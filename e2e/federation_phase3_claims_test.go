//go:build !windows

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae/internal/api"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type federationClaimFixture struct {
	bin           string
	hub           *testenv.Env
	hubProject    db.Project
	hubIssue      db.Issue
	replayAfterID int64
	spokeA        federationClaimSpoke
	spokeB        federationClaimSpoke
}

type federationClaimSpoke struct {
	dirs    e2eDirs
	url     string
	http    *http.Client
	db      *sqlitestore.Store
	stderr  *safeBuffer
	replica api.CreateFederationReplicaResponseBody
}

func startFederationClaimSpoke(ctx context.Context, t *testing.T, bin string) federationClaimSpoke {
	t.Helper()
	dirs := newE2EDirs(t)
	stderr := startDaemon(t, bin, append(dirs.env(), "POOMSAE_FEDERATION_PULL_INTERVAL_MS=25"))
	url, client := connectDaemon(t, dirs, stderr)
	store, err := sqlitestore.Open(ctx, dirs.dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return federationClaimSpoke{dirs: dirs, url: url, http: client, db: store, stderr: stderr}
}

type e2eClaimRun struct {
	stdout string
	stderr string
}

type e2eClaimActionBody struct {
	Granted bool `json:"granted"`
	Pending bool `json:"pending"`
	Holder  struct {
		Holder            string `json:"holder"`
		HolderInstanceUID string `json:"holder_instance_uid"`
		ClientKind        string `json:"client_kind"`
	} `json:"holder"`
	Claim *struct {
		Holder            string     `json:"holder"`
		HolderInstanceUID string     `json:"holder_instance_uid"`
		ClaimKind         string     `json:"claim_kind"`
		ExpiresAt         *time.Time `json:"expires_at"`
		ReleasedAt        *time.Time `json:"released_at"`
	} `json:"claim"`
}

type e2eClaimStealBody struct {
	ReleasedHolder string             `json:"released_holder"`
	NewHolder      string             `json:"new_holder"`
	Released       e2eClaimActionBody `json:"released"`
	Claimed        e2eClaimActionBody `json:"claimed"`
}

func hubClaimCLIDirs(t *testing.T, fx federationClaimFixture) e2eDirs {
	t.Helper()
	dirs := newE2EDirs(t)
	localConfig := fmt.Sprintf("version = 1\n\n[server]\nurl = %q\nallow_insecure = true\n", fx.hub.URL)
	require.NoError(t, os.WriteFile(filepath.Join(dirs.repoDir, ".poomsae.local.toml"), []byte(localConfig), 0o600))
	return dirs
}

func runKataJSON[T any](t *testing.T, bin string, dirs e2eDirs, args ...string) T {
	t.Helper()
	res := runKataOK(t, bin, dirs, append([]string{"--json"}, args...)...)
	var out T
	require.NoErrorf(t, json.Unmarshal([]byte(res.stdout), &out), "stdout: %s\nstderr: %s", res.stdout, res.stderr)
	return out
}

func runKataJSONWantExit[T any](t *testing.T, bin string, dirs e2eDirs, wantExit int, args ...string) struct {
	body   T
	stderr string
} {
	t.Helper()
	res := runKataWantExit(t, bin, dirs, wantExit, append([]string{"--json"}, args...)...)
	var out T
	require.NoErrorf(t, json.Unmarshal([]byte(res.stdout), &out), "stdout: %s\nstderr: %s", res.stdout, res.stderr)
	return struct {
		body   T
		stderr string
	}{body: out, stderr: res.stderr}
}

func runKataOK(t *testing.T, bin string, dirs e2eDirs, args ...string) e2eClaimRun {
	t.Helper()
	res, exitCode := runKataWithSQLiteBusyRetry(t, bin, dirs, args...)
	require.Equalf(t, 0, exitCode, "poomsae %s\nstdout: %s\nstderr: %s", strings.Join(args, " "), res.stdout, res.stderr)
	return res
}

func runKataWantExit(t *testing.T, bin string, dirs e2eDirs, wantExit int, args ...string) e2eClaimRun {
	t.Helper()
	res, exitCode := runKata(t, bin, dirs, args...)
	require.Equalf(t, wantExit, exitCode, "poomsae %s\nstdout: %s\nstderr: %s", strings.Join(args, " "), res.stdout, res.stderr)
	return res
}

func runKataWithSQLiteBusyRetry(t *testing.T, bin string, dirs e2eDirs, args ...string) (e2eClaimRun, int) {
	t.Helper()
	var res e2eClaimRun
	var exitCode int
	deadline := time.Now().Add(5 * time.Second)
	for attempt := 0; ; attempt++ {
		res, exitCode = runKata(t, bin, dirs, args...)
		if exitCode == 0 || !claimE2ESQLiteBusy(res) || time.Now().After(deadline) {
			return res, exitCode
		}
		// These e2e tests run foreground CLI commands against the same daemon DB
		// that background federation runners are polling. CI can occasionally hit
		// SQLite's short busy window; retry only success-expected commands.
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
}

func claimE2ESQLiteBusy(res e2eClaimRun) bool {
	return isSQLiteBusyMessage(res.stdout + "\n" + res.stderr)
}

func runKata(t *testing.T, bin string, dirs e2eDirs, args ...string) (e2eClaimRun, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // test-built binary and test-controlled args
	cmd.Dir = dirs.repoDir
	cmd.Env = append(dirs.env(), "POOMSAE_HTTP_TIMEOUT=10s")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := e2eClaimRun{stdout: stdout.String(), stderr: stderr.String()}
	if ctx.Err() != nil {
		t.Fatalf("poomsae %s timed out\nstdout: %s\nstderr: %s", strings.Join(args, " "), out.stdout, out.stderr)
	}
	if err == nil {
		return out, 0
	}
	var exitErr *exec.ExitError
	if !assert.ErrorAs(t, err, &exitErr) {
		t.Fatalf("poomsae %s failed to start: %v\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), err, out.stdout, out.stderr)
	}
	return out, exitErr.ExitCode()
}

func (r e2eClaimRun) String() string {
	return fmt.Sprintf("stdout: %s\nstderr: %s", r.stdout, r.stderr)
}
