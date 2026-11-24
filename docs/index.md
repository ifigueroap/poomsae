---
title: poomsae カタ documentation
description: Documentation for poomsae, the local-first issue tracker for humans and coding agents.
---

# poomsae documentation

poomsae カタ is a local-first issue tracker for coding agents and the humans
steering them. Agents manage the task ledger from the CLI, and you supervise
their work from the terminal or the browser. These pages are the operating
documentation. For the product story, start at the
[overview](https://github.com/ifigueroap/poomsae/) and the
[guide](https://github.com/ifigueroap/poomsae/guide/).

<section class="poomsae-install" data-install-panel markdown>
<div class="poomsae-install-header">
<span class="poomsae-install-label">Install</span>
<div class="poomsae-install-platforms" role="group" aria-label="Choose your operating system">
<button type="button" data-install-platform-button="macos" aria-pressed="true">macOS</button>
<button type="button" data-install-platform-button="linux" aria-pressed="false">Linux</button>
<button type="button" data-install-platform-button="windows" aria-pressed="false">Windows</button>
</div>
</div>
```sh { .poomsae-install-command data-install-platform-content="macos" }
brew install poomsae
```

```sh { .poomsae-install-command .poomsae-install-command--fallback-hidden data-install-platform-content="linux" hidden="hidden" }
curl -fsSL https://github.com/ifigueroap/poomsae/install.sh | bash
```

```powershell { .poomsae-install-command .poomsae-install-command--fallback-hidden data-install-platform-content="windows" hidden="hidden" }
powershell -ExecutionPolicy ByPass -c "irm https://github.com/ifigueroap/poomsae/install.ps1 | iex"
```
</section>

## New in 0.23.0

You can turn on search by meaning with one command or from **Settings** in the
web UI. In the terminal UI, you can move issues, change repeating schedules, and
edit workflows, and you can set up attachments with `poomsae docbank setup`.
Assistants that use MCP can set up without a shell and wake when their inbox
changes. Large stores and federated projects also search, create, and sync
faster. Read the [0.23.0 release notes](changelog.md#0230) for the schema 50
upgrade, the PostgreSQL migration, and contributor credits, and see
[Updating Poomsae](get-started/install.md#updating-poomsae) for installation and
federation upgrade guidance.

## Quickstart

```sh
cd your-repo
poomsae init                              # bind this workspace to a poomsae project
poomsae create "fix login race"           # prints a short id, e.g. abc4
poomsae list                              # see open work

# close only when the work is verified
poomsae close abc4 --done \
  --message "Fixed the login race; tests pass." --commit <sha>

poomsae tui                               # browse and triage interactively
poomsae ui                                # open the browser application
```

`poomsae create` prints each issue's short ID, which you use in later commands. If
you work with coding agents, run `poomsae init --with-agents` to write poomsae's
operating contract into `AGENTS.md`/`CLAUDE.md`, and run `poomsae quickstart` to
print the full agent contract. Follow the
[Quickstart](get-started/quickstart.md) for the complete walkthrough.

## Two ways to supervise

![poomsae TUI showing a simulated issue hierarchy](/docs/assets/screenshots/tui/hero.svg)

Run `poomsae tui` to triage issues in the terminal, or run `poomsae ui` to open the
same ledger in a full browser workspace:

![poomsae Web UI showing a synthetic project and issue hierarchy](/docs/assets/screenshots/web-ui/workspace.png)

The docs screenshot workflow generates both images from disposable synthetic
data. See the [Web UI guide](guide/web-ui.md) for projects, collections, issue
editing, relationships, recurrences, and configured-daemon switching.

## How it works

When you run a command, the `poomsae` CLI resolves the project from your
workspace, `.poomsae.toml`, or `--project`, and talks to a local daemon, starting
one automatically when needed. That daemon owns a SQLite database under
`POOMSAE_HOME`, applies mutations, and records an event stream that the CLI, TUI,
and hooks all read. Because your repo commits only the small `.poomsae.toml`
binding, issue history stays out of your code history.

Search is lexical by default, and you can opt into
[semantic search](guide/semantic-search.md) with a local or hosted
OpenAI-compatible embeddings endpoint. Optional
[GitHub sync](operations/github-sync.md) can mirror upstream GitHub issues into
poomsae, and federation can replicate selected projects through a hub.
Private-network remote daemon modes are explicit. Operators can use bearer auth
on trusted private HTTP, or opt a single-user private IP into tokenless writes.
See [Concepts](guide/concepts.md) and [Architecture](design/architecture.md)
for the full model.

Go applications can also run poomsae in-process through the listener-free
`github.com/ifigueroap/poomsae` service. The host application mounts the same HTTP API and
owns the listener, authentication boundary, and process lifecycle. See
[Embedding poomsae in Go](development/embedding.md) for details.

## When to use poomsae

Use poomsae when work should stay close to the machine doing it:

- Coding agents need to discover, claim, update, and close work from the CLI.
- You want an instant terminal loop instead of a browser session.
- Work spans local clones, worktrees, experiments, or non-git directories.
- Task state needs to survive chat compaction without becoming a markdown plan.
- Closes need to carry evidence and an audit trail.

poomsae is not a SaaS issue tracker. Linear, Jira, GitHub Issues, and ClickUp are
shared online systems for roadmaps, dashboards, and cross-team reporting,
whereas poomsae is a local ledger for the work itself. You can use them side by
side. See [Comparisons](guide/comparisons.md) for the trade-offs.

## Next steps

<div class="grid cards" markdown>

-   [__Concepts__](guide/concepts.md). The data model and how the pieces fit.
-   [__Web UI__](guide/web-ui.md). Manage projects and issues in the browser.
-   [__CLI reference__](reference/cli.md). Every command and flag.
-   [__Model Context Protocol__](reference/mcp.md). Connect an MCP client over
    stdio or Streamable HTTP to the workspace project, a fixed allowlist, or
    the whole daemon.
-   [__Native MCP inbox events__](operations/mcp-events.md). Deliver signed inbox wakes to a durable receiver.
-   [__Share projects between hubs__](guide/project-relays.md). Connect a selected
    project to your hub and devices.
-   [__Semantic search__](guide/semantic-search.md). Improve issue discovery
    with opt-in embeddings.
-   [__Local daemon profiles__](operations/local-daemon-profiles.md). Keep personal
    and work spokes separate and recover their existing data.
-   [__GitHub sync__](operations/github-sync.md). Bring GitHub issues into poomsae.
-   [__Notion sync__](operations/notion-sync.md). Mirror a Notion task data source with optional two-way status.
-   [__Twenty sync__](operations/twenty-sync.md). Mirror Twenty workspace tasks with optional two-way status.
-   [__TickTick sync__](operations/ticktick-sync.md). Import a task project and send completion back.
-   [__Forgejo connector__](operations/forgejo-connector.md). Bind a Forgejo issue to Poomsae.
-   [__Cron__](reference/cron.md). Share job definitions and create recurring child issues.
-   [__Cron failure alerts__](guide/cron-failure-alerts.md). Notify an inbox when a run fails.
-   [__Plane sync__](operations/plane-sync.md). Mirror a Plane project into native issues.
-   [__Linear sync__](operations/linear-sync.md). Import a selected team and synchronize completion and reopen.
-   [__Todoist sync__](operations/todoist-sync.md). Mirror a Todoist project and sync task completion.
-   [__Agent workflows__](workflows/agents.md). The operating contract for agents.
-   [__Comparisons__](guide/comparisons.md). poomsae vs. SaaS issue trackers.

</div>
