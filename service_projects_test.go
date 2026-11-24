package poomsae_test

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ifigueroap/poomsae"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceEnsureProjectIsIdempotentByStableIdentity(t *testing.T) {
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:  filepath.Join(t.TempDir(), "service.db"),
		Auth: poomsae.AuthConfig{TrustCallerAuthentication: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	spec := poomsae.ProjectSpec{
		UID:  "01HZNQ7VFPK1XGD8R5MABCD4EX",
		Name: "example-host-project",
	}
	first, err := service.EnsureProject(t.Context(), spec)
	require.NoError(t, err)
	assert.True(t, first.Created)
	assert.Equal(t, spec.UID, first.Project.UID)
	assert.Equal(t, spec.Name, first.Project.Name)
	assert.Equal(t, poomsae.ProjectActive, first.Project.State)

	second, err := service.EnsureProject(t.Context(), spec)
	require.NoError(t, err)
	assert.False(t, second.Created)
	assert.Equal(t, first.Project, second.Project)
}

func TestServiceEnsureProjectRejectsIdentityAndNameCollisions(t *testing.T) {
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:  filepath.Join(t.TempDir(), "service.db"),
		Auth: poomsae.AuthConfig{TrustCallerAuthentication: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	_, err = service.EnsureProject(t.Context(), poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "example-host-project",
	})
	require.NoError(t, err)

	_, err = service.EnsureProject(t.Context(), poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "different-project-name",
	})
	require.ErrorIs(t, err, poomsae.ErrProjectConflict)

	_, err = service.EnsureProject(t.Context(), poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EY", Name: "example-host-project",
	})
	require.ErrorIs(t, err, poomsae.ErrProjectConflict)
}

func TestServiceEnsureProjectConvergesConcurrentExactCalls(t *testing.T) {
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:  filepath.Join(t.TempDir(), "service.db"),
		Auth: poomsae.AuthConfig{TrustCallerAuthentication: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	spec := poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "example-host-project",
	}
	results := make([]poomsae.EnsureProjectResult, 2)
	errs := make([]error, 2)
	var start sync.WaitGroup
	start.Add(1)
	var calls sync.WaitGroup
	for i := range results {
		calls.Add(1)
		go func(index int) {
			defer calls.Done()
			start.Wait()
			results[index], errs[index] = service.EnsureProject(t.Context(), spec)
		}(i)
	}
	start.Done()
	calls.Wait()

	require.NoError(t, errors.Join(errs...))
	assert.Equal(t, results[0].Project, results[1].Project)
	assert.NotEqual(t, results[0].Created, results[1].Created,
		"exactly one concurrent call must report creation")
}

func TestServiceArchiveProjectIsIdempotentAndRetainsIdentity(t *testing.T) {
	service, err := poomsae.New(t.Context(), poomsae.Config{
		DSN:  filepath.Join(t.TempDir(), "service.db"),
		Auth: poomsae.AuthConfig{TrustCallerAuthentication: true},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	spec := poomsae.ProjectSpec{
		UID: "01HZNQ7VFPK1XGD8R5MABCD4EX", Name: "example-host-project",
	}
	ensured, err := service.EnsureProject(t.Context(), spec)
	require.NoError(t, err)

	archived, err := service.ArchiveProject(
		t.Context(), spec.UID, "Example Operator",
	)
	require.NoError(t, err)
	assert.True(t, archived.Changed)
	assert.Equal(t, ensured.Project.ID, archived.Project.ID)
	assert.Equal(t, poomsae.ProjectArchived, archived.Project.State)

	retry, err := service.ArchiveProject(
		t.Context(), spec.UID, "Example Operator",
	)
	require.NoError(t, err)
	assert.False(t, retry.Changed)
	assert.Equal(t, archived.Project, retry.Project)

	retained, err := service.EnsureProject(t.Context(), spec)
	require.NoError(t, err)
	assert.False(t, retained.Created)
	assert.Equal(t, archived.Project, retained.Project)
}
