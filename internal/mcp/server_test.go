package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/ifigueroap/poomsae/internal/daemon"
	"github.com/ifigueroap/poomsae/internal/db"
	"github.com/ifigueroap/poomsae/internal/db/sqlitestore"
	"github.com/ifigueroap/poomsae/internal/storageadmin"
	poomsaeclient "github.com/ifigueroap/poomsae/pkg/client"
	"github.com/ifigueroap/poomsae/pkg/client/generated"
)

var sectionLoaderNames = []string{
	"poomsae.load_activity",
	"poomsae.load_coordination",
	"poomsae.load_cron",
	"poomsae.load_docs",
	"poomsae.load_external_roots",
	"poomsae.load_federation",
	"poomsae.load_import",
	"poomsae.load_issue_discovery",
	"poomsae.load_issue_lifecycle",
	"poomsae.load_issue_mutation",
	"poomsae.load_leases",
	"poomsae.load_projects",
	"poomsae.load_recurrence",
	"poomsae.load_storage",
	"poomsae.load_sync",
	"poomsae.load_system",
	"poomsae.load_teams",
	"poomsae.load_tokens",
}

func TestServerPublishesOnlySectionLoadersInitially(t *testing.T) {
	session := connectRawTestServerWithOptions(t, Options{
		Client: inertTestClient(t), ProjectID: 42, ProjectName: "spoke-project",
		Actor: "example-agent", Version: "test-version",
	})

	require.True(t, session.InitializeResult().Capabilities.Tools.ListChanged)
	result, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, append(append([]string(nil), sectionLoaderNames...), "poomsae.search_tools", "poomsae.skills"), toolNames(result.Tools))
	for _, tool := range result.Tools {
		require.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
		require.False(t, *tool.Annotations.DestructiveHint, tool.Name)
	}
}

func TestServerPublishesStartupSectionsInitially(t *testing.T) {
	admin, err := storageadmin.New(storageadmin.Config{Root: t.TempDir(), SourceDSN: "source.db"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	for _, section := range toolSections(true, true) {
		t.Run(section.section, func(t *testing.T) {
			options := Options{
				Client: inertTestClient(t), Scope: NewAllScope(), Actor: "example-agent", Version: "test-version",
				EnableTokenAdmin: true, StorageAdmin: admin,
			}
			progressive := connectRawTestServerWithOptions(t, options)
			loaded, err := progressive.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: section.loader})
			require.NoError(t, err)
			require.False(t, loaded.IsError)
			want, err := progressive.ListTools(t.Context(), nil)
			require.NoError(t, err)

			options.StartupSections = []string{section.section}
			startup := connectRawTestServerWithOptions(t, options)
			got, err := startup.ListTools(t.Context(), nil)
			require.NoError(t, err)
			require.Equal(t, want.Tools, got.Tools)
		})
	}
	t.Run("combined sections", func(t *testing.T) {
		session := connectRawTestServerWithOptions(t, Options{
			Client: inertTestClient(t), ProjectID: 42, ProjectName: "spoke-project",
			Actor: "example-agent", Version: "test-version",
			StartupSections: []string{"issue_discovery", "issue_mutation", "coordination", "issue_lifecycle", "projects", "docs"},
		})

		initial, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		progressive := connectRawTestServerWithOptions(t, Options{
			Client: inertTestClient(t), ProjectID: 42, ProjectName: "spoke-project",
			Actor: "example-agent", Version: "test-version",
		})
		for _, section := range []string{"issue_discovery", "issue_mutation", "coordination", "issue_lifecycle", "projects", "docs"} {
			result, err := progressive.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "poomsae.load_" + section})
			require.NoError(t, err)
			require.False(t, result.IsError)
		}
		want, err := progressive.ListTools(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, want.Tools, initial.Tools)
	})
}

func TestServerRejectsInvalidStartupSections(t *testing.T) {
	for _, tt := range []struct {
		name     string
		sections []string
		want     string
	}{
		{name: "unknown", sections: []string{"docs", "missing"}, want: `unknown MCP startup section "missing"`},
		{name: "loader name", sections: []string{"poomsae.load_docs"}, want: `unknown MCP startup section "poomsae.load_docs"`},
		{name: "empty ID", sections: []string{""}, want: `unknown MCP startup section ""`},
		{name: "whitespace ID", sections: []string{" docs "}, want: `unknown MCP startup section " docs "`},
		{name: "case variant", sections: []string{"Docs"}, want: `unknown MCP startup section "Docs"`},
		{name: "duplicate", sections: []string{"docs", "docs"}, want: `duplicate MCP startup section "docs"`},
		{name: "storage unavailable", sections: []string{"storage"}, want: `MCP startup section "storage" is unavailable`},
		{name: "tokens disabled", sections: []string{"tokens"}, want: `MCP startup section "tokens" is unavailable`},
		{name: "teams disabled", sections: []string{"teams"}, want: `MCP startup section "teams" is unavailable`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, err := New(Options{
				Client: inertTestClient(t), Scope: NewAllScope(), Actor: "example-agent", Version: "test-version",
				StartupSections: tt.sections,
			})
			require.ErrorContains(t, err, tt.want)
			require.Nil(t, server)
		})
	}
}

func TestSectionLoaderActivatesOnlyItsTypedToolsAndIsIdempotent(t *testing.T) {
	session := connectRawTestServerWithOptions(t, Options{
		Client: inertTestClient(t), ProjectID: 42, ProjectName: "spoke-project",
		Actor: "example-agent", Version: "test-version",
	})

	for range 2 {
		result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
			Name: "poomsae.load_activity", Arguments: map[string]any{},
		})
		require.NoError(t, err)
		require.False(t, result.IsError)
		var output ToolSectionOutput
		require.NoError(t, json.Unmarshal(mustJSON(t, result.StructuredContent), &output))
		require.Equal(t, ToolSectionOutput{
			Section: "activity", Available: true, Loaded: true,
			Tools: []string{"poomsae.digest", "poomsae.events"},
		}, output)
	}

	result, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	want := append(append([]string(nil), sectionLoaderNames...), "poomsae.search_tools", "poomsae.skills", "poomsae.digest", "poomsae.events")
	sort.Strings(want)
	require.Equal(t, want, toolNames(result.Tools))

	t.Run("preloaded sections", func(t *testing.T) {
		changed := make(chan struct{}, 1)
		session := connectRawTestServerWithClientOptions(t, Options{
			Client: inertTestClient(t), ProjectID: 42, ProjectName: "spoke-project",
			Actor: "example-agent", Version: "test-version",
			StartupSections: []string{"issue_discovery", "issue_mutation", "coordination", "issue_lifecycle", "projects", "docs"},
		}, &sdkmcp.ClientOptions{ToolListChangedHandler: func(context.Context, *sdkmcp.ToolListChangedRequest) {
			select {
			case changed <- struct{}{}:
			default:
			}
		}})

		initial, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		for range 2 {
			result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "poomsae.load_issue_discovery"})
			require.NoError(t, err)
			require.False(t, result.IsError)
			var output ToolSectionOutput
			require.NoError(t, json.Unmarshal(mustJSON(t, result.StructuredContent), &output))
			require.Equal(t, ToolSectionOutput{
				Section: "issue_discovery", Available: true, Loaded: true,
				Tools: []string{"poomsae.graph", "poomsae.labels", "poomsae.list", "poomsae.next", "poomsae.ready", "poomsae.search", "poomsae.show"},
			}, output)
		}
		after, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, initial.Tools, after.Tools)
		result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "poomsae.load_activity"})
		require.NoError(t, err)
		require.False(t, result.IsError)
		// The SDK caches catalogs until it receives the subscribed change notification.
		select {
		case <-changed:
		case <-time.After(time.Second):
			t.Fatal("MCP catalog change notification did not arrive")
		}
		after, err = session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		wantNames := append(toolNames(initial.Tools), "poomsae.digest", "poomsae.events")
		sort.Strings(wantNames)
		require.Equal(t, wantNames, toolNames(after.Tools))
	})
}

func TestTokenSectionRequiresExplicitStartupCapability(t *testing.T) {
	session := connectRawTestServerWithOptions(t, Options{
		Client: inertTestClient(t), Scope: NewAllScope(),
		Actor: "example-agent", Version: "test-version",
	})

	result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.load_tokens", Arguments: map[string]any{},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	var output ToolSectionOutput
	require.NoError(t, json.Unmarshal(mustJSON(t, result.StructuredContent), &output))
	require.False(t, output.Available)
	require.False(t, output.Loaded)

	enabled := connectRawTestServerWithOptions(t, Options{
		Client: inertTestClient(t), Scope: NewAllScope(), EnableTokenAdmin: true,
		Actor: "example-agent", Version: "test-version",
	})
	result, err = enabled.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.load_tokens", Arguments: map[string]any{},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NoError(t, json.Unmarshal(mustJSON(t, result.StructuredContent), &output))
	require.True(t, output.Available)
	require.True(t, output.Loaded)
}

func TestServerToolOutputSchemasAreObjects(t *testing.T) {
	admin, err := storageadmin.New(storageadmin.Config{Root: t.TempDir(), SourceDSN: "source.db"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	session := connectTestServerWithOptions(t, Options{
		Client: inertTestClient(t), Scope: NewAllScope(), EnableTokenAdmin: true, StorageAdmin: admin,
		Actor: "example-agent", Version: "test-version",
	})
	result, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Tools)
	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			// MCP 2025-11-25 clients require an object root in outputSchema.
			require.Equal(t, "object", schemaObject(t, tool.OutputSchema)["type"])
		})
	}
}

func TestServerPublishesCurrentToolsOnly(t *testing.T) {
	session := connectTestServerWithOptions(t, Options{
		Client: inertTestClient(t), Scope: NewAllScope(), EnableTokenAdmin: true,
		Actor: "example-agent", Version: "test-version",
	})

	initialized := session.InitializeResult()
	require.Equal(t, currentProtocolVersion, initialized.ProtocolVersion)
	require.NotNil(t, initialized.ServerInfo)
	require.Equal(t, "poomsae", initialized.ServerInfo.Name)
	require.Equal(t, "test-version", initialized.ServerInfo.Version)
	require.NotNil(t, initialized.Capabilities.Tools)
	require.True(t, initialized.Capabilities.Tools.ListChanged)
	require.Nil(t, initialized.Capabilities.Prompts)
	require.Nil(t, initialized.Capabilities.Resources)
	capabilities := string(mustJSON(t, initialized.Capabilities))
	require.NotContains(t, capabilities, `"logging"`)

	result, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, 5*time.Minute.Milliseconds(), int64(result.TTLMs))
	require.Equal(t, "private", result.CacheScope)

	wantNames := []string{
		"poomsae.assign",
		"poomsae.audit_closes",
		"poomsae.bridge_bind",
		"poomsae.bridge_pause",
		"poomsae.bridge_reconcile",
		"poomsae.bridge_resolve_comment",
		"poomsae.bridge_resolve_field",
		"poomsae.bridge_resume",
		"poomsae.bridge_show",
		"poomsae.bridge_unbind",
		"poomsae.claim",
		"poomsae.close",
		"poomsae.comment",
		"poomsae.connector_field_map",
		"poomsae.connector_field_unmap",
		"poomsae.connector_fields",
		"poomsae.connectors",
		"poomsae.create",
		"poomsae.cron_capabilities",
		"poomsae.cron_job",
		"poomsae.cron_job_create",
		"poomsae.cron_job_delete",
		"poomsae.cron_job_restore",
		"poomsae.cron_job_update",
		"poomsae.cron_jobs",
		"poomsae.cron_new_uid",
		"poomsae.cron_workflows",
		"poomsae.cron_workflow",
		"poomsae.cron_workflow_create",
		"poomsae.cron_workflow_update",
		"poomsae.cron_workflow_delete",
		"poomsae.cron_workflow_restore",
		"poomsae.cron_runs",
		"poomsae.cron_run",
		"poomsae.cron_run_observe",
		"poomsae.delete",
		"poomsae.digest",
		"poomsae.edit",
		"poomsae.edit_comment",
		"poomsae.events",
		"poomsae.federation_bridge_connect",
		"poomsae.federation_bridge_disconnect",
		"poomsae.federation_bridge_status",
		"poomsae.federation_enrollment_revoke",
		"poomsae.federation_leave",
		"poomsae.federation_quarantine",
		"poomsae.federation_rebind",
		"poomsae.federation_root_key_repin",
		"poomsae.federation_spoke_name",
		"poomsae.federation_status",
		"poomsae.graph",
		"poomsae.import_issues",
		"poomsae.inbox",
		"poomsae.labels",
		"poomsae.lease",
		"poomsae.lease_force_release",
		"poomsae.lease_status",
		"poomsae.lease_steal",
		"poomsae.list",
		"poomsae.move",
		"poomsae.next",
		"poomsae.project_create",
		"poomsae.project_merge",
		"poomsae.project_purge",
		"poomsae.project_remove",
		"poomsae.project_restore",
		"poomsae.project_show",
		"poomsae.project_update",
		"poomsae.projects",
		"poomsae.purge",
		"poomsae.read_doc",
		"poomsae.ready",
		"poomsae.recurrence_delete",
		"poomsae.recurrence_update",
		"poomsae.recurrences",
		"poomsae.reopen",
		"poomsae.restore",
		"poomsae.search",
		"poomsae.search_docs",
		"poomsae.set_deadline",
		"poomsae.set_label",
		"poomsae.set_metadata",
		"poomsae.set_schedule",
		"poomsae.show",
		"poomsae.status",
		"poomsae.sync_once",
		"poomsae.sync_status",
		"poomsae.sync_update",
		"poomsae.system",
		"poomsae.project_access_set",
		"poomsae.project_access_show",
		"poomsae.team_create",
		"poomsae.team_delete",
		"poomsae.team_member_set",
		"poomsae.team_show",
		"poomsae.teams",
		"poomsae.token_create",
		"poomsae.token_revoke",
		"poomsae.tokens",
		"poomsae.unassign",
		"poomsae.wait",
	}
	wantNames = append(wantNames, "poomsae.search_tools", "poomsae.skills")
	wantNames = append(wantNames, sectionLoaderNames...)
	sort.Strings(wantNames)
	gotNames := make([]string, 0, len(result.Tools))
	projectSelectors := map[string]bool{
		"poomsae.cron_workflows": true, "poomsae.cron_workflow": true, "poomsae.cron_workflow_create": true, "poomsae.cron_workflow_update": true, "poomsae.cron_workflow_delete": true, "poomsae.cron_workflow_restore": true, "poomsae.cron_runs": true, "poomsae.cron_run": true, "poomsae.cron_run_observe": true,
		"poomsae.cron_new_uid": true, "poomsae.cron_capabilities": true, "poomsae.cron_jobs": true, "poomsae.cron_job": true, "poomsae.cron_job_create": true, "poomsae.cron_job_update": true, "poomsae.cron_job_delete": true, "poomsae.cron_job_restore": true,
		"poomsae.federation_bridge_connect": true, "poomsae.federation_bridge_disconnect": true, "poomsae.federation_bridge_status": true,
		"poomsae.project_access_set": true, "poomsae.project_access_show": true,
		"poomsae.audit_closes": true,
		"poomsae.create":       true, "poomsae.digest": true, "poomsae.events": true,
		"poomsae.federation_leave": true, "poomsae.federation_quarantine": true, "poomsae.federation_rebind": true,
		"poomsae.federation_root_key_repin": true,
		"poomsae.import_issues":             true, "poomsae.inbox": true, "poomsae.labels": true, "poomsae.list": true, "poomsae.next": true,
		"poomsae.project_purge": true, "poomsae.project_remove": true, "poomsae.project_restore": true,
		"poomsae.project_show": true, "poomsae.project_update": true, "poomsae.projects": true, "poomsae.ready": true,
		"poomsae.recurrence_delete": true, "poomsae.recurrence_update": true, "poomsae.recurrences": true,
		"poomsae.search": true, "poomsae.sync_once": true, "poomsae.sync_status": true, "poomsae.sync_update": true,
	}
	openWorld := map[string]bool{
		"poomsae.federation_bridge_connect": true, "poomsae.federation_bridge_disconnect": true, "poomsae.federation_bridge_status": true,
		"poomsae.bridge_bind":                  true,
		"poomsae.bridge_reconcile":             true,
		"poomsae.bridge_resolve_comment":       true,
		"poomsae.bridge_resolve_field":         true,
		"poomsae.bridge_resume":                true,
		"poomsae.connector_field_map":          true,
		"poomsae.connector_field_unmap":        true,
		"poomsae.connector_fields":             true,
		"poomsae.connectors":                   true,
		"poomsae.federation_enrollment_revoke": true,
		"poomsae.federation_spoke_name":        true,
		"poomsae.federation_leave":             true, "poomsae.federation_quarantine": true,
		"poomsae.federation_rebind": true, "poomsae.federation_status": true, "poomsae.import_issues": true,
		"poomsae.search": true, "poomsae.sync_once": true, "poomsae.sync_status": true, "poomsae.sync_update": true,
	}
	for _, tool := range result.Tools {
		gotNames = append(gotNames, tool.Name)
		require.NotEmpty(t, tool.Title, tool.Name)
		require.NotEmpty(t, tool.Description, tool.Name)

		input := schemaObject(t, tool.InputSchema)
		require.Equal(t, "https://json-schema.org/draft/2020-12/schema", input["$schema"], tool.Name)
		require.Equal(t, "object", input["type"], tool.Name)
		require.Equal(t, false, input["additionalProperties"], tool.Name)
		// The Anthropic Messages API rejects these at the top level of input_schema.
		for _, keyword := range []string{"oneOf", "allOf", "anyOf"} {
			require.NotContains(t, input, keyword, tool.Name)
		}
		properties, _ := input["properties"].(map[string]any)
		if tool.Name != "poomsae.audit_closes" {
			require.NotContains(t, properties, "actor", tool.Name)
		}
		require.Equal(t, projectSelectors[tool.Name], properties["project"] != nil, tool.Name)
		require.NotContains(t, properties, "project_id", tool.Name)
		require.NotContains(t, properties, "workspace", tool.Name)

		annotations := tool.Annotations
		require.NotNil(t, annotations, tool.Name)
		require.NotNil(t, annotations.OpenWorldHint, tool.Name)
		require.Equal(t, openWorld[tool.Name], *annotations.OpenWorldHint, tool.Name)
	}
	require.Equal(t, wantNames, gotNames)
	require.True(t, sort.StringsAreSorted(gotNames))
}

func TestServerToolAnnotationsMatchMutationRisk(t *testing.T) {
	session := connectTestServerWithOptions(t, Options{
		Client: inertTestClient(t), Scope: NewAllScope(), EnableTokenAdmin: true,
		Actor: "example-agent", Version: "test-version",
	})
	result, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	readOnly := map[string]bool{
		"poomsae.cron_workflows": true, "poomsae.cron_workflow": true, "poomsae.cron_runs": true, "poomsae.cron_run": true,
		"poomsae.cron_capabilities": true, "poomsae.cron_jobs": true, "poomsae.cron_job": true, "poomsae.cron_new_uid": true,
		"poomsae.skills":                   true,
		"poomsae.search_tools":             true,
		"poomsae.federation_bridge_status": true,
		"poomsae.teams":                    true, "poomsae.team_show": true, "poomsae.project_access_show": true,
		"poomsae.audit_closes":      true,
		"poomsae.bridge_show":       true,
		"poomsae.connector_fields":  true,
		"poomsae.connectors":        true,
		"poomsae.digest":            true,
		"poomsae.events":            true,
		"poomsae.federation_status": true,
		"poomsae.graph":             true,
		"poomsae.inbox":             true,
		"poomsae.labels":            true,
		"poomsae.lease_status":      true,
		"poomsae.list":              true,
		"poomsae.next":              true,
		"poomsae.project_show":      true,
		"poomsae.projects":          true,
		"poomsae.read_doc":          true,
		"poomsae.ready":             true,
		"poomsae.recurrences":       true,
		"poomsae.search":            true,
		"poomsae.search_docs":       true,
		"poomsae.show":              true,
		"poomsae.status":            true,
		"poomsae.sync_status":       true,
		"poomsae.system":            true,
		"poomsae.tokens":            true,
		"poomsae.wait":              true,
	}
	for _, name := range sectionLoaderNames {
		readOnly[name] = true
	}
	// Mixed-mode tools with any overwriting or removing input (forced
	// claims, lease release, recurrence patch) advertise destructive.
	additive := map[string]bool{
		"poomsae.cron_workflow_create": true, "poomsae.cron_workflow_restore": true,
		"poomsae.cron_job_create": true, "poomsae.cron_job_restore": true,
		"poomsae.team_create": true,
		"poomsae.comment":     true, "poomsae.create": true,
		"poomsae.project_create": true, "poomsae.project_restore": true,
		"poomsae.restore": true, "poomsae.token_create": true,
	}
	nonIdempotentTools := map[string]bool{
		"poomsae.cron_new_uid": true,
		"poomsae.team_create":  true,
		// Identical retries mint new records: no idempotency key or natural
		// unique key deduplicates these creations.
		"poomsae.token_create":      true,
		"poomsae.recurrence_update": true,
		// Identical retries produce additional effects: renew extends the
		// lease expiry again and a sync pass re-imports from the provider.
		"poomsae.lease":            true,
		"poomsae.bridge_reconcile": true,
		"poomsae.sync_once":        true,
	}
	for _, tool := range result.Tools {
		annotations := tool.Annotations
		require.Equal(t, readOnly[tool.Name], annotations.ReadOnlyHint, tool.Name)
		require.Equal(t, !nonIdempotentTools[tool.Name], annotations.IdempotentHint, tool.Name)
		require.NotNil(t, annotations.DestructiveHint, tool.Name)
		if readOnly[tool.Name] || additive[tool.Name] {
			require.False(t, *annotations.DestructiveHint, tool.Name)
		} else {
			require.True(t, *annotations.DestructiveHint, tool.Name)
		}
	}
}

func TestToolAdmissionMiddlewareBoundsCalls(t *testing.T) {
	t.Run("rate", func(t *testing.T) {
		toolError := errors.New("tool failed")
		var calls, observations atomic.Int64
		handler := toolAdmissionMiddleware(rate.NewLimiter(rate.Every(time.Hour), 1), make(chan struct{}, 1), func() { observations.Add(1) })(
			func(context.Context, string, sdkmcp.Request) (sdkmcp.Result, error) {
				calls.Add(1)
				return nil, toolError
			},
		)
		_, err := handler(t.Context(), "tools/call", nil)
		require.ErrorIs(t, err, toolError)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		_, err = handler(ctx, "tools/call", nil)
		require.ErrorContains(t, err, "deadline")
		_, err = handler(t.Context(), "tools/list", nil)
		require.ErrorIs(t, err, toolError)
		require.EqualValues(t, 2, calls.Load())
		require.EqualValues(t, 1, observations.Load())
	})

	t.Run("concurrency", func(t *testing.T) {
		var observations atomic.Int64
		started := make(chan struct{})
		release := make(chan struct{})
		handler := toolAdmissionMiddleware(rate.NewLimiter(rate.Inf, 1), make(chan struct{}, 1), func() { observations.Add(1) })(
			func(context.Context, string, sdkmcp.Request) (sdkmcp.Result, error) {
				close(started)
				<-release
				return nil, nil
			},
		)
		firstDone := make(chan error, 1)
		go func() {
			_, err := handler(t.Context(), "tools/call", nil)
			firstDone <- err
		}()
		<-started
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond) //nolint:kennlint // deadlinetest: the deadline is the expected result; release holds the only concurrency slot until it fires
		defer cancel()
		_, err := handler(ctx, "tools/call", nil)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		close(release)
		require.NoError(t, <-firstDone)
		require.EqualValues(t, 1, observations.Load())
	})

	t.Run("concurrency wait does not consume rate permit", func(t *testing.T) {
		concurrent := make(chan struct{}, 1)
		concurrent <- struct{}{}
		// An hour-long refill makes a spent permit fail the admitted call at once.
		handler := toolAdmissionMiddleware(rate.NewLimiter(rate.Every(time.Hour), 1), concurrent, nil)(
			func(context.Context, string, sdkmcp.Request) (sdkmcp.Result, error) {
				return nil, nil
			},
		)
		blockedContext, cancelBlocked := context.WithTimeout(t.Context(), 10*time.Millisecond) //nolint:kennlint // deadlinetest: the deadline is the expected result; the filled concurrency channel blocks admission until it fires
		defer cancelBlocked()
		_, err := handler(blockedContext, "tools/call", nil)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		<-concurrent

		admittedContext, cancelAdmitted := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancelAdmitted()
		_, err = handler(admittedContext, "tools/call", nil)
		require.NoError(t, err)
	})
}

func TestLinkSummaryPublishesOnlyDaemonFields(t *testing.T) {
	encoded, err := json.Marshal(LinkSummary{
		Type:         "blocks",
		QualifiedRef: "spoke-project#abc1",
		Status:       "open",
	})
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type":"blocks",
		"qualified_ref":"spoke-project#abc1",
		"status":"open"
	}`, string(encoded))
}

func TestCreateAndCommentRequireIdempotencyKeys(t *testing.T) {
	session := connectTestServer(t)
	result, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	byName := make(map[string]*sdkmcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		byName[tool.Name] = tool
	}
	for _, name := range []string{"poomsae.create", "poomsae.comment"} {
		input := schemaObject(t, byName[name].InputSchema)
		required := input["required"].([]any)
		require.Contains(t, required, "idempotency_key", name)
		properties := input["properties"].(map[string]any)
		keySchema := properties["idempotency_key"].(map[string]any)
		require.EqualValues(t, 1, keySchema["minLength"], name)
	}

	searchProperties := schemaObject(t, byName["poomsae.search"].InputSchema)["properties"].(map[string]any)
	require.ElementsMatch(t, []any{"auto", "lexical", "hybrid", "semantic"}, searchProperties["mode"].(map[string]any)["enum"])
	require.EqualValues(t, 100, searchProperties["limit"].(map[string]any)["maximum"])
	claimProperties := schemaObject(t, byName["poomsae.claim"].InputSchema)["properties"].(map[string]any)
	require.EqualValues(t, 60, claimProperties["ttl_seconds"].(map[string]any)["minimum"])
	require.EqualValues(t, 86400, claimProperties["ttl_seconds"].(map[string]any)["maximum"])

	closeProperties := schemaObject(t, byName["poomsae.close"].InputSchema)["properties"].(map[string]any)
	require.ElementsMatch(t, []any{"done", "wontfix", "duplicate", "superseded", "audit-no-change"}, closeProperties["reason"].(map[string]any)["enum"])
	evidenceItems := closeProperties["evidence"].(map[string]any)["items"].(map[string]any)
	require.Len(t, evidenceItems["oneOf"], 9)
	require.Contains(t, schemaObject(t, byName["poomsae.close"].InputSchema), "if")

	require.Contains(t, schemaObject(t, byName["poomsae.list"].InputSchema), "not")
	require.Contains(t, schemaObject(t, byName["poomsae.ready"].InputSchema), "not")
	require.Len(t, schemaObject(t, byName["poomsae.edit"].InputSchema)["not"].(map[string]any)["anyOf"], 5)

	labelProperties := schemaObject(t, byName["poomsae.set_label"].InputSchema)["properties"].(map[string]any)
	require.EqualValues(t, 64, labelProperties["label"].(map[string]any)["maxLength"])
	require.Equal(t, `^[a-z0-9._:-]+$`, labelProperties["label"].(map[string]any)["pattern"])

	reopenProperties := schemaObject(t, byName["poomsae.reopen"].InputSchema)["properties"].(map[string]any)
	require.NotContains(t, reopenProperties, "message")

	metadataProperties := schemaObject(t, byName["poomsae.set_metadata"].InputSchema)["properties"].(map[string]any)
	require.EqualValues(t, 0, metadataProperties["revision"].(map[string]any)["minimum"])
}

func TestCloseInputSchemaMirrorsDaemonEvidenceMatrix(t *testing.T) {
	schema, err := inputSchemaFor[CloseInput]("poomsae.close").Resolve(nil)
	require.NoError(t, err)
	message40 := strings.Repeat("a", 40)
	message60 := strings.Repeat("b", 60)
	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{name: "done with test", input: map[string]any{"ref": "abc1", "reason": "done", "message": message40, "evidence": []any{map[string]any{"type": "test", "command": "go test ./..."}}}},
		{name: "done with attachment", input: map[string]any{"ref": "abc1", "reason": "done", "message": message40, "evidence": []any{map[string]any{"type": "attachment", "attachment_id": "01K7E222222222222222222222"}}}},
		{name: "done with external account", input: map[string]any{"ref": "abc1", "reason": "done", "message": message40, "evidence": []any{map[string]any{"type": "external", "account": "email thread archived; calendar hold sent"}}}},
		{name: "done without evidence", input: map[string]any{"ref": "abc1", "reason": "done", "message": message40, "evidence": []any{}}, wantErr: true},
		{name: "wontfix", input: map[string]any{"ref": "abc1", "reason": "wontfix", "message": message60, "evidence": []any{}}},
		{name: "wontfix with evidence", input: map[string]any{"ref": "abc1", "reason": "wontfix", "message": message60, "evidence": []any{map[string]any{"type": "test", "command": "go test ./..."}}}, wantErr: true},
		{name: "duplicate", input: map[string]any{"ref": "abc1", "reason": "duplicate", "message": message40, "evidence": []any{map[string]any{"type": "duplicate-of", "issue_ref": "abc2"}}}},
		{name: "duplicate with test", input: map[string]any{"ref": "abc1", "reason": "duplicate", "message": message40, "evidence": []any{map[string]any{"type": "test", "command": "go test ./..."}}}, wantErr: true},
		{name: "superseded", input: map[string]any{"ref": "abc1", "reason": "superseded", "message": message40, "evidence": []any{map[string]any{"type": "superseded-by", "issue_ref": "abc2"}}}},
		{name: "audit", input: map[string]any{"ref": "abc1", "reason": "audit-no-change", "message": message40, "evidence": []any{map[string]any{"type": "no-change-audit", "rationale": "No change is required."}}}},
		{name: "audit plus reviewed paths", input: map[string]any{"ref": "abc1", "reason": "audit-no-change", "message": message40, "evidence": []any{map[string]any{"type": "no-change-audit", "rationale": "No change is required."}, map[string]any{"type": "reviewed-paths", "paths": []any{"internal/mcp"}}}}},
		{name: "audit without rationale", input: map[string]any{"ref": "abc1", "reason": "audit-no-change", "message": message40, "evidence": []any{map[string]any{"type": "reviewed-paths", "paths": []any{"internal/mcp"}}}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.Validate(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConvertEvidenceExternalPreservesAccount(t *testing.T) {
	var input Evidence
	require.NoError(t, json.Unmarshal([]byte(`{
		"type":"external",
		"account":"email thread archived; calendar hold sent"
	}`), &input))

	converted, err := (toolHandlers{}).convertEvidence(t.Context(), ProjectIdentity{}, input)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type":"external",
		"account":"email thread archived; calendar hold sent"
	}`, string(encoded))
}

func TestSetDeadlineInputSchemaRequiresOneMutation(t *testing.T) {
	schema, err := inputSchemaFor[SetDeadlineInput]("poomsae.set_deadline").Resolve(nil)
	require.NoError(t, err)

	require.NoError(t, schema.Validate(map[string]any{"ref": "abc1", "deadline": "2026-09-01T09:30"}))
	require.NoError(t, schema.Validate(map[string]any{"ref": "abc1", "clear_deadline": true}))
	require.Error(t, schema.Validate(map[string]any{"ref": "abc1"}))
	require.Error(t, schema.Validate(map[string]any{"ref": "abc1", "deadline": "2026-09-01", "clear_deadline": true}))
	require.Error(t, schema.Validate(map[string]any{"ref": "abc1", "clear_deadline": false}))
}

func TestSetScheduleInputSchemaRequiresOneMutation(t *testing.T) {
	schema, err := inputSchemaFor[SetScheduleInput]("poomsae.set_schedule").Resolve(nil)
	require.NoError(t, err)

	require.NoError(t, schema.Validate(map[string]any{"ref": "abc1", "schedule": "2026-09-01T09:30"}))
	require.NoError(t, schema.Validate(map[string]any{"ref": "abc1", "clear_schedule": true}))
	require.Error(t, schema.Validate(map[string]any{"ref": "abc1"}))
	require.Error(t, schema.Validate(map[string]any{"ref": "abc1", "schedule": "2026-09-01", "clear_schedule": true}))
	require.Error(t, schema.Validate(map[string]any{"ref": "abc1", "clear_schedule": false}))
}

func TestEditAndListInputSchemasForbidConflictingFields(t *testing.T) {
	edit, err := inputSchemaFor[EditInput]("poomsae.edit").Resolve(nil)
	require.NoError(t, err)
	require.NoError(t, edit.Validate(map[string]any{"ref": "abc1", "owner": "example-agent"}))
	require.NoError(t, edit.Validate(map[string]any{"ref": "abc1", "owner": "example-agent", "clear_owner": false}))
	require.Error(t, edit.Validate(map[string]any{"ref": "abc1", "owner": "example-agent", "clear_owner": true}))
	require.Error(t, edit.Validate(map[string]any{"ref": "abc1", "priority": 1, "clear_priority": true}))
	require.Error(t, edit.Validate(map[string]any{"ref": "abc1", "parent": "abc2", "remove_parent": "abc3"}))

	list, err := inputSchemaFor[ListInput]("poomsae.list").Resolve(nil)
	require.NoError(t, err)
	require.NoError(t, list.Validate(map[string]any{"owner": "example-agent"}))
	require.Error(t, list.Validate(map[string]any{"owner": "example-agent", "unowned": true}))
}

func TestRecurrencePatchSchemaRequiresRevision(t *testing.T) {
	schema, err := inputSchemaFor[RecurrenceUpdateInput]("poomsae.recurrence_update").Resolve(nil)
	require.NoError(t, err)
	require.Error(t, schema.Validate(map[string]any{"action": "patch", "uid": "recurrence-uid"}))
	require.NoError(t, schema.Validate(map[string]any{"action": "patch", "uid": "recurrence-uid", "revision": 1}))
	require.NoError(t, schema.Validate(map[string]any{"action": "create", "template": map[string]any{"title": "Review"}}))
}

func discoveryHealth(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/v1/health" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"api_schema_version":"0.27.0"}`))
	return true
}

func TestToolsUseBoundDaemonProjectAndActor(t *testing.T) {
	requests := make(chan capturedRequest, 20)
	daemon := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if discoveryHealth(writer, request) {
			return
		}
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		requests <- capturedRequest{
			Method: request.Method,
			Path:   request.URL.Path,
			Query:  request.URL.RawQuery,
			Header: request.Header.Clone(),
			Body:   body,
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(daemonResponse(request))
	}))
	t.Cleanup(daemon.Close)
	apiClient, err := poomsaeclient.NewWithHTTPClient(daemon.URL, daemon.Client())
	require.NoError(t, err)
	session := connectTestServerWithClient(t, apiClient)

	tests := []struct {
		name       string
		tool       string
		arguments  map[string]any
		wantMethod string
		wantPath   string
		wantQuery  []string
		wantActor  bool
		wantKey    string
		wantMatch  string
	}{
		{name: "search", tool: "poomsae.search", arguments: map[string]any{"query": "callback race", "limit": 3, "labels": []string{"bug"}}, wantMethod: http.MethodGet, wantPath: "/api/v1/projects/42/search", wantQuery: []string{"q=callback+race", "limit=3", "label=bug"}, wantMatch: "abc1"},
		{name: "list", tool: "poomsae.list", arguments: map[string]any{"status": "open", "limit": 3, "unowned": true}, wantMethod: http.MethodGet, wantPath: "/api/v1/projects/42/issues", wantQuery: []string{"status=open", "limit=3", "unowned=true", "sort=created"}, wantMatch: "abc1"},
		{name: "show", tool: "poomsae.show", arguments: map[string]any{"ref": "abc1", "comment_limit": 1}, wantMethod: http.MethodGet, wantPath: "/api/v1/projects/42/issues/abc1", wantMatch: "Issue title"},
		{name: "ready", tool: "poomsae.ready", arguments: map[string]any{"limit": 3, "unowned": true}, wantMethod: http.MethodGet, wantPath: "/api/v1/projects/42/ready", wantQuery: []string{"limit=4", "unowned=true"}, wantMatch: "abc1"},
		{name: "labels", tool: "poomsae.labels", arguments: map[string]any{}, wantMethod: http.MethodGet, wantPath: "/api/v1/projects/42/labels", wantMatch: "bug"},
		{name: "create", tool: "poomsae.create", arguments: map[string]any{"title": "New issue", "idempotency_key": "new-issue-1", "labels": []string{"bug"}}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues", wantActor: true, wantKey: "new-issue-1", wantMatch: "abc1"},
		{name: "edit", tool: "poomsae.edit", arguments: map[string]any{"ref": "abc1", "title": "Updated title", "add_related": []string{"spoke-project#def2"}}, wantMethod: http.MethodPatch, wantPath: "/api/v1/projects/42/issues/abc1", wantActor: true, wantMatch: "abc1"},
		{name: "comment", tool: "poomsae.comment", arguments: map[string]any{"ref": "abc1", "body": "Progress note", "idempotency_key": "comment-1"}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/comments", wantActor: true, wantKey: "comment-1", wantMatch: "Progress note"},
		{name: "claim", tool: "poomsae.claim", arguments: map[string]any{"ref": "abc1"}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/actions/claim", wantActor: true, wantMatch: "abc1"},
		{name: "set deadline", tool: "poomsae.set_deadline", arguments: map[string]any{"ref": "abc1", "deadline": "2026-09-01T09:30", "revision": 7}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/metadata", wantActor: true, wantMatch: "abc1"},
		{name: "set schedule", tool: "poomsae.set_schedule", arguments: map[string]any{"ref": "abc1", "schedule": "2026-09-01T09:30", "revision": 7}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/metadata", wantActor: true, wantMatch: "abc1"},
		{name: "add label", tool: "poomsae.set_label", arguments: map[string]any{"ref": "abc1", "label": "bug", "present": true}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/labels", wantActor: true, wantMatch: "abc1"},
		{name: "patch metadata", tool: "poomsae.set_metadata", arguments: map[string]any{"ref": "abc1", "patch": map[string]any{"work.attention": "ok"}, "revision": 7}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/metadata", wantActor: true, wantMatch: "abc1"},
		{name: "close", tool: "poomsae.close", arguments: map[string]any{"ref": "abc1", "reason": "done", "message": "Completed the requested work and verified its behavior.", "evidence": []map[string]any{{"type": "test", "command": "go test ./..."}}}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/actions/close", wantActor: true, wantMatch: "abc1"},
		{name: "reopen", tool: "poomsae.reopen", arguments: map[string]any{"ref": "abc1"}, wantMethod: http.MethodPost, wantPath: "/api/v1/projects/42/issues/abc1/actions/reopen", wantActor: true, wantMatch: "abc1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.arguments,
			})
			require.NoError(t, err)
			require.False(t, result.IsError, result.Content)
			require.Empty(t, result.Content, "latest clients use structured content only")
			structured, ok := result.StructuredContent.(map[string]any)
			require.True(t, ok, "%T", result.StructuredContent)
			project := structured["project"].(map[string]any)
			require.Equal(t, "spoke-project", project["name"])
			require.Contains(t, string(mustJSON(t, structured)), tt.wantMatch)
			assertSummaryHydration(t, tt.tool, structured)
			if tt.tool == "poomsae.edit" {
				require.Len(t, structured["events"], 2)
				changes := structured["changes"].(map[string]any)
				require.Len(t, changes["related_added"], 1)
			}

			if tt.tool == "poomsae.edit" {
				resolution := <-requests
				require.Equal(t, http.MethodGet, resolution.Method)
				require.Equal(t, "/api/v1/projects/42/issues/def2", resolution.Path)
			}
			request := <-requests
			require.Equal(t, tt.wantMethod, request.Method)
			require.Equal(t, tt.wantPath, request.Path)
			for _, query := range tt.wantQuery {
				require.Contains(t, request.Query, query)
			}
			if tt.wantKey != "" {
				require.Equal(t, tt.wantKey, request.Header.Get("Idempotency-Key"))
			}
			if tt.tool == "poomsae.set_metadata" || tt.tool == "poomsae.set_deadline" || tt.tool == "poomsae.set_schedule" {
				require.Equal(t, `"rev-7"`, request.Header.Get("If-Match"))
			}
			if tt.wantActor {
				var body map[string]any
				require.NoError(t, json.Unmarshal(request.Body, &body))
				require.Equal(t, "example-agent", body["actor"])
				require.NotContains(t, body, "project")
				if tt.tool == "poomsae.edit" {
					links := body["links_delta"].(map[string]any)
					require.Equal(t, []any{"01ARZ3NDEKTSV4RRFFQ69G5FAV"}, links["add_related"])
					require.Equal(t, map[string]any{
						"01ARZ3NDEKTSV4RRFFQ69G5FAV": "01ARZ3NDEKTSV4RRFFQ69G5FAA",
					}, links["expected_project_uids"])
				}
				if tt.tool == "poomsae.set_deadline" {
					require.Equal(t, map[string]any{"deadline_on": "2026-09-01T09:30"}, body["patch"])
				}
				if tt.tool == "poomsae.set_schedule" {
					require.Equal(t, map[string]any{"scheduled_on": "2026-09-01T09:30"}, body["patch"])
				}
			}
			if tt.tool == "poomsae.edit" {
				scopeRequest := <-requests
				require.Equal(t, http.MethodGet, scopeRequest.Method)
				require.Equal(t, "/api/v1/issues/01ARZ3NDEKTSV4RRFFQ69G5FAW", scopeRequest.Path)
			}
		})
	}
}

func TestToolsRoundTripAgainstRealDaemon(t *testing.T) {
	store, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "poomsae.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	project, err := store.CreateProject(t.Context(), "spoke-project")
	require.NoError(t, err)
	foreignProject, err := store.CreateProject(t.Context(), "other-project")
	require.NoError(t, err)
	foreignIssue, _, err := store.CreateIssue(t.Context(), db.CreateIssueParams{
		ProjectID: foreignProject.ID, Title: "Foreign issue", Author: "example-agent",
	})
	require.NoError(t, err)
	daemonServer := daemon.NewServer(daemon.ServerConfig{DB: store, StartedAt: time.Now().UTC()})
	t.Cleanup(func() { require.NoError(t, daemonServer.Close()) })
	httpServer := httptest.NewServer(daemonServer.Handler())
	t.Cleanup(httpServer.Close)
	apiClient, err := poomsaeclient.NewWithHTTPClient(httpServer.URL, httpServer.Client())
	require.NoError(t, err)
	session := connectTestServerWithOptions(t, Options{
		Client: apiClient, ProjectID: project.ID, ProjectName: project.Name,
		Actor: "example-agent", Version: "test-version",
	})

	created, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.create",
		Arguments: map[string]any{
			"title": "Round-trip issue", "labels": []string{"bug"},
			"idempotency_key": "round-trip-create-1",
		},
	})
	require.NoError(t, err)
	require.False(t, created.IsError)
	createdIssue := created.StructuredContent.(map[string]any)["issue"].(map[string]any)
	require.NotContains(t, createdIssue, "labels")
	ref := createdIssue["ref"].(string)
	for _, tc := range []struct {
		tool      string
		arguments map[string]any
	}{
		{"poomsae.set_label", map[string]any{"ref": ref, "label": "priority:high", "present": true}},
		{"poomsae.create", map[string]any{"title": "Priority label", "labels": []string{"p2"}, "idempotency_key": "priority-label"}},
	} {
		result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: tc.tool, Arguments: tc.arguments})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].(*sdkmcp.TextContent).Text, "priority")
	}
	escaped, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.edit", Arguments: map[string]any{
			"ref": ref, "add_related": []string{"spoke-project#" + strings.ToLower(foreignIssue.UID)},
		},
	})
	require.NoError(t, err)
	require.True(t, escaped.IsError, "qualified full-length short IDs must not resolve globally")
	reused, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.create",
		Arguments: map[string]any{
			"title": "Round-trip issue", "labels": []string{"bug"},
			"idempotency_key": "round-trip-create-1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, true, reused.StructuredContent.(map[string]any)["reused"])

	labeled, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_label", Arguments: map[string]any{"ref": ref, "label": "urgent", "present": true},
	})
	require.NoError(t, err)
	require.False(t, labeled.IsError)
	require.NotContains(t, labeled.StructuredContent.(map[string]any)["issue"], "labels")

	shown, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.show", Arguments: map[string]any{"ref": ref},
	})
	require.NoError(t, err)
	require.False(t, shown.IsError)
	shownIssue := shown.StructuredContent.(map[string]any)["issue"].(map[string]any)
	require.ElementsMatch(t, []any{"bug", "urgent"}, shownIssue["labels"])
	require.NotContains(t, shownIssue, "blocked")

	revision := shownIssue["revision"]
	invalid, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_metadata", Arguments: map[string]any{
			"ref": ref, "patch": map[string]any{"work.attention": "working"}, "revision": revision,
		},
	})
	require.NoError(t, err)
	require.True(t, invalid.IsError)
	patched, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_metadata", Arguments: map[string]any{
			"ref": ref, "patch": map[string]any{"work.attention": "ok"}, "revision": revision,
		},
	})
	require.NoError(t, err)
	require.False(t, patched.IsError, patched.Content)

	conflict, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_metadata", Arguments: map[string]any{
			"ref": ref, "patch": map[string]any{"work.attention": "stuck"}, "revision": revision,
		},
	})
	require.NoError(t, err)
	require.True(t, conflict.IsError)
	require.Contains(t, string(mustJSON(t, conflict.Content)), "revision")

	deadlineSet, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_deadline", Arguments: map[string]any{"ref": ref, "deadline": "2026-09-01T16:30:00Z"},
	})
	require.NoError(t, err)
	require.False(t, deadlineSet.IsError, deadlineSet.Content)

	shown, err = session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.show", Arguments: map[string]any{"ref": ref},
	})
	require.NoError(t, err)
	require.Equal(t, "2026-09-01T16:30:00Z",
		shown.StructuredContent.(map[string]any)["issue"].(map[string]any)["metadata"].(map[string]any)["deadline_on"])

	deadlineCleared, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_deadline", Arguments: map[string]any{"ref": ref, "clear_deadline": true},
	})
	require.NoError(t, err)
	require.False(t, deadlineCleared.IsError, deadlineCleared.Content)

	shown, err = session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.show", Arguments: map[string]any{"ref": ref},
	})
	require.NoError(t, err)
	require.NotContains(t,
		shown.StructuredContent.(map[string]any)["issue"].(map[string]any)["metadata"].(map[string]any), "deadline_on")

	scheduleSet, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_schedule", Arguments: map[string]any{"ref": ref, "schedule": "2026-09-02T09:30"},
	})
	require.NoError(t, err)
	require.False(t, scheduleSet.IsError, scheduleSet.Content)

	shown, err = session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.show", Arguments: map[string]any{"ref": ref},
	})
	require.NoError(t, err)
	require.Equal(t, "2026-09-02T09:30",
		shown.StructuredContent.(map[string]any)["issue"].(map[string]any)["metadata"].(map[string]any)["scheduled_on"])

	scheduleCleared, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_schedule", Arguments: map[string]any{"ref": ref, "clear_schedule": true},
	})
	require.NoError(t, err)
	require.False(t, scheduleCleared.IsError, scheduleCleared.Content)

	shown, err = session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.show", Arguments: map[string]any{"ref": ref},
	})
	require.NoError(t, err)
	require.NotContains(t,
		shown.StructuredContent.(map[string]any)["issue"].(map[string]any)["metadata"].(map[string]any), "scheduled_on")
}

func assertSummaryHydration(t *testing.T, tool string, structured map[string]any) {
	t.Helper()
	switch tool {
	case "poomsae.list", "poomsae.ready":
		issue := structured["issues"].([]any)[0].(map[string]any)
		require.Contains(t, issue, "labels")
		require.Contains(t, issue, "blocked")
	case "poomsae.search":
		issue := structured["results"].([]any)[0].(map[string]any)["issue"].(map[string]any)
		require.NotContains(t, issue, "labels")
		require.NotContains(t, issue, "blocked")
	case "poomsae.show":
		issue := structured["issue"].(map[string]any)
		require.Contains(t, issue, "labels")
		require.NotContains(t, issue, "blocked")
	case "poomsae.labels":
		return
	default:
		issue := structured["issue"].(map[string]any)
		require.NotContains(t, issue, "labels")
		require.NotContains(t, issue, "blocked")
		require.NotContains(t, structured, "reused")
	}
}

func TestDiscoveryToolsReportIncompletePages(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		arguments map[string]any
		path      string
		arrayKey  string
	}{
		{name: "search", tool: "poomsae.search", arguments: map[string]any{"query": "work", "limit": 2}, path: "/api/v1/projects/42/search", arrayKey: "results"},
		{name: "list", tool: "poomsae.list", arguments: map[string]any{"limit": 2}, path: "/api/v1/projects/42/issues", arrayKey: "issues"},
		{name: "ready", tool: "poomsae.ready", arguments: map[string]any{"limit": 2}, path: "/api/v1/projects/42/ready", arrayKey: "issues"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daemon := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if discoveryHealth(writer, request) {
					return
				}
				require.Equal(t, tt.path, request.URL.Path)
				count := 3
				if tt.tool != "poomsae.ready" {
					count = 2
				}
				require.Equal(t, fmt.Sprint(count), request.URL.Query().Get("limit"))
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write(listLikeResponse(tt.tool, count))
			}))
			t.Cleanup(daemon.Close)
			apiClient, err := poomsaeclient.NewWithHTTPClient(daemon.URL, daemon.Client())
			require.NoError(t, err)
			session := connectTestServerWithClient(t, apiClient)

			result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: tt.tool, Arguments: tt.arguments})
			require.NoError(t, err)
			require.False(t, result.IsError)
			structured := result.StructuredContent.(map[string]any)
			require.True(t, structured["truncated"].(bool))
			require.Len(t, structured[tt.arrayKey], 2)
		})
	}
}

func TestLinkSummaryReversesIncomingDirectionalLinks(t *testing.T) {
	issueUID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	peer := generated.LinkPeer{UID: "01ARZ3NDEKTSV4RRFFQ69G5FAW", QualifiedID: "spoke-project#def2", Status: "open"}

	blockedBy := linkSummary(issueUID, generated.LinkOut{
		Type: "blocks",
		From: peer,
		To:   generated.LinkPeer{UID: issueUID},
	})
	require.Equal(t, "blocked_by", blockedBy.Type)

	child := linkSummary(issueUID, generated.LinkOut{
		Type: "parent",
		From: peer,
		To:   generated.LinkPeer{UID: issueUID},
	})
	require.Equal(t, "child", child.Type)
}

func TestBoundRelationshipRefCanonicalizesQualifiedRef(t *testing.T) {
	handlers := toolHandlers{options: Options{ProjectName: "spoke-project"}}

	ref, err := handlers.boundRelationshipRef("spoke-project#abc1")
	require.NoError(t, err)
	require.Equal(t, "abc1", ref)

	ref, err = handlers.boundRelationshipRef("abc1")
	require.NoError(t, err)
	require.Equal(t, "abc1", ref)

	_, err = handlers.boundRelationshipRef("spoke-project#other-project#def2")
	require.ErrorContains(t, err, "outside bound project")

	_, err = handlers.boundRelationshipRef("spoke-project#01arz3ndektsv4rrffq69g5fav")
	require.ErrorContains(t, err, "full-length")
}

func TestToolValidationStopsBeforeDaemonRequest(t *testing.T) {
	var calls atomic.Int64
	daemon := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(writer, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(daemon.Close)
	apiClient, err := poomsaeclient.NewWithHTTPClient(daemon.URL, daemon.Client())
	require.NoError(t, err)
	session := connectTestServerWithClient(t, apiClient)

	tests := []struct {
		name      string
		tool      string
		arguments map[string]any
	}{
		{name: "unknown argument", tool: "poomsae.show", arguments: map[string]any{"ref": "abc1", "actor": "other"}},
		{name: "cross project subject", tool: "poomsae.show", arguments: map[string]any{"ref": "other-project#abc1"}},
		{name: "cross project relationship", tool: "poomsae.create", arguments: map[string]any{"title": "Work", "idempotency_key": "work-1", "blocked_by": []string{"other-project#abc1"}}},
		{name: "unscoped UID relationship", tool: "poomsae.create", arguments: map[string]any{"title": "Work", "idempotency_key": "work-2", "blocked_by": []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"}}},
		{name: "oversize limit", tool: "poomsae.list", arguments: map[string]any{"limit": 101}},
		{name: "empty idempotency key", tool: "poomsae.comment", arguments: map[string]any{"ref": "abc1", "body": "note", "idempotency_key": " "}},
		{name: "edit conflict", tool: "poomsae.edit", arguments: map[string]any{"ref": "abc1", "priority": 1, "clear_priority": true}},
		{name: "missing deadline mutation", tool: "poomsae.set_deadline", arguments: map[string]any{"ref": "abc1"}},
		{name: "conflicting deadline mutation", tool: "poomsae.set_deadline", arguments: map[string]any{"ref": "abc1", "deadline": "2026-09-01", "clear_deadline": true}},
		{name: "missing schedule mutation", tool: "poomsae.set_schedule", arguments: map[string]any{"ref": "abc1"}},
		{name: "conflicting schedule mutation", tool: "poomsae.set_schedule", arguments: map[string]any{"ref": "abc1", "schedule": "2026-09-01", "clear_schedule": true}},
		{name: "empty metadata patch", tool: "poomsae.set_metadata", arguments: map[string]any{"ref": "abc1", "patch": map[string]any{}}},
		{name: "invalid close reason", tool: "poomsae.close", arguments: map[string]any{"ref": "abc1", "reason": "finished", "message": "done", "evidence": []any{}}},
		{name: "incomplete close evidence", tool: "poomsae.close", arguments: map[string]any{"ref": "abc1", "reason": "done", "message": "done", "evidence": []any{map[string]any{"type": "commit"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: tt.tool, Arguments: tt.arguments})
			require.NoError(t, err)
			require.True(t, result.IsError)
		})
	}
	require.Zero(t, calls.Load())
}

func TestUnknownToolIsProtocolErrorAndBadArgumentsAreToolError(t *testing.T) {
	session := connectTestServer(t)

	result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "poomsae.missing", Arguments: map[string]any{}})
	require.Error(t, err)
	require.Nil(t, result)

	result, err = session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "poomsae.show", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, result.IsError)
}

func TestSetLabelAbsentUsesDeleteWithBoundActor(t *testing.T) {
	requestSeen := make(chan capturedRequest, 1)
	daemon := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requestSeen <- capturedRequest{Method: request.Method, Path: request.URL.Path, Query: request.URL.RawQuery, Body: body}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(daemonResponse(request))
	}))
	t.Cleanup(daemon.Close)
	apiClient, err := poomsaeclient.NewWithHTTPClient(daemon.URL, daemon.Client())
	require.NoError(t, err)
	session := connectTestServerWithClient(t, apiClient)

	result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.set_label",
		Arguments: map[string]any{
			"ref": "abc1", "label": "bug", "present": false,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	request := <-requestSeen
	require.Equal(t, http.MethodDelete, request.Method)
	require.Equal(t, "/api/v1/projects/42/issues/abc1/labels/bug", request.Path)
	require.Equal(t, "actor=example-agent", request.Query)
	require.Empty(t, request.Body)
}

func TestDaemonErrorsBecomeToolExecutionErrors(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"status":409,"error":{"code":"owner_conflict","message":"issue is owned by another actor"}}`))
	}))
	t.Cleanup(daemon.Close)
	apiClient, err := poomsaeclient.NewWithHTTPClient(daemon.URL, daemon.Client())
	require.NoError(t, err)
	session := connectTestServerWithClient(t, apiClient)

	result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name: "poomsae.claim", Arguments: map[string]any{"ref": "abc1"},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.NotEmpty(t, result.Content)
}

func TestToolCancellationStopsDaemonRequest(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	daemon := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(cancelled)
	}))
	t.Cleanup(daemon.Close)
	apiClient, err := poomsaeclient.NewWithHTTPClient(daemon.URL, daemon.Client())
	require.NoError(t, err)
	session := connectTestServerWithClient(t, apiClient)

	ctx, cancel := context.WithCancel(t.Context())
	callDone := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name: "poomsae.search", Arguments: map[string]any{"query": "slow request"},
		})
		callDone <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-callDone, context.Canceled)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("daemon request was not cancelled")
	}
}

func connectTestServer(t *testing.T) *sdkmcp.ClientSession {
	t.Helper()
	return connectTestServerWithClient(t, inertTestClient(t))
}

func connectTestServerWithClient(t *testing.T, apiClient *poomsaeclient.Client) *sdkmcp.ClientSession {
	t.Helper()
	return connectTestServerWithOptions(t, defaultCatalogOptions(apiClient))
}

func defaultCatalogOptions(apiClient *poomsaeclient.Client) Options {
	return Options{
		Client: apiClient, ProjectID: 42, ProjectName: "spoke-project",
		Actor: "example-agent", Version: "test-version",
	}
}

func connectTestServerWithOptions(t *testing.T, options Options) *sdkmcp.ClientSession {
	t.Helper()
	session := connectRawTestServerWithOptions(t, options)
	for _, name := range sectionLoaderNames {
		result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		require.NoError(t, err)
		require.False(t, result.IsError, "%s: %s", name, mustJSON(t, result))
	}
	return session
}

func connectRawTestServerWithOptions(t *testing.T, options Options) *sdkmcp.ClientSession {
	t.Helper()
	return connectRawTestServerWithClientOptions(t, options, nil)
}

func connectRawTestServerWithClientOptions(t *testing.T, options Options, clientOptions *sdkmcp.ClientOptions) *sdkmcp.ClientSession {
	t.Helper()
	server, err := New(options)
	require.NoError(t, err)
	return connectServerSession(t, server, clientOptions, nil)
}

func connectServerSession(t *testing.T, server *sdkmcp.Server, clientOptions *sdkmcp.ClientOptions, capture io.Writer) *sdkmcp.ClientSession {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	var writer io.Writer = serverConn
	if capture != nil {
		writer = io.MultiWriter(capture, serverConn)
	}
	ctx, cancel := context.WithCancel(t.Context())
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, NewStdioTransport(serverConn, writer))
	}()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "test-client",
		Version: "1",
	}, clientOptions)
	session, err := client.Connect(ctx, &sdkmcp.IOTransport{
		Reader: clientConn,
		Writer: clientConn,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		select {
		case <-serverDone:
		case <-time.After(time.Second):
			t.Error("MCP server did not stop")
		}
	})
	return session
}

type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

func daemonResponse(request *http.Request) []byte {
	issue := map[string]any{
		"id": 1, "uid": "01ARZ3NDEKTSV4RRFFQ69G5FAV", "project_id": 42,
		"project_uid": "01ARZ3NDEKTSV4RRFFQ69G5FAA", "short_id": "abc1",
		"qualified_id": "spoke-project#abc1", "title": "Issue title", "body": "Issue body",
		"status": "open", "author": "example-agent", "owner": "example-agent",
		"labels": []string{"bug"}, "metadata": map[string]any{"work.attention": "ok"},
		"revision": 7, "blocked": false,
		"created_at": "2026-08-08T00:00:00Z", "updated_at": "2026-08-08T00:00:00Z",
	}
	event := map[string]any{
		"id": 9, "uid": "01ARZ3NDEKTSV4RRFFQ69G5FAW", "type": "issue.updated",
		"actor": "example-agent", "content_hash": "hash", "created_at": "2026-08-08T00:00:00Z",
		"origin_instance_uid": "01ARZ3NDEKTSV4RRFFQ69G5FAX", "payload": "{}",
		"project_id": 42, "project_name": "spoke-project", "project_uid": "01ARZ3NDEKTSV4RRFFQ69G5FAA",
	}
	path := request.URL.Path
	switch {
	case strings.HasSuffix(path, "/search"):
		return mustJSONBytes(map[string]any{"query": "callback race", "mode": "lexical", "complete": true, "results": []any{map[string]any{"issue": issue, "score": 1.5, "matched_in": []string{"title"}}}})
	case strings.HasSuffix(path, "/ready"):
		return mustJSONBytes(map[string]any{"complete": true, "issues": []any{issue}})
	case strings.HasSuffix(path, "/labels") && request.Method == http.MethodGet:
		return mustJSONBytes(map[string]any{"labels": []any{map[string]any{"label": "bug", "count": 1}}})
	case strings.HasSuffix(path, "/comments"):
		return mustJSONBytes(map[string]any{"changed": true, "issue": issue, "event": event, "comment": map[string]any{"id": 2, "uid": "01ARZ3NDEKTSV4RRFFQ69G5FAY", "issue_id": 1, "author": "example-agent", "body": "Progress note", "created_at": "2026-08-08T00:00:00Z"}})
	case strings.HasSuffix(path, "/actions/claim"):
		return mustJSONBytes(map[string]any{"changed": true, "issue": issue, "event": event})
	case strings.HasSuffix(path, "/metadata"):
		return mustJSONBytes(map[string]any{"changed": true, "issue": issue, "event": event})
	case strings.HasSuffix(path, "/labels") && request.Method == http.MethodPost:
		return mustJSONBytes(map[string]any{"changed": true, "issue": issue, "event": event, "label": map[string]any{"issue_id": 1, "label": "bug", "author": "example-agent", "created_at": "2026-08-08T00:00:00Z"}})
	case strings.Contains(path, "/actions/"):
		return mustJSONBytes(map[string]any{"changed": true, "issue": issue, "event": event})
	case strings.HasSuffix(path, "/issues") && request.Method == http.MethodGet:
		return mustJSONBytes(map[string]any{"complete": true, "issues": []any{issue}})
	case strings.HasSuffix(path, "/issues") && request.Method == http.MethodPost:
		return mustJSONBytes(map[string]any{"changed": true, "issue": issue, "event": event})
	case request.Method == http.MethodPatch:
		peer := map[string]any{
			"project": "spoke-project", "qualified_id": "spoke-project#def2",
			"short_id": "def2", "status": "open", "uid": "01ARZ3NDEKTSV4RRFFQ69G5FAW",
		}
		return mustJSONBytes(map[string]any{
			"changed": true, "issue": issue, "event": event, "events": []any{event, event},
			"changes": map[string]any{"related_added": []any{peer}},
		})
	case request.Method == http.MethodGet:
		return mustJSONBytes(map[string]any{"issue": issue, "labels": []any{map[string]any{"issue_id": 1, "label": "bug", "author": "example-agent", "created_at": "2026-08-08T00:00:00Z"}}, "comments": []any{map[string]any{"id": 2, "uid": "01ARZ3NDEKTSV4RRFFQ69G5FAY", "issue_id": 1, "author": "example-agent", "body": "Progress note", "created_at": "2026-08-08T00:00:00Z"}}, "links": []any{}})
	default:
		return mustJSONBytes(map[string]any{"changed": true, "issue": issue, "event": event})
	}
}

func listLikeResponse(tool string, count int) []byte {
	issues := make([]any, 0, count)
	results := make([]any, 0, count)
	for index := range count {
		shortID := fmt.Sprintf("abc%d", index)
		issue := map[string]any{
			"id": index + 1, "uid": fmt.Sprintf("01ARZ3NDEKTSV4RRFFQ69G5FA%d", index),
			"short_id": shortID, "qualified_id": "spoke-project#" + shortID,
			"project_id": 42, "title": "Issue title", "body": "Issue body",
			"author": "example-agent", "status": "open", "revision": 7,
			"metadata": map[string]any{}, "labels": []string{"bug"}, "blocked": false,
			"created_at": "2026-08-08T00:00:00Z", "updated_at": "2026-08-08T00:00:00Z",
		}
		issues = append(issues, issue)
		results = append(results, map[string]any{"issue": issue, "score": 1.0, "matched_in": []string{"title"}})
	}
	if tool == "poomsae.search" {
		return mustJSONBytes(map[string]any{"query": "work", "mode": "lexical", "complete": false, "truncated": true, "results": results})
	}
	return mustJSONBytes(map[string]any{"complete": false, "truncated": true, "next_cursor": "page-two", "issues": issues})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func mustJSONBytes(value any) []byte {
	var buffer bytes.Buffer
	_ = json.NewEncoder(&buffer).Encode(value)
	return buffer.Bytes()
}

func schemaObject(t *testing.T, schema any) map[string]any {
	t.Helper()
	object, ok := schema.(map[string]any)
	require.True(t, ok, "schema is %T", schema)
	return object
}

func TestListPreservesIssueBrowserURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if discoveryHealth(w, r) {
			return
		}
		require.Equal(t, "/api/v1/projects/42/issues", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"complete":true,"issues":[{"uid":"01ARZ3NDEKTSV4RRFFQ69G5FAV","short_id":"5fav","web_url":"https://tasks.example/poomsae?issue=01ARZ3NDEKTSV4RRFFQ69G5FAV"}]}`))
	}))
	t.Cleanup(server.Close)
	client, err := poomsaeclient.NewWithHTTPClient(server.URL, server.Client())
	require.NoError(t, err)
	session := connectTestServerWithClient(t, client)
	result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "poomsae.list", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	structured := result.StructuredContent.(map[string]any)
	rows := structured["issues"].([]any)
	require.Len(t, rows, 1)
	require.Equal(t, "https://tasks.example/poomsae?issue=01ARZ3NDEKTSV4RRFFQ69G5FAV", rows[0].(map[string]any)["web_url"])
}

func TestConvertEvidenceAttachmentPreservesID(t *testing.T) {
	const id = " report "
	converted, err := (toolHandlers{}).convertEvidence(t.Context(), ProjectIdentity{}, Evidence{Type: "attachment", AttachmentID: id})
	require.NoError(t, err)
	require.NotNil(t, converted.AttachmentID)
	require.Equal(t, id, *converted.AttachmentID)
	_, err = (toolHandlers{}).convertEvidence(t.Context(), ProjectIdentity{}, Evidence{Type: "attachment"})
	require.Error(t, err)
}
