package jsonl_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/ifigueroap/poomsae/internal/jsonl"
	"github.com/stretchr/testify/require"
)

func TestCommentSearchCutoverUpgradesVersion32(t *testing.T) {
	t.Setenv("POOMSAE_HOME", t.TempDir())
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, projectHubRelaySQLiteSchema32+`
 INSERT INTO meta(key,value) VALUES('schema_version','32'),('instance_uid','00000000000000000000000009');
 INSERT INTO projects(uid,name) VALUES('00000000000000000000000001','example-project');
 INSERT INTO issues(uid,project_id,short_id,title,author) SELECT '00000000000000000000000002',id,'0002','login','tester' FROM projects WHERE name='example-project';
 INSERT INTO comments(uid,issue_id,author,body) SELECT '00000000000000000000000003',id,'tester','safari' FROM issues;`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	require.NoError(t, jsonl.AutoCutover(ctx, path))
	upgraded, err := sqlitestore.Open(ctx, path)
	require.NoError(t, err)
	defer func() { _ = upgraded.Close() }()
	p, err := upgraded.ProjectByName(ctx, "example-project")
	require.NoError(t, err)
	hits, err := upgraded.SearchFTS(ctx, db.SearchFTSParams{ProjectID: p.ID, Query: "safari"})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, []string{"comments"}, hits[0].MatchedIn)
	var count int
	require.NoError(t, upgraded.QueryRowContext(ctx, `SELECT count(*) FROM comments_fts`).Scan(&count))
	require.Equal(t, 1, count)
}
