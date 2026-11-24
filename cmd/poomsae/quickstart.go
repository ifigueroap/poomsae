package main

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ifigueroap/poomsae/internal/agentguide"
)

const hooklessQuickstartText = `
# Hookless consumer Muse

Preview setup: poomsae agent-hook instructions install muse --home <path> --actor <actor> --dry.
This generates standing instructions, a skill and a scheduled poll spec. It relies on
instruction-following, not enforcement, and provides no native attention support.
Choose authentication explicitly; create the scheduled task through Muse itself.
`

var agentQuickstartText = agentguide.Quickstart

const agentQuickstartCompactText = `Use poomsae as the shared issue ledger for this workspace.
Find commands with poomsae cli search "<task>" --agent; poomsae search finds issues.
Do not create practice, tutorial, example, or scratchpad issues.
Search before creating or updating work.
If create refuses a look-alike, reuse it; --force-new only if the work truly differs.
Related but different work: create it and link each hit with --parent <ref> or --related <ref>.
No bound workspace: use <project>#<id> refs or --project <project>.
Choose one unclaimed issue with poomsae next --unowned --agent.
Inspect a filtered queue with poomsae ready --unowned --label bug --no-label blocked --agent.
` + agentguide.ClaimRules + `Default to --agent for ordinary poomsae reads and mutations in agent logs.
Use --json only when your script needs complete structured data.
Launch each child with POOMSAE_TEAMMATE=teammate-1 and POOMSAE_INBOX_USER=coordinator/teammate-1.
Comments store teammate; new issues store metadata.teammate while author remains accountable.

` + agentguide.CommentRules + `
When a comment-sized task needs ownership or tracked completion, make it a child issue:

    poomsae create "fix retry handling" --parent <ref>
    poomsae claim <child> --ttl 2h

POOMSAE_INBOX_USER selects an inbox and does not set attribution; --teammate overrides the attribution default.
Request actor or teammate attention: poomsae notify <ref> --to <actor>[/<teammate>] --message "<reason>".
Read exact requests with poomsae inbox --for <actor>[/<teammate>]; clear after handling with poomsae notify <ref> --to <actor>[/<teammate>] --clear.
Use poomsae inbox --for <actor>[/<teammate>] --all for the selected daemon's active projects; refs are qualified.
An external harness polls idle inboxes and wakes the exact mapped runtime; quickstart does not install that integration.
Close verified work: poomsae close <ref> --done --message "<scope and verification, 40+ characters>" --evidence commit:<sha> | pr:<url> | test:<cmd> | reviewed-paths:<path>.
Evidence shortcuts: --commit, --pr, --test, --reviewed.
Retire duplicate or superseded work with --duplicate-of <ref> or --superseded-by <ref> (20+ characters); dropped work with --wontfix (60+ characters).
Close each verified issue promptly; valid evidence keeps sibling close bursts admissible by default.
Do not run delete or purge unless explicitly asked for that exact action and issue ref.
Poll poomsae events with a saved cursor; reset cached state on reset_required.
Never remove a workspace override to repair a stopped daemon.
Run poomsae daemon diagnose first; poomsae daemon recover starts only a registered existing local profile.
Use --expect-project-uid <uid> when project identity is known; never initialize a replacement project.
Explicit server URLs stay pinned; restore their server or tunnel.
`

const optionalSetupText = `
Optional setup:
For federation, run poomsae federation identity on the spoke and give its instance UID
and intended actor/project to the hub administrator. Keep the selected daemon pointed at the spoke.
Enrollment, joining, and polling: https://github.com/ifigueroap/poomsae/docs/operations/federation/
Embeddings are optional; lexical search works without them. Setup and credentials:
https://github.com/ifigueroap/poomsae/docs/guide/semantic-search/
`

func newQuickstartCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "quickstart",
		Aliases: []string{"agent-instructions"},
		Short:   "print instructions for agents using poomsae",
		Long: `Print instructions for agents using poomsae.

Default: full instructions for agents using poomsae.
--agent: concise instructions for agent logs.
--json: instructions in a JSON response.
--format contract: the canonical managed contract, exactly what poomsae agent-hook contract injects.

Run poomsae agent-hook install --all to load the contract in every session that starts in a poomsae workspace.
Without hooks, run poomsae quickstart at session start.
Setup guides cover federation enrollment and optional embeddings.
The managed contract format stays static.
` + hooklessQuickstartText,
		Example: `  poomsae quickstart --format contract
  poomsae quickstart --agent`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if currentOutputMode() == outputContract {
				_, err := fmt.Fprint(cmd.OutOrStdout(), agentContractText)
				return err
			}
			full := agentQuickstartText + optionalSetupText
			switch currentOutputMode() {
			case outputJSON:
				var buf bytes.Buffer
				if err := emitJSON(&buf, map[string]string{
					"quickstart": full,
				}); err != nil {
					return err
				}
				_, err := fmt.Fprint(cmd.OutOrStdout(), buf.String())
				return err
			case outputAgent:
				_, err := fmt.Fprint(cmd.OutOrStdout(), "OK quickstart\n"+agentQuickstartCompactText+optionalSetupText)
				return err
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), full)
			return err
		},
	}
}
