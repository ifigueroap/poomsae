---
title: Agent sync
description: How an agent keeps its own copy of Poomsae issues without a built-in integration.
last_edited: 2026-11-24
---

# Agent sync

Poomsae does not ship connectors for other trackers. An agent that needs a
copy of the issues builds that sync itself with the local CLI, the MCP tools,
and the event log.

## Read and write issues

Use `--json` when a program consumes the result. These commands print one JSON
document and exit with a non-zero status on failure:

```sh
poomsae list --json
poomsae show <ref> --json
poomsae search "<words>" --json
poomsae create "<title>" --idempotency-key <key> --json
poomsae meta get <ref> --json
```

The same operations are MCP tools: `poomsae.list`, `poomsae.show`,
`poomsae.search`, `poomsae.create`, `poomsae.edit`, `poomsae.comment`,
`poomsae.close`, and `poomsae.set_metadata`. Load a section with its
`poomsae.load_*` tool before calling the tools in that section.

## Remember an external id

Issue metadata is the place to store the other system's id. Pick one key and
keep it stable:

```sh
poomsae meta set <ref> sync.external_id <their-id> --json
poomsae meta get <ref> sync.external_id --json
```

`poomsae import` also matches an existing issue by `source` plus `external_id`
and updates that issue in place. Use that when the other system is the source
of a batch, and use `sync.external_id` when the agent owns the mapping.

## Follow the event log

`poomsae events` lists events in id order. Pass the last id you stored as
`--after` and read the next page with `--limit`:

```sh
poomsae events --after 0 --limit 100 --json
poomsae events --after <last-id> --limit 100 --json
```

The MCP tool `poomsae.events` takes the same `after` cursor. Stop when a page
is empty, and store the highest event id you applied. `--tail` streams new
events until the process stops.

## Copy the database

`poomsae export` writes a JSONL snapshot of the local database. `poomsae import`
reads that snapshot back. Stop the daemon before either command unless you pass
the flag that allows a running daemon. Export is a full copy, not a cursor;
use the event log for the changes since the last export.
