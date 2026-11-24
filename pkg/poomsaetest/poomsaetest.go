// Package poomsaetest runs a real, in-process Poomsae service for tests of code that
// talks to Poomsae through github.com/ifigueroap/poomsae/pkg/client.
//
// Each Server owns a private SQLite database in a test temporary directory, so
// tests exercise the same HTTP API, validation, and error envelopes as a
// production daemon without a separate process or shared state.
package poomsaetest

import (
	"context"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae"
	"github.com/ifigueroap/poomsae/pkg/client"
)

// Server is a running Poomsae service reachable at Endpoint.
type Server struct {
	// Endpoint is the daemon base URL to hand to client constructors: an
	// http://127.0.0.1:<port> origin, or unix:///<path> with WithUnixSocket.
	Endpoint string
	// Service is the Poomsae service behind Endpoint.
	Service *poomsae.Service

	token string
}

type options struct {
	token string
	unix  bool
}

// Option configures New.
type Option func(*options)

// WithToken requires bearer token authentication for every API request.
// Without it, the service trusts callers the way an unauthenticated local
// daemon does.
func WithToken(token string) Option {
	return func(o *options) { o.token = token }
}

// WithUnixSocket serves the service on a Unix socket instead of loopback TCP.
func WithUnixSocket() Option {
	return func(o *options) { o.unix = true }
}

// New starts a Poomsae service and stops it when the test ends.
func New(tb testing.TB, opts ...Option) *Server {
	tb.Helper()
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	auth := poomsae.AuthConfig{TrustCallerAuthentication: true}
	if o.token != "" {
		auth = poomsae.AuthConfig{Token: o.token}
	}
	service, err := poomsae.New(tb.Context(), poomsae.Config{
		DSN:    filepath.Join(tb.TempDir(), "poomsae.db"),
		Auth:   auth,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		tb.Fatalf("poomsaetest: start poomsae service: %v", err)
	}
	// The service must keep running until the cleanup below closes the HTTP
	// server first; tb.Context() is canceled before cleanups run.
	runCtx, stopRun := context.WithCancel(context.WithoutCancel(tb.Context()))
	runDone := make(chan error, 1)
	go func() { runDone <- service.Run(runCtx) }()

	httpServer := httptest.NewUnstartedServer(service.Handler())
	endpoint := ""
	if o.unix {
		socket := unixSocketPath(tb)
		listener, err := new(net.ListenConfig).Listen(tb.Context(), "unix", socket)
		if err != nil {
			tb.Fatalf("poomsaetest: listen on %s: %v", socket, err)
		}
		_ = httpServer.Listener.Close()
		httpServer.Listener = listener
		endpoint = "unix://" + socket
	}
	httpServer.Start()
	if endpoint == "" {
		endpoint = httpServer.URL
	}
	tb.Cleanup(func() {
		httpServer.Close()
		stopRun()
		if err := <-runDone; err != nil {
			tb.Errorf("poomsaetest: run poomsae service: %v", err)
		}
		if err := service.Close(); err != nil {
			tb.Errorf("poomsaetest: close poomsae service: %v", err)
		}
	})
	return &Server{Endpoint: endpoint, Service: service, token: o.token}
}

// Client returns a client for the server, authenticated with the token from
// WithToken when one was set.
func (s *Server) Client(tb testing.TB, opts ...client.Option) *client.Client {
	tb.Helper()
	api, err := client.NewForTarget(tb.Context(), s.Endpoint, client.TargetAuth{Token: s.token}, opts...)
	if err != nil {
		tb.Fatalf("poomsaetest: build client: %v", err)
	}
	return api
}

// unixSocketPath returns a socket path short enough for every platform's
// sun_path limit; test temporary directories can exceed it on macOS.
func unixSocketPath(tb testing.TB) string {
	tb.Helper()
	dir, err := os.MkdirTemp("", "poomsaetest-")
	if err != nil {
		tb.Fatalf("poomsaetest: create socket directory: %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "poomsae.sock")
}
