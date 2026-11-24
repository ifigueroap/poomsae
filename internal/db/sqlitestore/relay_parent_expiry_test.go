package sqlitestore_test

import (
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db/dbtest"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/stretchr/testify/require"
)

func TestRelayParentCredentialCanExpire(t *testing.T) {
	store, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "parent.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	dbtest.RunRelayParentCredentialExpiration(t, store)
}
