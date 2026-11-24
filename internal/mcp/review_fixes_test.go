package mcpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	poomsaeclient "github.com/ifigueroap/poomsae/pkg/client"
	"github.com/ifigueroap/poomsae/pkg/client/generated"
)

func TestReadEventStreamAcceptsLargeEvents(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"body": strings.Repeat("x", 70<<10), "link_to_uid": "foreign-uid", "type": "related",
	})
	require.NoError(t, err)
	event := generated.EventEnvelope{
		EventID: 1, EventUID: "01HAAAAAAAAAAAAAAAAAAAAAAA", Type: "links_changed",
		Actor: "example-agent", CreatedAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
		ProjectID: 1, ProjectUID: "01HBBBBBBBBBBBBBBBBBBBBBBB", ProjectName: "spoke-project",
		Payload: payload, RelatedIssueUID: optionalString("foreign-uid"), RelatedIssueShortID: optionalString("out1"),
	}
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	stream := "event: links_changed\ndata: " + string(encoded) + "\n\n"

	output, err := readEventStream(t.Context(), strings.NewReader(stream), 0, 1)
	require.NoError(t, err)
	require.Len(t, output.Events, 1)
	require.Equal(t, "foreign-uid", output.Events[0].RelatedIssueUID)
	decoded := output.Events[0].Payload.(map[string]any)
	require.Len(t, decoded["body"], 70<<10)
}

func TestEventPeerRedactionPreservesAllowedPeers(t *testing.T) {
	allowedUID := "01HCCCCCCCCCCCCCCCCCCCCCCC"
	foreignUID := "01HDDDDDDDDDDDDDDDDDDDDDDD"
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, "01HAAAAAAAAAAAAAAAAAAAAAAA", "spoke-project"),
				projectJSON(2, "01HBBBBBBBBBBBBBBBBBBBBBBB", "other-project"),
			}})
		case strings.HasPrefix(request.URL.Path, "/api/v1/issues/"):
			uid := strings.TrimPrefix(request.URL.Path, "/api/v1/issues/")
			projectID, projectName := int64(1), "spoke-project"
			if uid == foreignUID {
				projectID, projectName = 2, "other-project"
			}
			issue := issueJSON(projectID, projectName, "peer1")
			issue["uid"] = uid
			writeJSON(writer, map[string]any{
				"issue": issue, "labels": []any{}, "comments": []any{}, "links": []any{},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	scope, err := NewAllowlistScope([]ProjectIdentity{{
		ID: 1, UID: "01HAAAAAAAAAAAAAAAAAAAAAAA", Name: "spoke-project",
	}})
	require.NoError(t, err)
	handlers := toolHandlers{options: Options{Client: client, Scope: scope}}
	output := EventsOutput{Events: []StreamEvent{
		{RelatedIssueUID: allowedUID, RelatedIssueShortID: "in1", Payload: map[string]any{"link_to_uid": allowedUID}},
		{RelatedIssueUID: foreignUID, RelatedIssueShortID: "out1", Payload: map[string]any{"link_to_uid": foreignUID}},
	}}

	require.NoError(t, handlers.redactEventsOutsideScope(t.Context(), &output))
	require.Equal(t, allowedUID, output.Events[0].RelatedIssueUID)
	require.Equal(t, allowedUID, output.Events[0].Payload.(map[string]any)["link_to_uid"])
	require.Empty(t, output.Events[1].RelatedIssueUID)
	require.Empty(t, output.Events[1].RelatedIssueShortID)
	require.NotContains(t, output.Events[1].Payload.(map[string]any), "link_to_uid")
}

func TestEventPeerRedactionPropagatesTransientLookupFailure(t *testing.T) {
	peerUID := "01HCCCCCCCCCCCCCCCCCCCCCCC"
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, "01HAAAAAAAAAAAAAAAAAAAAAAA", "spoke-project"),
			}})
		case "/api/v1/issues/" + peerUID:
			writer.WriteHeader(http.StatusInternalServerError)
			writeJSON(writer, map[string]any{
				"status": http.StatusInternalServerError,
				"error":  map[string]any{"code": "internal_error", "message": "try again"},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	scope, err := NewAllowlistScope([]ProjectIdentity{{
		ID: 1, UID: "01HAAAAAAAAAAAAAAAAAAAAAAA", Name: "spoke-project",
	}})
	require.NoError(t, err)
	handlers := toolHandlers{options: Options{Client: client, Scope: scope}}
	output := EventsOutput{Events: []StreamEvent{{
		RelatedIssueUID: peerUID, RelatedIssueShortID: "in1",
		Payload: map[string]any{"link_to_uid": peerUID},
	}}}

	err = handlers.redactEventsOutsideScope(t.Context(), &output)
	require.ErrorContains(t, err, "resolve event peer")
	require.NotContains(t, err.Error(), peerUID)
	require.NotContains(t, err.Error(), "try again")
	require.Equal(t, peerUID, output.Events[0].RelatedIssueUID)
	require.Equal(t, peerUID, output.Events[0].Payload.(map[string]any)["link_to_uid"])
}

func TestEventPeerRedactionCoversParentLinkAndArrayShapes(t *testing.T) {
	allowedUID := "01HCCCCCCCCCCCCCCCCCCCCCCC"
	foreignUID := "01HDDDDDDDDDDDDDDDDDDDDDDD"
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, "01HAAAAAAAAAAAAAAAAAAAAAAA", "spoke-project"),
			}})
		case strings.HasPrefix(request.URL.Path, "/api/v1/issues/"):
			uid := strings.TrimPrefix(request.URL.Path, "/api/v1/issues/")
			projectID, projectName := int64(1), "spoke-project"
			if uid == foreignUID {
				projectID, projectName = 2, "other-project"
			}
			issue := issueJSON(projectID, projectName, "peer1")
			issue["uid"] = uid
			writeJSON(writer, map[string]any{
				"issue": issue, "labels": []any{}, "comments": []any{}, "links": []any{},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	scope, err := NewAllowlistScope([]ProjectIdentity{{
		ID: 1, UID: "01HAAAAAAAAAAAAAAAAAAAAAAA", Name: "spoke-project",
	}})
	require.NoError(t, err)
	handlers := toolHandlers{options: Options{Client: client, Scope: scope}}
	output := EventsOutput{Events: []StreamEvent{
		{Payload: map[string]any{
			"parent_set": "in1", "parent_set_uid": allowedUID,
			"blocks_added": []any{"in1", "out1"}, "blocks_added_uids": []any{allowedUID, foreignUID},
			"links": []any{map[string]any{"type": "blocks", "to_short_id": "out1", "to_issue_uid": foreignUID}},
		}},
		{Payload: map[string]any{
			"parent_removed": "out1", "parent_removed_uid": foreignUID,
			"related_added": []any{"out1"}, "related_added_uids": []any{foreignUID},
		}},
	}}

	require.NoError(t, handlers.redactEventsOutsideScope(t.Context(), &output))
	first := output.Events[0].Payload.(map[string]any)
	require.Equal(t, allowedUID, first["parent_set_uid"])
	require.Equal(t, "in1", first["parent_set"])
	require.Equal(t, []any{allowedUID}, first["blocks_added_uids"])
	require.Equal(t, []any{"in1"}, first["blocks_added"])
	link := first["links"].([]any)[0].(map[string]any)
	require.NotContains(t, link, "to_issue_uid")
	require.NotContains(t, link, "to_short_id")
	second := output.Events[1].Payload.(map[string]any)
	require.NotContains(t, second, "parent_removed_uid")
	require.NotContains(t, second, "parent_removed")
	require.NotContains(t, second, "related_added_uids")
	require.NotContains(t, second, "related_added")
}

func TestEventRedactionCoversClosedParentFields(t *testing.T) {
	foreignUID := "01HDDDDDDDDDDDDDDDDDDDDDDD"
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, "01HAAAAAAAAAAAAAAAAAAAAAAA", "spoke-project"),
			}})
		case strings.HasPrefix(request.URL.Path, "/api/v1/issues/"):
			issue := issueJSON(2, "other-project", "peer1")
			issue["uid"] = foreignUID
			writeJSON(writer, map[string]any{
				"issue": issue, "labels": []any{}, "comments": []any{}, "links": []any{},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	scope, err := NewAllowlistScope([]ProjectIdentity{{
		ID: 1, UID: "01HAAAAAAAAAAAAAAAAAAAAAAA", Name: "spoke-project",
	}})
	require.NoError(t, err)
	handlers := toolHandlers{options: Options{Client: client, Scope: scope}}
	output := EventsOutput{Events: []StreamEvent{{
		Type:    "issue.closed",
		Payload: map[string]any{"reason": "done", "parent_uid": foreignUID, "parent_short_id": "p1"},
	}}}

	require.NoError(t, handlers.redactEventsOutsideScope(t.Context(), &output))
	payload := output.Events[0].Payload.(map[string]any)
	require.NotContains(t, payload, "parent_uid")
	require.NotContains(t, payload, "parent_short_id")
	require.Equal(t, "done", payload["reason"])
}

func TestDigestRedactsForeignLinkActionTargets(t *testing.T) {
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, "01HAAAAAAAAAAAAAAAAAAAAAAA", "spoke-project"),
			}})
		case "/api/v1/projects/1/digest":
			writeJSON(writer, map[string]any{
				"project_id": 1, "event_count": 2,
				"since": "2026-08-11T00:00:00Z", "until": "2026-08-12T00:00:00Z",
				"totals": map[string]any{},
				"actors": []any{map[string]any{
					"actor": "example-agent", "totals": map[string]any{},
					"issues": []any{map[string]any{
						"issue_uid": "01HCCCCCCCCCCCCCCCCCCCCCCC", "issue_short_id": "abc1",
						"project_id": 1, "project_name": "spoke-project",
						"actions": []any{"created", "labeled:bug", "blocks:def2", "unparent:zz9", "related:xyz9", "commented:2"},
					}},
				}},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	scope, err := NewAllowlistScope([]ProjectIdentity{{
		ID: 1, UID: "01HAAAAAAAAAAAAAAAAAAAAAAA", Name: "spoke-project",
	}})
	require.NoError(t, err)
	handlers := toolHandlers{options: Options{Client: client, Scope: scope}}

	_, output, err := handlers.digest(t.Context(), nil, DigestInput{Project: "spoke-project", Since: "2026-08-11T00:00:00Z"})
	require.NoError(t, err)
	require.Len(t, output.Digests, 1)
	actions := output.Digests[0].Digest.Actors[0].Issues[0].Actions
	require.Equal(t, []string{"created", "labeled:bug", "blocks", "unparent", "related", "commented:2"}, actions,
		"historical link tokens carry no provable identity, so every target is stripped to its action type; a current same-short-ID link must not vouch a historical peer")
}

func TestAllProjectActivityUsesOnlyActiveProjectEndpoints(t *testing.T) {
	const (
		activeProjectUID   = "01HAAAAAAAAAAAAAAAAAAAAAAA"
		archivedProjectUID = "01HBBBBBBBBBBBBBBBBBBBBBBB"
	)
	activeEvent := reviewActivityEventJSON(7, 1, activeProjectUID, "active-project")
	archivedEvent := reviewActivityEventJSON(8, 2, archivedProjectUID, "archived-project")
	archivedStreamEvent, err := json.Marshal(archivedEvent)
	require.NoError(t, err)
	var globalRequests atomic.Int64
	var projectDigestRequests atomic.Int64
	var projectEventRequests atomic.Int64
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, activeProjectUID, "active-project"),
			}})
		case "/api/v1/projects/1/digest":
			projectDigestRequests.Add(1)
			writeJSON(writer, reviewDigestBodyJSON(1, "active-project"))
		case "/api/v1/projects/1/events":
			projectEventRequests.Add(1)
			writeJSON(writer, map[string]any{"events": []any{activeEvent}, "next_after_id": 7, "reset_required": false})
		case "/api/v1/digest":
			globalRequests.Add(1)
			writeJSON(writer, reviewDigestBodyJSON(2, "archived-project"))
		case "/api/v1/events":
			globalRequests.Add(1)
			writeJSON(writer, map[string]any{"events": []any{archivedEvent}, "next_after_id": 8, "reset_required": false})
		case "/api/v1/events/stream":
			globalRequests.Add(1)
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(writer, "event: issue.edited\ndata: %s\n\n", archivedStreamEvent)
		default:
			http.NotFound(writer, request)
		}
	})
	handlers := toolHandlers{options: Options{Client: client, LongRunningClient: client, Scope: NewAllScope()}}

	_, digest, err := handlers.digest(t.Context(), nil, DigestInput{Since: "2026-08-11T00:00:00Z"})
	require.NoError(t, err)
	require.Len(t, digest.Digests, 1)
	require.NotNil(t, digest.Digests[0].Project)
	require.Equal(t, "active-project", digest.Digests[0].Project.Name)
	require.Equal(t, "active-project", digest.Digests[0].Digest.Actors[0].Issues[0].ProjectName)

	_, polled, err := handlers.events(t.Context(), nil, EventsInput{Mode: "poll"})
	require.NoError(t, err)
	require.Len(t, polled.Events, 1)
	require.Equal(t, activeProjectUID, polled.Events[0].ProjectUID)

	_, waited, err := handlers.events(t.Context(), nil, EventsInput{Mode: "wait", WaitSeconds: 1})
	require.NoError(t, err)
	require.Len(t, waited.Events, 1)
	require.Equal(t, activeProjectUID, waited.Events[0].ProjectUID)
	require.Zero(t, globalRequests.Load(), "all-project activity must not use archive-inclusive global endpoints")
	require.EqualValues(t, 1, projectDigestRequests.Load())
	require.EqualValues(t, 2, projectEventRequests.Load())
}

func TestAllProjectEventPollToleratesProjectArchivedAfterCatalogRead(t *testing.T) {
	const activeProjectUID = "01HAAAAAAAAAAAAAAAAAAAAAAA"
	activeEvent := reviewActivityEventJSON(7, 1, activeProjectUID, "active-project")
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, activeProjectUID, "active-project"),
				projectJSON(2, "01HBBBBBBBBBBBBBBBBBBBBBBB", "soon-archived-project"),
			}})
		case "/api/v1/projects/1/events":
			writeJSON(writer, map[string]any{"events": []any{activeEvent}, "next_after_id": 7, "reset_required": false})
		case "/api/v1/projects/2/events":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusNotFound)
			writeJSON(writer, map[string]any{
				"status": 404, "error": map[string]any{"code": "project_not_found", "message": "project was archived"},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	handlers := toolHandlers{options: Options{Client: client, Scope: NewAllScope()}}

	output, err := handlers.pollScopedEvents(t.Context(), "", 0, 20)
	require.NoError(t, err)
	require.Len(t, output.Events, 1)
	require.Equal(t, activeProjectUID, output.Events[0].ProjectUID)
}

func TestAllProjectDigestToleratesProjectArchivedAfterCatalogRead(t *testing.T) {
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, "01HAAAAAAAAAAAAAAAAAAAAAAA", "active-project"),
				projectJSON(2, "01HBBBBBBBBBBBBBBBBBBBBBBB", "soon-archived-project"),
			}})
		case "/api/v1/projects/1/digest":
			writeJSON(writer, reviewDigestBodyJSON(1, "active-project"))
		case "/api/v1/projects/2/digest":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusNotFound)
			writeJSON(writer, map[string]any{
				"status": 404, "error": map[string]any{"code": "project_not_found", "message": "project was archived"},
			})
		default:
			http.NotFound(writer, request)
		}
	})
	handlers := toolHandlers{options: Options{Client: client, Scope: NewAllScope()}}

	_, output, err := handlers.digest(t.Context(), nil, DigestInput{Since: "2026-08-11T00:00:00Z"})
	require.NoError(t, err)
	require.Len(t, output.Digests, 1)
	require.NotNil(t, output.Digests[0].Project)
	require.Equal(t, "active-project", output.Digests[0].Project.Name)
}

func TestAuditPaginationInvalidatesWhenHistoryBelowCursorChanges(t *testing.T) {
	auditRow := func(eventID int, issue string) map[string]any {
		return map[string]any{"time": "2026-08-12T00:00:00Z", "actor": "example-agent", "reason": "done", "issue": issue, "event_id": eventID}
	}
	merged := false
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/projects":
			writeJSON(writer, map[string]any{"projects": []any{
				projectJSON(1, "01HAAAAAAAAAAAAAAAAAAAAAAA", "spoke-project"),
			}})
		case "/api/v1/audit/closes":
			rows := []any{auditRow(11, "abc1"), auditRow(14, "def2"), auditRow(15, "ghi3")}
			if merged {
				// A project merge rehomed an older close event whose ID
				// falls below the first page's cursor.
				rows = []any{auditRow(11, "abc1"), auditRow(12, "hij4"), auditRow(14, "def2"), auditRow(15, "ghi3")}
			}
			writeJSON(writer, map[string]any{"rows": rows})
		default:
			http.NotFound(writer, request)
		}
	})
	scope, err := NewAllowlistScope([]ProjectIdentity{{
		ID: 1, UID: "01HAAAAAAAAAAAAAAAAAAAAAAA", Name: "spoke-project",
	}})
	require.NoError(t, err)
	handlers := toolHandlers{options: Options{Client: client, Scope: scope}}

	_, first, err := handlers.auditCloses(t.Context(), nil, AuditClosesInput{Project: "spoke-project", Limit: 2})
	require.NoError(t, err)
	require.True(t, first.Truncated)
	require.NotNil(t, first.NextCursor)

	merged = true
	_, _, err = handlers.auditCloses(t.Context(), nil, AuditClosesInput{Project: "spoke-project", Limit: 2, Cursor: *first.NextCursor})
	require.ErrorContains(t, err, "restart without a cursor",
		"history merged in below the cursor must invalidate pagination, not be skipped")

	merged = false
	_, resumed, err := handlers.auditCloses(t.Context(), nil, AuditClosesInput{Project: "spoke-project", Limit: 2, Cursor: *first.NextCursor})
	require.NoError(t, err, "an unchanged prefix keeps the cursor valid")
	require.Len(t, resumed.Rows, 1)
	require.Equal(t, "ghi3", resumed.Rows[0].Issue)
}

func TestScopedServersCannotAdministerProjects(t *testing.T) {
	var requests int
	client := reviewClient(t, func(writer http.ResponseWriter, request *http.Request) {
		requests++
		http.NotFound(writer, request)
	})
	scope, err := NewBoundScope(ProjectIdentity{ID: 1, Name: "spoke-project"})
	require.NoError(t, err)
	handlers := toolHandlers{options: Options{Client: client, Scope: scope}}

	_, _, err = handlers.projectRemove(t.Context(), nil, ProjectRemoveInput{Project: "spoke-project"})
	require.ErrorContains(t, err, "requires the --all daemon-wide scope")
	_, _, err = handlers.projectPurge(t.Context(), nil, ProjectPurgeInput{Project: "spoke-project", Confirm: "PURGE spoke-project"})
	require.ErrorContains(t, err, "requires the --all daemon-wide scope")
	_, _, err = handlers.projectUpdate(t.Context(), nil, ProjectUpdateInput{Project: "spoke-project", Action: "rename", Name: "renamed"})
	require.ErrorContains(t, err, "requires the --all daemon-wide scope")
	_, _, err = handlers.projectMerge(t.Context(), nil, ProjectMergeInput{Source: "spoke-project", Target: "spoke-project"})
	require.ErrorContains(t, err, "requires the --all daemon-wide scope")
	_, _, err = handlers.projectRestore(t.Context(), nil, ProjectRestoreInput{Project: "spoke-project"})
	require.ErrorContains(t, err, "requires the --all daemon-wide scope")
	require.Zero(t, requests, "scoped project administration must fail before any daemon request")
}

func reviewClient(t *testing.T, handler http.HandlerFunc) *poomsaeclient.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := poomsaeclient.NewWithHTTPClient(server.URL, server.Client())
	require.NoError(t, err)
	return client
}
func reviewActivityEventJSON(eventID, projectID int64, projectUID, projectName string) map[string]any {
	return map[string]any{
		"actor": "example-agent", "content_hash": "hash", "created_at": "2026-08-12T00:00:00Z",
		"event_id": eventID, "event_uid": fmt.Sprintf("event-%d", eventID), "hlc_counter": 0,
		"hlc_physical_ms": 1, "origin_instance_uid": "01HCCCCCCCCCCCCCCCCCCCCCCC",
		"payload": map[string]any{"body": projectName + " body"}, "project_id": projectID,
		"project_name": projectName, "project_uid": projectUID, "type": "issue.edited",
	}
}

func reviewDigestBodyJSON(projectID int64, projectName string) map[string]any {
	return map[string]any{
		"project_id": projectID, "event_count": 1,
		"since": "2026-08-11T00:00:00Z", "until": "2026-08-12T00:00:00Z",
		"totals": map[string]any{},
		"actors": []any{map[string]any{
			"actor": "example-agent", "totals": map[string]any{},
			"issues": []any{map[string]any{
				"issue_uid": "01HDDDDDDDDDDDDDDDDDDDDDDD", "issue_short_id": "abc1",
				"project_id": projectID, "project_name": projectName, "actions": []any{"edited"},
			}},
		}},
	}
}

