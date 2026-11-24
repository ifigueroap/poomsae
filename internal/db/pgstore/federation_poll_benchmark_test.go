package pgstore_test

import (
	"testing"

	"github.com/ifigueroap/poomsae/internal/db/pgstore"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/require"
)

func BenchmarkFederationPoll(b *testing.B) {
	b.Cleanup(pgstore.CountStatementsForTest())
	dsn, cleanup := conformanceTemplate.NewDatabase(b, b.Context())
	b.Cleanup(cleanup)
	store, err := pgstore.OpenWithConfig(b.Context(), dsn, conformanceConfig())
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, store.Close()) })
	testenv.BenchmarkFederationPoll(b, store)
}
