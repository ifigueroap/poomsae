package poomsae_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ifigueroap/poomsae"
)

type fixedCredentialStore struct {
	credential poomsae.FederationCredential
}

func (s *fixedCredentialStore) FederationCredential(context.Context, string) (poomsae.FederationCredential, bool, error) {
	return s.credential, true, nil
}

func (s *fixedCredentialStore) StoreFederationCredential(context.Context, string, poomsae.FederationCredential) error {
	return nil
}

func (s *fixedCredentialStore) DeleteFederationCredential(context.Context, string) error {
	return nil
}

// A mounted service must forward native cron writes like the standalone
// daemon: the shared push callback drains pending spoke events before the hub
// mutation, so a newly created local parent reaches the hub first.

func readAll(t *testing.T, response *http.Response) string {
	t.Helper()
	out := new(bytes.Buffer)
	_, err := out.ReadFrom(response.Body)
	require.NoError(t, err)
	return out.String()
}
