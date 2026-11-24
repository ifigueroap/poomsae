package main

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// applyRootHelp keeps the discoverable CLI guide and grouping in one place.
func applyRootHelp(root *cobra.Command) {
	cobra.AddTemplateFunc("poomsaeGlobalFlags", poomsaeGlobalFlags)
	root.Short = "issue tracker for coding agents"
	root.Long = poomsaeRootHelp
	for _, group := range []cobra.Group{
		{ID: "find", Title: "Find and read work:"},
		{ID: "change", Title: "Change issues:"},
		{ID: "coord", Title: "Coordinate agents:"},
		{ID: "setup", Title: "Set up and diagnose:"},
		{ID: "admin", Title: "Administer (rarely needed by agents):"},
	} {
		root.AddGroup(&group)
	}
	for _, child := range root.Commands() {
		child.GroupID = poomsaeCommandGroups[child.Name()]
	}
	root.SetHelpCommandGroupID("setup")
	root.SetCompletionCommandGroupID("setup")
	root.SetUsageTemplate(poomsaeUsageTemplate)
}

var poomsaeCommandGroups = map[string]string{
	"cli":                 "find",
	"search":              "find",
	"list":                "find",
	"ready":               "find",
	"next":                "find",
	"show":                "find",
	"status":              "find",
	"inbox":               "find",
	"labels":              "find",
	"whoami":              "find",
	"create":              "change",
	"edit":                "change",
	"comment":             "change",
	"claim":               "change",
	"assign":              "change",
	"unassign":            "change",
	"label":               "change",
	"meta":                "change",
	"docbank":             "change",
	"schedule":            "change",
	"deadline":            "change",
	"notify":              "change",
	"move":                "change",
	"close":               "change",
	"reopen":              "change",
	"wait":                "coord",
	"events":              "coord",
	"digest":              "coord",
	"audit":               "coord",
	"quickstart":          "coord",
	"agent-hook":          "coord",
	"agent-contract-hook": "coord",
	"cron":                "admin",
	"agent":               "setup",
	"init":                "setup",
	"doctor":              "setup",
	"health":              "setup",
	"daemon":              "setup",
	"mcp":                 "setup",
	"ui":                  "setup",
	"tui":                 "setup",
	"migrate-from-kata":   "setup",
	"update":              "setup",
	"version":             "setup",
	"projects":            "admin",
	"export":              "admin",
	"import":              "admin",
	"openapi":             "admin",
	"delete":              "admin",
	"restore":             "admin",
	"purge":               "admin",
}

func poomsaeGlobalFlags(cmd *cobra.Command) string {
	var names []string
	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		name := "--" + flag.Name
		if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
			name = "-" + flag.Shorthand + "/" + name
		}
		if flag.Value.Type() != "bool" {
			name += " <" + flag.Value.Type() + ">"
		}
		names = append(names, name)
	})
	return strings.Join(names, ", ")
}

const poomsaeRootHelp = `Agents: find the right command with: poomsae cli search "<what you want to do>"

Poomsae is an issue tracker for coding agents and the people who direct them.
Commands talk to a poomsae daemon (local and auto-started by default;
POOMSAE_SERVER selects a remote one) and act on the project bound to the
current directory.

Agent workflow (add --agent to each command):
  poomsae agent setup                                      # connect Claude Code or Codex
  poomsae search "<one or two distinctive words>" --agent  # 1. reuse an existing issue first
  poomsae create "<title>" --body "<context>" --idempotency-key <stable-key> --agent
  poomsae claim <ref> --ttl 2h --agent                     # 2. take ownership; renew the same way, without --if-unowned
  poomsae close <ref> --done -m "<what changed and how it was verified>" --test "<command>" --agent   # or --commit <sha>
  poomsae label add <ref> needs-review --agent   # not done; leave it open
Full agent contract: poomsae quickstart --format contract

Refs: abc4 (short id in the bound project), project#abc4 (any project), or the
26-character ULID. Legacy issue numbers such as 12 are rejected.
Project: found from .poomsae.toml at or above --workspace (default: cwd). Create the
binding with poomsae init, or pass --project <name>.
Output: human text by default. --agent prints an OK or ERR status line, then
command-specific details; --json prints the full JSON envelope for scripts.
Errors go to stderr.
Exit codes: 0 ok, 1 internal, 2 usage, 3 validation, 4 not found, 5 conflict,
6 confirmation required, 7 daemon unavailable, 8 wait timeout.
CLI panics exit 1 as internal errors; daemon and background task panics keep diagnostic traces.
Environment: POOMSAE_AUTHOR actor; POOMSAE_TEAMMATE teammate attribution;
POOMSAE_INBOX_USER inbox address (actor or actor/teammate); POOMSAE_SERVER remote
daemon URL; POOMSAE_HOME local daemon state; POOMSAE_AUTH_TOKEN daemon bearer token.`

// Cobra v1.10.2 default usage template, with compact inherited flags.
const poomsaeUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global: {{poomsaeGlobalFlags .}}. Details: poomsae --help{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
