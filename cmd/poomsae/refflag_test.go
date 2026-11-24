package main_test

import (
	"testing"

	main "github.com/ifigueroap/poomsae/cmd/poomsae"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRef_QualifiedSelectsProject(t *testing.T) {
	r, err := main.ResolveRef("poomsae#abc4", "fallback-project")
	require.NoError(t, err)
	assert.Equal(t, "poomsae", r.ProjectName)
	assert.Equal(t, "abc4", r.RefForAPI)
}

func TestResolveRef_BareUsesFallback(t *testing.T) {
	r, err := main.ResolveRef("abc4", "demo")
	require.NoError(t, err)
	assert.Equal(t, "demo", r.ProjectName)
	assert.Equal(t, "abc4", r.RefForAPI)
}

func TestResolveRef_ULIDUsesFallbackProject(t *testing.T) {
	r, err := main.ResolveRef("01HZNQ7VFPK1XGD8R5MABCD4EX", "demo")
	require.NoError(t, err)
	assert.Equal(t, "demo", r.ProjectName)
	assert.Equal(t, "01HZNQ7VFPK1XGD8R5MABCD4EX", r.RefForAPI)
}

func TestResolveRef_LegacyNumberFails(t *testing.T) {
	_, err := main.ResolveRef("12", "demo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "looks like a legacy issue number")
}

func TestResolveRef_RequiresProjectForBare(t *testing.T) {
	_, err := main.ResolveRef("abc4", "")
	assert.Error(t, err)
}
