package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ifigueroap/poomsae/internal/federationsigning"
	"github.com/ifigueroap/poomsae/internal/federationsigning/signingpolicy"
	"github.com/stretchr/testify/require"
)

// FederationSigning creates a verifier and its matching spoke key file.
func FederationSigning(t *testing.T, base, keyID string) (*federationsigning.Verifier, string) {
	t.Helper()
	dir := t.TempDir()
	key := filepath.Join(dir, "spoke signing.key")
	require.NoError(t, os.WriteFile(key, []byte(strings.Repeat("k", 64)), 0600))
	state := filepath.Join(dir, "replay.state")
	require.NoError(t, federationsigning.InitializeReplayState(state))
	keys := []signingpolicy.Key{{KeyID: keyID, KeyFile: key, EnrollmentID: 1}}
	verifier, err := federationsigning.NewVerifier(base, keys, state)
	require.NoError(t, err)
	verifier.SkipStartupForTest()
	t.Cleanup(func() { require.NoError(t, verifier.Close()) })
	return verifier, key
}
