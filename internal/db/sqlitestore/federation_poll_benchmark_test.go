package sqlitestore_test

import (
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/require"
)

func BenchmarkFederationPoll(b *testing.B) {
	b.Cleanup(sqlitestore.CountStatementsForTest())
	store, err := sqlitestore.Open(b.Context(), filepath.Join(b.TempDir(), "poomsae.db"))
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, store.Close()) })
	testenv.BenchmarkFederationPoll(b, store)
}
