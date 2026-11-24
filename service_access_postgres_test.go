package poomsae_test

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ifigueroap/poomsae"
	"github.com/ifigueroap/poomsae/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServicePostgresTransactionFenceRollsBackWithMutation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	dsn, cleanup := testenv.NewPostgresContainer(t, ctx)
	t.Cleanup(cleanup)

	controller := &recordingAccessController{}
	controller.transactionFence = func(ctx context.Context, tx poomsae.Transaction) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO poomsae.fence_markers(attempt) VALUES($1)`, 1); err != nil {
			return err
		}
		return poomsae.ErrAccessDenied
	}
	service, err := poomsae.New(ctx, poomsae.Config{
		DSN: dsn, Access: controller,
		Postgres: poomsae.PostgresConfig{Schema: "poomsae", SchemaMode: poomsae.PostgresSchemaBootstrap},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	project, err := service.EnsureProject(ctx, poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "example-project",
	})
	require.NoError(t, err)

	inspection, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, inspection.Close()) })
	_, err = inspection.ExecContext(ctx,
		`CREATE TABLE poomsae.fence_markers (attempt BIGINT NOT NULL)`)
	require.NoError(t, err)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/projects/"+
		strconv.FormatInt(project.Project.ID, 10)+"/issues",
		bytes.NewBufferString(`{"actor":"ignored","title":"must roll back"}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(poomsae.WithPrincipal(request.Context(), poomsae.Principal{
		Subject: "user-123", Actor: "Example User",
	}))
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	var markerCount, issueCount int
	require.NoError(t, inspection.QueryRowContext(ctx,
		`SELECT count(*) FROM poomsae.fence_markers`).Scan(&markerCount))
	require.NoError(t, inspection.QueryRowContext(ctx,
		`SELECT count(*) FROM poomsae.issues WHERE title = $1`, "must roll back").Scan(&issueCount))
	assert.Zero(t, markerCount)
	assert.Zero(t, issueCount)
}
