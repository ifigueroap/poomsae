package mcpserver

import (
	"context"
	"fmt"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// LoadedSections retains section selections across servers with the same
// principal and options. Its zero value is ready to use; it holds no servers.
type LoadedSections struct {
	mu  sync.Mutex
	ids map[string]bool
}

func (s *LoadedSections) selectSection(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids == nil {
		s.ids = make(map[string]bool)
	}
	s.ids[id] = true
}

func (s *LoadedSections) snapshot() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.ids))
	for id := range s.ids {
		ids = append(ids, id)
	}
	return ids
}

// ToolSectionInput loads one section's typed tools into the MCP catalog.
type ToolSectionInput struct{}

// ToolSectionOutput reports the tools available through one section.
type ToolSectionOutput struct {
	Section   string   `json:"section"`
	Available bool     `json:"available"`
	Loaded    bool     `json:"loaded"`
	Tools     []string `json:"tools"`
}

type toolSection struct {
	loader      string
	section     string
	title       string
	description string
	tools       []string
	available   bool
	register    func(*sdkmcp.Server, toolHandlers)
}

func registerSectionLoaders(server *sdkmcp.Server, options Options) error {
	handlers := toolHandlers{options: options}
	sections := toolSections(options.StorageAdmin != nil)
	if options.Events {
		for i := range sections {
			if sections[i].section == "coordination" {
				sections[i].tools = []string{"poomsae.assign", "poomsae.event_subscriptions", "poomsae.inbox", "poomsae.status", "poomsae.unassign"}
				sections[i].description += " Native Events can signal exact-recipient inbox changes. Includes the principal's webhook delivery health from the daemon."
				sections[i].register = func(server *sdkmcp.Server, h toolHandlers) {
					registerCoordinationTools(server, h)
					registerEventDeliveryTools(server, h)
				}
			}
		}
	}
	byID := make(map[string]toolSection, len(sections))
	for _, definition := range sections {
		byID[definition.section] = definition
	}
	loaded := make(map[string]bool, len(sections))
	load := func(definition toolSection) {
		if definition.available && !loaded[definition.section] {
			definition.register(server, handlers)
			loaded[definition.section] = true
		}
	}
	var mutex sync.Mutex
	for _, definition := range sections {
		addTool(server, definition.loader, definition.title, definition.description,
			toolHints(true, false, false),
			func(_ context.Context, _ *sdkmcp.CallToolRequest, _ ToolSectionInput) (*sdkmcp.CallToolResult, ToolSectionOutput, error) {
				mutex.Lock()
				defer mutex.Unlock()
				load(definition)
				if loaded[definition.section] {
					options.LoadedSections.selectSection(definition.section)
				}
				return successResult(), ToolSectionOutput{
					Section: definition.section, Available: definition.available,
					Loaded: loaded[definition.section], Tools: append([]string(nil), definition.tools...),
				}, nil
			})
	}
	for _, id := range options.StartupSections {
		definition, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown MCP startup section %q", id)
		}
		if loaded[id] {
			return fmt.Errorf("duplicate MCP startup section %q", id)
		}
		if !definition.available {
			return fmt.Errorf("MCP startup section %q is unavailable", id)
		}
		load(definition)
	}
	for _, id := range options.LoadedSections.snapshot() {
		if definition, ok := byID[id]; ok {
			load(definition)
		}
	}
	return nil
}

func toolSections(storageAvailable bool) []toolSection {
	return []toolSection{
		{
			loader: "poomsae.load_activity", section: "activity", title: "Load activity tools",
			description: "Load typed digest and event-stream tools, then refresh the MCP tool list.",
			tools:       []string{"poomsae.digest", "poomsae.events"}, available: true, register: registerActivityTools,
		},
		{
			loader: "poomsae.load_coordination", section: "coordination", title: "Load coordination tools",
			description: "Load inbox, assign, unassign, and issue-status tools. Inbox reads are paginated. Refresh the MCP tool list.",
			tools:       []string{"poomsae.assign", "poomsae.inbox", "poomsae.status", "poomsae.unassign"}, available: true, register: registerCoordinationTools,
		},
		{
			loader: "poomsae.load_cron", section: "cron", title: "Load cron tools",
			description: "Load native recurring notification jobs and external-adapter execution definitions. Run evidence does not launch work. Distinct from RRULE recurrence tools; refresh the MCP tool list.",
			tools:       []string{"poomsae.cron_capabilities", "poomsae.cron_new_uid", "poomsae.cron_jobs", "poomsae.cron_job", "poomsae.cron_job_create", "poomsae.cron_job_update", "poomsae.cron_job_delete", "poomsae.cron_job_restore", "poomsae.cron_workflows", "poomsae.cron_workflow", "poomsae.cron_workflow_create", "poomsae.cron_workflow_update", "poomsae.cron_workflow_delete", "poomsae.cron_workflow_restore", "poomsae.cron_runs", "poomsae.cron_run", "poomsae.cron_run_observe"},
			available:   true, register: registerCronTools,
		},
		{
			loader: "poomsae.load_docs", section: "docs", title: "Load documentation tools",
			description: "Load typed tools that search and read Poomsae's bundled user documentation, then refresh the MCP tool list.",
			tools:       []string{"poomsae.read_doc", "poomsae.search_docs"}, available: true, register: registerDocsTools,
		},
		{
			loader: "poomsae.load_import", section: "import", title: "Load import tools",
			description: "Load the typed structured issue-import tool, then refresh the MCP tool list.",
			tools:       []string{"poomsae.import_issues"}, available: true, register: registerImportTools,
		},
		{
			loader: "poomsae.load_issue_discovery", section: "issue_discovery", title: "Load issue discovery tools",
			description: "Load typed issue search, list, show, ready, next, label, and graph tools, then refresh the MCP tool list.",
			tools:       []string{"poomsae.graph", "poomsae.labels", "poomsae.list", "poomsae.next", "poomsae.ready", "poomsae.search", "poomsae.show"},
			available:   true, register: registerIssueDiscoveryTools,
		},
		{
			loader: "poomsae.load_issue_lifecycle", section: "issue_lifecycle", title: "Load issue lifecycle tools",
			description: "Load typed close, reopen, delete, restore, purge, wait, and close-audit tools, then refresh the MCP tool list.",
			tools:       []string{"poomsae.audit_closes", "poomsae.close", "poomsae.delete", "poomsae.purge", "poomsae.reopen", "poomsae.restore", "poomsae.wait"},
			available:   true, register: registerIssueLifecycleTools,
		},
		{
			loader: "poomsae.load_issue_mutation", section: "issue_mutation", title: "Load issue mutation tools",
			description: "Load typed issue creation, edit, comment, claim, label, metadata, planning-date, and move tools, then refresh the MCP tool list.",
			tools:       []string{"poomsae.claim", "poomsae.comment", "poomsae.create", "poomsae.edit", "poomsae.edit_comment", "poomsae.move", "poomsae.set_deadline", "poomsae.set_label", "poomsae.set_metadata", "poomsae.set_schedule"},
			available:   true, register: registerIssueMutationTools,
		},
		{
			loader: "poomsae.load_projects", section: "projects", title: "Load project tools",
			description: "Load typed project discovery, detail, and administration tools, then refresh the MCP tool list.",
			tools:       []string{"poomsae.project_create", "poomsae.project_merge", "poomsae.project_purge", "poomsae.project_remove", "poomsae.project_restore", "poomsae.project_show", "poomsae.project_update", "poomsae.projects"},
			available:   true, register: registerProjectTools,
		},
		{
			loader: "poomsae.load_recurrence", section: "recurrence", title: "Load recurrence tools",
			description: "Load typed recurrence discovery and administration tools, then refresh the MCP tool list.",
			tools:       []string{"poomsae.recurrence_delete", "poomsae.recurrence_update", "poomsae.recurrences"},
			available:   true, register: registerRecurrenceTools,
		},
		{
			loader: "poomsae.load_storage", section: "storage", title: "Load storage tools",
			description: "Load typed JSONL storage tools when host storage was enabled at startup, then refresh the MCP tool list.",
			tools:       []string{"poomsae.storage_export", "poomsae.storage_import"}, available: storageAvailable, register: registerStorageTools,
		},
		{
			loader: "poomsae.load_system", section: "system", title: "Load system tools",
			description: "Load the typed redacted daemon system-information tool, then refresh the MCP tool list.",
			tools:       []string{"poomsae.system"}, available: true, register: registerSystemTools,
		},
	}
}
