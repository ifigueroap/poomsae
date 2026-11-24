package jsonl_test

import (
	"context"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/stretchr/testify/require"
)

// openCutoverTargetDB opens a SQLite store at path. sqlitestore.Open
// bootstraps the canonical schema in one transaction when the file is
// fresh, so the returned handle is immediately ready for use. POOMSAE_HOME
// is scoped to a per-test TempDir so snapshot files land under it.
//
//nolint:revive // test helper: t *testing.T conventionally precedes ctx.
func openCutoverTargetDB(t *testing.T, ctx context.Context, path string) *sqlitestore.Store {
	t.Helper()
	t.Setenv("POOMSAE_HOME", t.TempDir())
	d, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}
