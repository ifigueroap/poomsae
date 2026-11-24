package mcpserver

import (
	"errors"
	"net/http"
	"testing"

	poomsaeclient "github.com/ifigueroap/poomsae/pkg/client"
	"github.com/stretchr/testify/require"
)

// Catalog-only tests still use an initialized client so production can bind
// immutable request limits. Any accidental daemon request fails the test.
func inertTestClient(t *testing.T) *poomsaeclient.Client {
	t.Helper()
	client, err := poomsaeclient.NewWithHTTPClient("http://daemon.invalid", &http.Client{Transport: inertMCPTransport{t: t}})
	require.NoError(t, err)
	return client
}

type inertMCPTransport struct {
	t *testing.T
}

func (transport inertMCPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.t.Helper()
	transport.t.Errorf("unexpected daemon request in catalog-only test: %s %s", request.Method, request.URL.Path)
	return nil, errors.New("unexpected daemon request in catalog-only test")
}
