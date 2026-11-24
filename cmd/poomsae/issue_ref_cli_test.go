package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestructiveCLI_MovedIssueConfirmation(t *testing.T) {
	for _, verb := range []string{"delete", "purge"} {
		for _, refKind := range []string{"alias", "ULID"} {
			t.Run(verb+"/"+refKind, func(t *testing.T) {
				env, dir, source, target, issue := setupMoveCLIProjects(t)
				runCLI(t, env, dir, "--project", source.Name, "move", issue.ShortID, target.Name)
				moved, err := env.DB.IssueByID(t.Context(), issue.ID)
				require.NoError(t, err)
				currentRef := target.Name + "#" + moved.ShortID
				if verb == "purge" {
					runCLI(t, env, dir, "delete", currentRef, "--force", "--confirm", "DELETE "+currentRef)
				}
				ref := source.Name + "#" + issue.ShortID
				if refKind == "ULID" {
					ref = issue.UID
				}

				resetFlags(t)
				stubIsTTY(t, true)
				cmd := newRootCmd()
				var output bytes.Buffer
				cmd.SetOut(&output)
				cmd.SetErr(&output)
				cmd.SetIn(strings.NewReader(strings.ToUpper(verb) + " " + currentRef + "\n"))
				cmd.SetContext(contextWithBaseURL(t.Context(), env.URL))
				cmd.SetArgs([]string{"--workspace", dir, "--project", source.Name, verb, ref, "--force"})
				require.NoError(t, cmd.Execute(), "confirmation must use the issue's current project")
				assert.Contains(t, output.String(), currentRef)
				stored, err := env.DB.IssueByID(t.Context(), issue.ID)
				if verb == "purge" {
					require.ErrorIs(t, err, db.ErrNotFound)
				} else {
					require.NoError(t, err)
					assert.NotNil(t, stored.DeletedAt)
				}
			})
		}
	}
}

func TestRefToWireForwardsQualifiedRefsOnlyForOtherProjects(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "bare short ID stays bare", ref: "abc4", want: "abc4"},
		{name: "same-project qualified ref collapses", ref: "poomsae#abc4", want: "abc4"},
		{name: "cross-project qualified ref stays qualified", ref: "other#abc4", want: "other#abc4"},
		{name: "ULID stays bare", ref: "01HZNQ7VFPK1XGD8R5MABCD4EX", want: "01HZNQ7VFPK1XGD8R5MABCD4EX"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := refToWire(tt.ref, "--related", "poomsae")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRefToWireWithoutCurrentProjectKeepsRefsBare(t *testing.T) {
	for _, ref := range []string{"abc4", "other#abc4"} {
		t.Run(ref, func(t *testing.T) {
			got, err := refToWire(ref, "--related", "")
			require.NoError(t, err)
			assert.Equal(t, "abc4", got)
		})
	}
}

func TestRefToWireRejectsEmptyAndLegacyNumericRefsWithTheFlagName(t *testing.T) {
	for _, ref := range []string{"", "  ", "12"} {
		t.Run(ref, func(t *testing.T) {
			_, err := refToWire(ref, "--blocked-by", "poomsae")
			var cliErr *cliError
			require.ErrorAs(t, err, &cliErr)
			assert.Equal(t, kindValidation, cliErr.Kind)
			assert.Equal(t, ExitValidation, cliErr.ExitCode)
			assert.Contains(t, cliErr.Message, "--blocked-by")
		})
	}
}

func TestRefsToWireKeepsOrderAndFailsTheWholeList(t *testing.T) {
	got, err := refsToWire([]string{"other#abc4", "def5", "poomsae#jkm6"}, "--blocks", "poomsae")
	require.NoError(t, err)
	assert.Equal(t, []string{"other#abc4", "def5", "jkm6"}, got)

	got, err = refsToWire(nil, "--blocks", "poomsae")
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = refsToWire([]string{"abc4", "12", "def5"}, "--blocks", "poomsae")
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestSingletonRefToWireAcceptsEquivalentFormsAndRejectsDistinctOnes(t *testing.T) {
	got, err := singletonRefToWire([]string{"abc4", "poomsae#abc4", "abc4"}, "--parent", "poomsae")
	require.NoError(t, err)
	assert.Equal(t, "abc4", got)

	got, err = singletonRefToWire(nil, "--parent", "poomsae")
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = singletonRefToWire([]string{"abc4", "other#abc4"}, "--parent", "poomsae")
	var cliErr *cliError
	require.ErrorAs(t, err, &cliErr)
	assert.Equal(t, kindValidation, cliErr.Kind)
	assert.Equal(t, ExitValidation, cliErr.ExitCode)
	assert.Contains(t, cliErr.Message, "--parent")
	assert.Contains(t, cliErr.Message, `"abc4"`)
	assert.Contains(t, cliErr.Message, `"other#abc4"`)
}

func TestAddAndRemoveRefsResolveIdentically(t *testing.T) {
	refs := []string{"abc4", "poomsae#def5", "other#jkm6"}

	add, err := refsToWire(refs, "--related", "poomsae")
	require.NoError(t, err)
	remove, err := refsToWire(refs, "--remove-related", "poomsae")
	require.NoError(t, err)

	assert.Equal(t, add, remove)
}
