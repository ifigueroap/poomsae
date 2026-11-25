package client_test

import (
	"encoding/json"
	"testing"

	"github.com/ifigueroap/poomsae/pkg/client/generated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedExternalFieldConflictCandidatesRoundTrip(t *testing.T) {
	input := []byte(`{
		"poomsae_field":"scheduled_on",
		"poomsae_candidate":{"kind":"date","value":"2026-08-21"},
		"external_candidate":{"kind":"date","value":"2026-08-22"}
	}`)
	var conflict generated.ExternalFieldConflictOut
	require.NoError(t, json.Unmarshal(input, &conflict))
	require.NotNil(t, conflict.PoomsaeCandidate)
	require.NotNil(t, conflict.ExternalCandidate)
	assert.Equal(t, "date", conflict.PoomsaeCandidate.Kind)
	require.NotNil(t, conflict.PoomsaeCandidate.Value)
	assert.Equal(t, "2026-08-21", *conflict.PoomsaeCandidate.Value)
	assert.Equal(t, "date", conflict.ExternalCandidate.Kind)
	require.NotNil(t, conflict.ExternalCandidate.Value)
	assert.Equal(t, "2026-08-22", *conflict.ExternalCandidate.Value)

	encoded, err := json.Marshal(conflict)
	require.NoError(t, err)
	assert.JSONEq(t, string(input), string(encoded))
}
