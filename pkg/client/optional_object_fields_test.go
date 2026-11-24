package client

import (
	"encoding/json"
	"encoding/json/jsontext"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ifigueroap/poomsae/pkg/client/generated"
)

// A changes block whose edit touched no parent carries no parent_set or
// parent_removed peer; the generated client must validate and round-trip it
// without inventing empty peer objects.
func TestLinkChangesWithoutParentPeersValidatesAndOmitsParentPeers(t *testing.T) {
	changes := generated.LinkChanges{}
	require.NoError(t, changes.Validate(),
		"a changes block without parent peers must validate")

	out, err := json.Marshal(changes)
	require.NoError(t, err)
	var roundTrip map[string]jsontext.Value
	require.NoError(t, json.Unmarshal(out, &roundTrip))
	require.NotContains(t, roundTrip, "parent_set",
		"absent parent_set must not marshal as an empty object")
	require.NotContains(t, roundTrip, "parent_removed",
		"absent parent_removed must not marshal as an empty object")
}

// A parentless issue response carries no parent peer. The generated client
// must keep IssueOut.Parent a pointer so a parentless IssueOut validates and
// round-trips without inventing an empty parent object — the regression that
// surfaces when an optional object response field regenerates as a value
// type (additionalProperties leaking onto the client-flavor schema).
func TestIssueOutWithoutParentValidatesAndOmitsParent(t *testing.T) {
	issue := generated.IssueOut{
		UID:         "01TESTISSUEAAAAAAAAAAAAAAA",
		ShortID:     "abc4",
		QualifiedID: "spoke-project#abc4",
		Title:       "parentless issue",
		Body:        "no parent here",
		Status:      "open",
		Author:      "tester",
		CreatedAt:   time.Unix(1, 0).UTC(),
		UpdatedAt:   time.Unix(1, 0).UTC(),
	}
	require.NoError(t, issue.Validate(),
		"a parentless issue must validate without a parent peer")

	out, err := json.Marshal(issue)
	require.NoError(t, err)
	var roundTrip map[string]jsontext.Value
	require.NoError(t, json.Unmarshal(out, &roundTrip))
	require.NotContains(t, roundTrip, "parent",
		"absent parent must not marshal as an empty object")
}

// A project with no issue sync binding returns status only. The generated
// client must keep IssueSyncBody.Binding optional so callers can represent
// the not_enabled state without inventing an empty binding object.

func TestMovePreviewResponseWithoutNewShortIDValidatesAndOmitsField(t *testing.T) {
	issue := generated.Issue{
		Author:    "tester",
		Body:      "preview only",
		CreatedAt: time.Unix(1, 0).UTC(),
		ShortID:   "abc4",
		Status:    "open",
		Title:     "move preview",
		UID:       "01TESTISSUEAAAAAAAAAAAAAAA",
		UpdatedAt: time.Unix(1, 0).UTC(),
	}
	response := generated.MoveIssueResponseBody{Issue: issue}
	require.NoError(t, response.Validate(),
		"a dry-run response has no allocated target short ID")

	out, err := json.Marshal(response)
	require.NoError(t, err)
	var roundTrip map[string]jsontext.Value
	require.NoError(t, json.Unmarshal(out, &roundTrip))
	require.NotContains(t, roundTrip, "new_short_id",
		"an absent target short ID must not marshal as an empty string")
}

// The run-once endpoint accepts an empty request object and returns a sync
// result object. The generated request options must point at the request-body
// schema so callers can naturally send {} without constructing a response
// body containing binding/status/import fields.
