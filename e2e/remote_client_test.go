//go:build !windows

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemoteClient_DaemonOnTCPClientViaPOOMSAE_SERVER spins up a daemon
// bound to TCP loopback under one POOMSAE_HOME, then runs a client
// subprocess in a separate POOMSAE_HOME and workspace pointed at that
// daemon via POOMSAE_SERVER. Exercises init/create/list/close end-to-end
// and asserts the client never auto-started a local daemon.
func TestRemoteClient_DaemonOnTCPClientViaPOOMSAE_SERVER(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	bin := buildPoomsaeBinary(t)

	// Pre-allocate a free port. We can't use --listen 127.0.0.1:0
	// because the runtime file is written before the daemon actually
	// binds — endpoint.Address() returns the literal "127.0.0.1:0".
	port := freeTCPPort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// Server-side state: its own POOMSAE_HOME, runs the daemon.
	serverHome := t.TempDir()

	daemonStderr := &safeBuffer{}
	// t.Cleanup owns this process; t.Context() would kill it before that runs.
	daemon := exec.CommandContext(context.WithoutCancel(t.Context()), bin, "daemon", "start", "--foreground", "--listen", addr) //nolint:gosec
	daemon.Env = append(os.Environ(),
		"POOMSAE_HOME="+serverHome,
		"POOMSAE_DB="+filepath.Join(serverHome, "poomsae.db"),
	)
	daemon.Stdout = io.Discard
	daemon.Stderr = daemonStderr
	daemon.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, daemon.Start())
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("daemon stderr:\n%s", daemonStderr.String())
		}
	})
	t.Cleanup(func() { stopDaemon(daemon) })

	// Wait for /api/v1/ping to answer on the bound address.
	waitForPing(t, "http://"+addr, 5*time.Second)

	// Client-side state: separate POOMSAE_HOME, separate workspace.
	clientHome := t.TempDir()
	clientWS := initRepo(t, "https://github.com/wesm/system.git")

	clientEnv := append(os.Environ(),
		"POOMSAE_HOME="+clientHome,
		"POOMSAE_DB="+filepath.Join(clientHome, "poomsae.db"),
		"POOMSAE_SERVER=http://"+addr,
		"POOMSAE_AUTHOR=e2e-bot",
	)

	// init through the remote.
	runRemoteCmd(t, bin, clientWS, clientEnv, "init")

	// create an issue.
	runRemoteCmd(t, bin, clientWS, clientEnv,
		"create", "hello from remote", "--body", "via POOMSAE_SERVER")

	// list and confirm the issue is in the SERVER's DB. Capture the
	// short_id so we can close the issue without relying on a fixed
	// numeric ref (which the CLI now rejects).
	out := runRemoteCmdOutput(t, bin, clientWS, clientEnv, "list", "--json")
	assert.Contains(t, out, "hello from remote")
	var listed struct {
		Issues []struct {
			ShortID string `json:"short_id"`
		} `json:"issues"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &listed),
		"list --json should decode: %s", out)
	require.Len(t, listed.Issues, 1, "expected exactly one issue: %s", out)
	require.NotEmpty(t, listed.Issues[0].ShortID)

	// close it by short_id.
	runRemoteCmd(t, bin, clientWS, clientEnv, "close", listed.Issues[0].ShortID,
		"--reason", "done",
		"--message", "Implemented the remote close round-trip and confirmed it ran.",
		"--evidence", "commit:abc1234")

	// Critical assertion: the client POOMSAE_HOME has no runtime files.
	// If a local daemon had been auto-started on the client side,
	// we'd find a daemon.<pid>.json under <clientHome>/runtime/<dbhash>/.
	clientRuntime := filepath.Join(clientHome, "runtime")
	assertNoDaemonRuntimeFiles(t, clientRuntime)
}

// TestRemoteInit_PathFreeWireShape spins up a daemon on TCP, slots a
// recording reverse-proxy in front of it, and runs `poomsae init`
// through the proxy. It asserts the on-the-wire request body so we
// know the daemon never receives a client filesystem path — that's
// what makes init work when client and daemon are on different
// hosts. Single-host can't physically force a path-free request, so
// this test fixes the contract at the HTTP layer.
func TestRemoteInit_PathFreeWireShape(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	bin := buildPoomsaeBinary(t)

	port := freeTCPPort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	serverHome := t.TempDir()
	daemonStderr := &safeBuffer{}
	// t.Cleanup owns this process; t.Context() would kill it before that runs.
	daemon := exec.CommandContext(context.WithoutCancel(t.Context()), bin, "daemon", "start", "--foreground", "--listen", addr) //nolint:gosec
	daemon.Env = append(os.Environ(),
		"POOMSAE_HOME="+serverHome,
		"POOMSAE_DB="+filepath.Join(serverHome, "poomsae.db"),
	)
	daemon.Stdout = io.Discard
	daemon.Stderr = daemonStderr
	daemon.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, daemon.Start())
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("daemon stderr:\n%s", daemonStderr.String())
		}
	})
	t.Cleanup(func() { stopDaemon(daemon) })
	waitForPing(t, "http://"+addr, 5*time.Second)

	// Recording reverse-proxy: forwards every request to the daemon
	// while capturing POST /api/v1/projects bodies for inspection.
	target, err := url.Parse("http://" + addr)
	require.NoError(t, err)
	proxy := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) {
		request.SetURL(target)
		request.Out.Host = target.Host
	}}
	var (
		mu     sync.Mutex
		bodies []string
	)
	recorder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/projects" && r.Method == http.MethodPost {
			bs, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, string(bs))
			mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(bs))
			r.ContentLength = int64(len(bs))
		}
		proxy.ServeHTTP(w, r) //nolint:gosec // G704: forwards to a fixed-test loopback daemon, not user input
	}))
	t.Cleanup(recorder.Close)

	clientHome := t.TempDir()
	clientWS := initRepo(t, "https://github.com/wesm/system.git")
	clientEnv := append(os.Environ(),
		"POOMSAE_HOME="+clientHome,
		"POOMSAE_DB="+filepath.Join(clientHome, "poomsae.db"),
		"POOMSAE_SERVER="+recorder.URL,
		"POOMSAE_AUTHOR=e2e-bot",
	)

	runRemoteCmd(t, bin, clientWS, clientEnv, "init")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, bodies, 1, "poomsae init should issue exactly one POST /api/v1/projects")
	body := bodies[0]
	assert.Contains(t, body, `"name":"system"`,
		"client must send the locally-derived project name")
	assert.NotContains(t, body, "project_identity")
	assert.NotContains(t, body, "start_path",
		"remote init must not leak the client filesystem path to the daemon")

	// .poomsae.toml must be written on the client side, not the daemon's POOMSAE_HOME.
	assert.FileExists(t, filepath.Join(clientWS, ".poomsae.toml"))
}

// TestRemoteResolve_PathFreeWireShape is the regression e2e for issue
// #35. After poomsae init binds a workspace, every subsequent project-
// scoped command (list, show, create, …) goes through
// POST /api/v1/projects/resolve. The wire shape must stay path-free
// — {name, alias?} when .poomsae.toml is committed — so a client on host
// B can resolve against a daemon on host A without the daemon stat'ing
// a path that doesn't exist on its filesystem.
func TestRemoteResolve_PathFreeWireShape(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	bin := buildPoomsaeBinary(t)

	port := freeTCPPort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	serverHome := t.TempDir()
	daemonStderr := &safeBuffer{}
	// t.Cleanup owns this process; t.Context() would kill it before that runs.
	daemon := exec.CommandContext(context.WithoutCancel(t.Context()), bin, "daemon", "start", "--foreground", "--listen", addr) //nolint:gosec
	daemon.Env = append(os.Environ(),
		"POOMSAE_HOME="+serverHome,
		"POOMSAE_DB="+filepath.Join(serverHome, "poomsae.db"),
	)
	daemon.Stdout = io.Discard
	daemon.Stderr = daemonStderr
	daemon.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, daemon.Start())
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("daemon stderr:\n%s", daemonStderr.String())
		}
	})
	t.Cleanup(func() { stopDaemon(daemon) })
	waitForPing(t, "http://"+addr, 5*time.Second)

	target, err := url.Parse("http://" + addr)
	require.NoError(t, err)
	proxy := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) {
		request.SetURL(target)
		request.Out.Host = target.Host
	}}
	var (
		mu            sync.Mutex
		resolveBodies []string
	)
	recorder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/projects/resolve" && r.Method == http.MethodPost {
			bs, _ := io.ReadAll(r.Body)
			mu.Lock()
			resolveBodies = append(resolveBodies, string(bs))
			mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(bs))
			r.ContentLength = int64(len(bs))
		}
		proxy.ServeHTTP(w, r) //nolint:gosec // G704: forwards to a fixed-test loopback daemon
	}))
	t.Cleanup(recorder.Close)

	clientHome := t.TempDir()
	clientWS := initRepo(t, "https://github.com/wesm/system.git")
	clientEnv := append(os.Environ(),
		"POOMSAE_HOME="+clientHome,
		"POOMSAE_DB="+filepath.Join(clientHome, "poomsae.db"),
		"POOMSAE_SERVER="+recorder.URL,
		"POOMSAE_AUTHOR=e2e-bot",
	)

	// Bind the workspace first; init writes .poomsae.toml.
	runRemoteCmd(t, bin, clientWS, clientEnv, "init")
	require.FileExists(t, filepath.Join(clientWS, ".poomsae.toml"))

	// list goes through resolve. With .poomsae.toml present, the request
	// must carry {name, alias} and never start_path.
	runRemoteCmd(t, bin, clientWS, clientEnv, "list")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, resolveBodies, "poomsae list must issue POST /api/v1/projects/resolve")
	body := resolveBodies[0]
	assert.Contains(t, body, `"name":"system"`,
		"client must send the locally-derived project name")
	assert.Contains(t, body, `"alias"`,
		"client must send alias metadata so the daemon can do alias-first repair")
	assert.NotContains(t, body, "start_path",
		"remote resolve must not leak the client filesystem path to the daemon (issue #35)")
}

// freeTCPPort binds 127.0.0.1:0, captures the bound port, and closes.
// There is a small race window before the caller binds again; accept it.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// waitForPing polls /api/v1/ping until it answers 200 with {"ok": true}
// or the deadline expires.
func waitForPing(t *testing.T, base string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/api/v1/ping", nil)
		require.NoError(t, reqErr)
		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var info struct {
					OK bool `json:"ok"`
				}
				if json.Unmarshal(body, &info) == nil && info.OK {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("daemon at %s did not answer /api/v1/ping within %s", base, timeout)
}

// runRemoteCmd runs a poomsae subcommand and asserts success, dumping
// combined output on failure for debugging.
func runRemoteCmd(t *testing.T, bin, workdir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bin, args...) //nolint:gosec
	cmd.Dir = workdir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "poomsae %s: %s", strings.Join(args, " "), out)
}

// runRemoteCmdOutput runs a poomsae subcommand, returns combined output,
// and asserts success.
func runRemoteCmdOutput(t *testing.T, bin, workdir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bin, args...) //nolint:gosec
	cmd.Dir = workdir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "poomsae %s: %s", strings.Join(args, " "), out)
	return string(out)
}

// assertNoDaemonRuntimeFiles walks <dir> and fails if any daemon.<pid>.json
// is present. Missing dir is a pass (no daemon was started).
func assertNoDaemonRuntimeFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		t.Fatalf("reading client runtime dir %s: %v", dir, err)
	}
	for _, e := range entries {
		sub := filepath.Join(dir, e.Name())
		files, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, f := range files {
			n := f.Name()
			if strings.HasPrefix(n, "daemon.") && strings.HasSuffix(n, ".json") {
				t.Errorf("client unexpectedly wrote runtime file %s/%s", sub, n)
			}
		}
	}
}
