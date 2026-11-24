<h1 align="center">poomsae</h1>

<p align="center">
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/ifigueroap/poomsae" alt="Go version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
  <a href="https://github.com/ifigueroap/poomsae/tree/main/docs"><img src="https://img.shields.io/badge/Docs-github.com/ifigueroap/poomsae-blue" alt="Documentation"></a>
</p>

<p align="center">
  <a href="https://github.com/ifigueroap/poomsae/docs/">Documentation</a> ·
  <a href="https://github.com/ifigueroap/poomsae/docs/get-started/quickstart/">Setup Guide</a> ·
  <a href="https://github.com/ifigueroap/poomsae/docs/#two-ways-to-supervise">Interactive TUI</a>
</p>

**The issue tracker built for coding agents and the humans steering them.**

Coding agents need somewhere durable to track work: not a chat thread, not a
markdown to-do list. poomsae gives them a local task ledger they can drive from the
CLI: create, claim, relate, and close issues with evidence. Humans supervise the
same work in a terminal UI. By default, issue state lives in a local SQLite
database, so your repo stays clean and no hosted tracker is required. When a team
of humans and agents needs to share, you can opt into a remote daemon or
federation; a shared daemon can use Postgres by setting `POOMSAE_DSN`.

## What you can do

- **Track work from the CLI.** Create, claim, relate, and close issues with
  evidence that records what was done and who did it.
- **Supervise in the terminal or browser.** Browse and triage the same work with
  `poomsae tui` or `poomsae ui`. See the [Web UI guide](docs/guide/web-ui.md).
- **Automate recurring work.** Schedule `poomsae cron` jobs and workflows that run
  on triggers, keep per-run evidence, and let you preview and prune old terminal
  runs in batches. See the [cron reference](docs/reference/cron.md).
- **Request attention where the work is.** `poomsae notify <ref> --to
  <actor>/<teammate>` flags an issue for a person or agent with a reason and
  resolves it with `--clear` once handled. See the
  [CLI reference](docs/reference/cli.md).
- **Give subagents their own identity.** `POOMSAE_TEAMMATE` attributes a helper's
  comments and new issues, while `POOMSAE_INBOX_USER` points `poomsae inbox` and the
  TUI at that helper's own inbox, so delegated work stays separable. See
  [Configuration](docs/reference/configuration.md).
- **Answer evidence with evidence.** Reply to any comment, or record a verdict
  with `poomsae comment --confirm`, `--refute`, or `--supersede`, each backed by
  its own evidence note. See
  [Teammate requests](docs/reference/cli.md#teammate-requests).
- **Share when you need to.** Opt into a remote daemon or
  [federation](docs/operations/federation.md). Shared daemons can use Postgres
  with `POOMSAE_DSN`, including separate schema-owner and runtime roles; see the
  [Postgres operator guide](docs/operations/postgres.md).
- **Connect MCP clients.** Run `poomsae mcp serve` over stdio or Streamable HTTP.
  It defaults to the workspace's project, with optional project allowlists or
  the full daemon catalog and section loaders for typed workflows. See the
  [MCP reference](docs/reference/mcp.md).
- **Embed poomsae in Go.** Mount its listener-free HTTP service in your own
  application. See [Embedding poomsae in Go](docs/development/embedding.md).
- **Keep installations current.** Follow [Updating Poomsae](docs/get-started/install.md#updating-poomsae)
  for installation and federation upgrade guidance, and the
  [changelog](docs/changelog.md) for release notes.

## Installation

Linux and macOS:

```sh
curl -fsSL https://github.com/ifigueroap/poomsae/install.sh | bash
```

Windows PowerShell:

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/ifigueroap/poomsae/install.ps1 | iex"
```

The installer detects your OS and CPU architecture, downloads the latest GitHub
release archive, and verifies it against `SHA256SUMS` before installing. Confirm
the install with:

```sh
poomsae version
```

Release-archive builds update themselves with `poomsae update`. Archives are
published for Linux, macOS, and Windows on `amd64` and `arm64`, with
`SHA256SUMS` beside them. Prefer to build from source? poomsae needs **Go
1.27.4 or later**:

```sh
go install github.com/ifigueroap/poomsae/cmd/poomsae@latest
```

Module-source installs include the CLI, daemon, and TUI but not the compiled
browser bundle. Install a release binary, or build from a clone with
`make install`, to use `poomsae ui`.

Go installs to `$(go env GOBIN)`, falling back to `$(go env GOPATH)/bin` (often
`~/go/bin`); put that directory on your `PATH`. See
[Install](docs/get-started/install.md) for package downloads, manual release
downloads, and build-from-source steps.

## Migrating from kata

The first `poomsae` command copies an existing kata data directory when the
poomsae directory is not there yet. You can also run it yourself:

```sh
poomsae migrate-from-kata
```

The command looks for kata data in `$KATA_HOME`, `$HOME/.kata`, the XDG data
and config locations (`$XDG_DATA_HOME/kata`, `$XDG_CONFIG_HOME/kata`, or
`~/.local/share/kata` and `~/.config/kata`), and on Windows `%APPDATA%\kata`
and `%LOCALAPPDATA%\kata`. It stops a running kata daemon when it can, then
copies the directory. The copy includes the SQLite database, `kata.db-wal`,
`kata.db-shm`, attachments, and backups. The database is also available as
`poomsae.db`, and opening it runs the current schema migrations. The original
kata directory is not moved or deleted.

In a workspace, `.kata.toml` and `.kata.local.toml` are copied to
`.poomsae.toml` and `.poomsae.local.toml` the first time poomsae reads them, and
again when you pass that workspace to `migrate-from-kata`. Existing poomsae
files are left as they are, so the command is safe to repeat.

`KATA_*` environment variables are copied to the matching `POOMSAE_*` names
when the poomsae variable is unset, and poomsae prints a deprecation warning.
`KATA_HOME`, `KATA_DB`, and `KATA_DSN` are not pointed back at the original
files. A Postgres `KATA_DSN` is mapped to `POOMSAE_DSN`. Set `POOMSAE_HOME` when
the data should live somewhere other than `~/.poomsae`.

Coding agents follow this loop. `poomsae quickstart` covers subagents, inboxes,
attention signals, and scheduling.

1. For a question or review that doesn't change the workspace, answer without poomsae.
2. For a change, search with `poomsae search "<one or two words>" --agent`.
3. If an open issue covers the work, claim it with a time limit: `poomsae claim <ref> --ttl 2h --agent`. Otherwise, create one with `poomsae create "<title>" --idempotency-key <key> --agent`. Claim it if you'll work on it; leave it open if you're only recording it. Renew the claim before it expires by claiming again with `--ttl`, without `--if-unowned`.
4. Do the work and comment on findings a later reader needs.
5. If the work is verified complete, close it with `poomsae close <ref> --done --message "<what and how verified>" --test <cmd>`. Otherwise, leave it open, update `work.attention` and `work.attention_msg` with what remains, and add the `needs-review` label.

## Quickstart

```sh
cd your-repo
poomsae init                              # bind this workspace to a poomsae project
poomsae create "fix login race"           # prints a short id, e.g. abc4
poomsae list                              # see open work
poomsae show abc4                         # inspect by short id
poomsae tui                               # browse and triage interactively
poomsae tui abc4                          # open an issue directly in the TUI
poomsae ui                                # open the same ledger in a browser
```

`poomsae create` prints each issue's short id; use it in later commands. Close only
when the work is complete and verified:

```sh
poomsae close abc4 --done \
  --message "Fixed the login race and verified the relevant tests pass." \
  --commit <sha>
```

Reply to a specific comment, then read the thread it belongs to:

```sh
poomsae comment abc4 --reply c:xbzxrq --body-stdin <<'EOF'
This builds on the finding.
EOF
poomsae show abc4 --thread c:xbzxrq
```

The TUI and Web UI show nonzero counts for authorized direct relations on
original comments. Open a count to inspect chronological evidence with its
author, time, handle, and body, then jump to it and return focus. The TUI uses a
keyboard picker; Web controls are accessible. Replies show their outgoing
relation. Partial counts stay explicit. Conflicting assertions stay visible
together without scores, automatic resolution, or hiding. Pending, removed,
moved, and edited targets remain visible.

Use `--confirm`, `--refute`, or `--supersede` when the response has that meaning.
See [Reply to a finding](docs/workflows/agents.md#reply-to-a-finding)
for handles and retries. Reply notifications and coordinator broadcasts are
planned. For work that needs
ownership or tracked completion, a comment-sized task becomes a child issue.

For agent-heavy workspaces, `poomsae init --with-agents` also writes a managed poomsae
briefing into agent guidance files. It refreshes existing real `AGENTS.md` and
`CLAUDE.md` files, or creates `AGENTS.md` when neither exists, without
overwriting the rest of either file. Codex-only workspaces can instead use
`poomsae init --with-codex-hooks` to install dynamic contract context and the
`work.attention` SessionStart lifecycle into `.codex/hooks.json`.

## Why poomsae

- **Built for agents.** Stable short refs, `--json` and `--agent` output,
  idempotent creates, semantic-aware search, a claim flow, and predictable
  failure modes agents can script against.
- **Made for humans too.** `poomsae tui` and `poomsae ui` browse, triage, and
  supervise agent-written work over the same data. The browser app is served
  by the daemon and needs no separate backend.
- **Local-first, repo-clean.** One Go binary, no runtime dependencies. Issue
  state lives in SQLite under `POOMSAE_HOME`; your repo commits only a small,
  secret-free `.poomsae.toml`.
- **Auditable by design.** Closing an issue is an explicit completion claim with
  a reason, message, evidence, and actor attribution, on top of editable
  comments and durable events.

## How poomsae compares

poomsae is intentionally small. It is not a project-management suite, a git
workflow engine, or an agent worker pool. It is a durable task ledger that
humans and agents can both operate.

It is also not a SaaS issue tracker. Linear, Jira, GitHub Issues, ClickUp, and
similar tools are shared online systems for planning, dashboards, assignment,
and cross-team reporting. poomsae is local-first, instant from the CLI/TUI, and
designed around agent-first ergonomics: stable refs, predictable output,
idempotent creates, claim flows, and evidence-based closes. See
[Comparisons with SaaS issue trackers](docs/guide/comparisons.md) for the
matrix.

[Beads](https://github.com/gastownhall/beads) keeps issue state in a
project-local `.beads/` Dolt database with native history, branching, and
push/pull. [git-bug](https://github.com/git-bug/git-bug) stores issues as git
objects under custom refs and syncs them over `git push` and `git pull`. poomsae
makes a different bet: the ledger is a local service next to your workspaces,
not data carried in the repository. That keeps the workspace clean, works the
same in non-git directories, and keeps issue history out of code history. The
trade-off is that poomsae does not ride git remotes for sharing; the remote daemon
and federation cover that instead.

Moving from Beads? See
[Migrating from Beads](docs/guide/migrating-from-beads.md).
`poomsae import --source-format beads` drives the `bd` CLI and merges your issues
into a poomsae project.

## Documentation

The [docs site](docs/) is the definitive reference:

- Get started: [Quickstart](docs/get-started/quickstart.md) ·
  [Install](docs/get-started/install.md) ·
  [Changelog](docs/changelog.md)
- Guide: [Concepts](docs/guide/concepts.md) ·
  [Workspaces and projects](docs/guide/workspaces-projects.md) ·
  [Issue attachments](docs/guide/attachments.md) ·
  [Semantic search](docs/guide/semantic-search.md) ·
  [Migrating from Beads](docs/guide/migrating-from-beads.md)
- Reference: [CLI](docs/reference/cli.md) ·
  [Model Context Protocol](docs/reference/mcp.md) ·
  [Configuration](docs/reference/configuration.md)
- Workflows: [Agent workflows](docs/workflows/agents.md) ·
  [Gated workflows](docs/workflows/gated-workflows.md) ·
  [Sharing models](docs/workflows/sharing.md)
- Operations: [GitHub sync](docs/operations/github-sync.md) ·
  [Notion sync](docs/operations/notion-sync.md) ·
  [Plane sync](docs/operations/plane-sync.md) ·
  [Linear sync](docs/operations/linear-sync.md) ·
  [Twenty sync](docs/operations/twenty-sync.md) ·
  [Todoist sync](docs/operations/todoist-sync.md) ·
  [TickTick sync](docs/operations/ticktick-sync.md) ·
  [Forgejo connector](docs/operations/forgejo-connector.md) ·
  [Remote daemon](docs/operations/remote-daemon.md) ·
  [Federation](docs/operations/federation.md) ·
  [Hosted mode](docs/operations/hosted-mode.md) ·
  [Containers](docs/operations/containers.md) ·
  [PostgreSQL](docs/operations/postgres.md) ·
  [Backup and restore](docs/operations/backup-restore.md)
- Development: [Embedding poomsae in Go](docs/development/embedding.md) ·
  [Contributing](docs/development/contributing.md)

## For coding agents

Run `poomsae quickstart` (alias `poomsae agent-instructions`) for the operating
contract: search before creating, pass an idempotency key on create, prefer
`--agent` output, claim work with a time limit (`poomsae claim <ref> --ttl 2h`) and
renew it, and close only when the work is verified. Close each verified issue
promptly with valid evidence and a substantive message. [Agent workflows](docs/workflows/agents.md) is the same
contract in long form.

On `main`, install the contract for Claude Code and Codex sessions that start
inside a poomsae workspace on this machine. Setup selects whichever are installed.

Not in the latest release yet.

```sh
poomsae agent setup
```

For Codex, open Codex and run `/hooks` to trust the hooks.

`poomsae agent-hook` requires Poomsae 0.19.0 or later. See
[Contract in every session](docs/workflows/agents.md#contract-in-every-session)
for all 18 targets, runtime requirements, attention capabilities and additional
Codex homes. Contract and available attention hooks are enabled by default;
use `poomsae agent-hook install --contract-only` to opt out of attention, or
`poomsae init --agent-hooks=codex,pi` for project setup.

Print the shorter managed briefing without changing the repository:

```sh
poomsae quickstart --format contract
# equivalent alias; selectors are accepted for user-local integrations
poomsae agent-instructions --format contract --workspace /path/to/workspace
poomsae quickstart --format contract --project spoke-project
```

Contract output is marker-free, has no terminal framing, works without an
initialized workspace, and performs no workspace mutation. It is rendered from
the same canonical body that `poomsae init --with-agents` places between its
managed markers, so static and session-injected guidance cannot drift.

For a Codex SessionStart response without a stdin payload, run
`poomsae agent-contract-hook`. To replace the prompt, use
`poomsae agent-contract-hook --source ./agent-prompt.txt`; a missing file uses the
built-in contract and an empty file supplies an empty prompt.

Codex SessionStart hooks require structured JSON rather than plain contract
stdout. For workspace hooks, use `poomsae init --with-codex-hooks`; see the
[init reference](docs/reference/cli.md#workspace-initialization) for contract
deduplication and the separate attention lifecycle.

## Contributing

See [Contributing](docs/development/contributing.md) for the repository layout
and local checks (`make test`, `make lint`, `make vet`, `make nilaway`).
Licensed under the terms in [LICENSE](LICENSE).
