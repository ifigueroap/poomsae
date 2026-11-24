package pgstore_test

import (
	"testing"

	"github.com/ifigueroap/poomsae/internal/db/pgstore/pgstoretest"

	"github.com/ifigueroap/poomsae/internal/db/dbtest"
	"github.com/ifigueroap/poomsae/internal/db/pgstore"
	"github.com/stretchr/testify/require"
)

func TestResetFederatedProjectIgnoresRejectedPendingClaims(t *testing.T) {
	dsn, cleanup := pgstoretest.NewDatabase(t, t.Context())
	t.Cleanup(cleanup)
	store, err := pgstore.Open(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	dbtest.RunFederationResetIgnoresRejectedPendingClaims(t, store)
}
