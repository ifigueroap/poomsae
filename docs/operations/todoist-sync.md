---
title: Todoist sync
description: Import one Todoist project and optionally synchronize standalone task completion and reopening.
last_edited: 2026-11-20
---

# Todoist sync

Todoist sync mirrors one Todoist project into one Poomsae project. By default, the
daemon imports active tasks and completion history every five minutes. If you
enable two-way status sync, explicit close and reopen actions in Poomsae also
update standalone, non-recurring tasks in Todoist. All other fields remain
incoming-only.

## Configure the daemon

1. Get a personal API token from Todoist's integration settings, or prepare an
   operator-obtained OAuth token. OAuth tokens require `data:read` for import
   and `data:read_write` for two-way status sync. Poomsae does not need deletion
   scopes and does not run an OAuth authorization flow. Client workstations need
   only their normal Poomsae credentials. For authentication, permissions, and
   endpoint limits, see the
   [official Todoist API](https://developer.todoist.com/api/v1/).
2. Set `POOMSAE_TODOIST_TOKEN` in the daemon's service environment. To read from a
   different environment variable, add this block to `<POOMSAE_HOME>/config.toml`:

   ```toml
   [todoist_sync]
   token_env = "EXAMPLE_TODOIST_TOKEN"
   ```

3. Restart the daemon after changing its service environment or configuration.

The daemon sends all requests to the public `https://api.todoist.com` origin and
rejects redirects. It captures the token once per run and never writes it into
binding config, exports, command output, or provider error messages. When you
rotate the token, keep the same Todoist account; switching to a different
account blocks the saved binding. Embedded services use `Config.TodoistSync`. If
you want continuous remote polling on a selected daemon, set
`autostart_idle_timeout = "0"`.

## Enable and operate

From a bound Poomsae workspace, run these commands to enable, inspect, or pause
Todoist sync:

```sh
poomsae sync todoist enable --todoist-project project123
poomsae sync todoist once
poomsae sync todoist status
poomsae --agent sync todoist status
poomsae --json sync todoist status
poomsae sync todoist enable --status-sync two-way
poomsae sync todoist disable
```

Pass the opaque Todoist project ID with `--todoist-project`, and select the
native Poomsae project with Poomsae's global `--project` flag. When you enable sync,
Poomsae resolves the credential's account identity and validates that the selected
project is active. Shared projects work as long as that account can access them,
and task ownership does not restrict imports to tasks owned by the credential
holder.

New bindings start in one-way mode with `[Todoist] ` title prefixes. When you
re-enable a binding, Poomsae keeps any mode, interval, and title presentation
options you omit. Pass `--interval 2m` to change the polling interval, or pass
`--title-prefix=false` to keep the original titles and add the `todoist` label.
Disabling sync stops polling and outbound delivery while keeping your native
issues, bindings, and pending status intent.

By default, the completion-history floor starts thirty days before you first
enable sync. To choose a different floor on that first enable, pass
`--history-since 2026-09-01` or a whole-second RFC3339 instant. Poomsae imports all
active tasks regardless of that floor. Once set, the account, project, and
history floor cannot change. To sync a different scope, use a separate Poomsae
project. This prevents an omitted option or credential change from silently
broadening an existing binding.

## Field ownership and completion

| Field | Ownership |
|---|---|
| Title, description, labels, priority, author and assignee | Imported from Todoist under Poomsae's existing import conflict rules; local edits are never sent upstream. |
| Open/closed status | Incoming in one-way mode; explicit Poomsae close/reopen events also deliver in two-way mode. |
| Due dates, deadlines, recurrence, reminders, comments, sections and parent relationships | Remain in Todoist; not projected or written back. |
| Task creation, deletion and project membership | Remain in Todoist; Poomsae never creates, deletes or moves provider tasks. |

Todoist priority 4 maps to Poomsae priority 1, and Todoist priority 1 maps to Poomsae
priority 4. Imported labels use Poomsae's canonical import spelling: lowercase
names, whitespace and unsupported punctuation converted to hyphens, and bounded
names with duplicates removed. Original labels stay in Todoist. Imported issue
bodies include a link to the Todoist task. A completed task maps to closed with
reason `done`, and a local close reason does not change Todoist's completion
type.

Pending Poomsae intent wins until a fresh provider read verifies it; otherwise,
fresh provider observations determine status. Repeated imports and verified
converged writes do not echo changes back to Todoist. A newer local close or
reopen event replaces older queued intent. Poomsae's existing claim and admission
checks fence disabled bindings, changed settings, and superseded intent before
dispatch.

Poomsae still imports recurring tasks, and it blocks their outbound completion
because Todoist's close operation moves them to their next date. Complete
recurring tasks in Todoist instead. Poomsae also blocks closing tasks that have
active subtasks, reopening subtasks, and reopening tasks in an archived section,
because Todoist applies those operations to the subtasks, parent tasks, or
section as well. Todoist offers no atomic conditional status operation, so
provider edits made between these checks and the write remain an API concurrency
limitation.

## Recovery and limits

Each run reads every active task using project-filtered cursor pagination. It
reads completion history in windows of at most three months, which is the limit
Todoist allows per completion-date request:

- The first run, and the first run after a title-presentation change, read
  history from the configured floor.
- Later runs read completions since the start of the last successful run, with a
  two-minute overlap. A failed run does not advance that checkpoint.

A failed read prevents a partial content import. Status scans have their own
durable checkpoints and can make progress even when content reads fail.

Two-way status scans read each mapped task's active state. Once Poomsae has
verified a task's completion, it stops looking that task up in history as long
as the task stays inactive and the Poomsae issue stays closed. Todoist returns
`404` for both completed and deleted tasks, so an open Poomsae issue always needs
fresh history evidence before Poomsae closes it. For a task last seen open, Poomsae
searches history from that observation onward; other lookups start at the
configured floor.

Todoist sometimes reports unknown, `null`, creation or update times. When that
happens, Poomsae uses the task's other known timestamps instead, or falls back to
the history floor if none are known. Two-way status scans and imports never move
such a task behind the version Poomsae already stored. While no one has edited the
issue in Poomsae, its status and title still follow Todoist. Other fields update
only when a newer timestamp, such as a later completion time, moves the task
past that version.

An active task always wins over historical completions with the same ID,
including old recurring occurrences. Poomsae never treats a missing task as
completed or deleted. It accepts a completion only when it finds the exact task
in scoped history and confirms the task is still not active after that read.
Poomsae skips deleted tasks in completion history. Deleted, moved, or inaccessible
tasks block outbound delivery and keep your native issues intact. An archived or
deleted project stops status delivery before any write. Tasks completed before
the configured history floor cannot be resolved through that history.

Reads retry `429` and `5xx` responses with exponential backoff and honor
`Retry-After`. A successful response with an empty body fails the run without
advancing it, so no safety check or history read relies on missing data.
Authentication and permission failures block the binding. Poomsae sends a close or
reopen once and verifies it with a fresh read. If a response is lost or
unverified, Poomsae keeps the pending intent, and the next run reads Todoist before
deciding whether another write is needed. Check `poomsae sync todoist status` for
pending changes and blocked errors. After repairing credentials or provider
state, run `poomsae sync todoist once` or wait for the next scheduled run.
