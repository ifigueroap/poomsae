---
title: Workspaces and projects
description: Understand how workspaces resolve Poomsae projects and configure local or remote daemon targets.
last_edited: 2026-11-20
---

# Workspaces and projects

Poomsae keeps repository files separate from issue data. Your repositories and
workspaces store only enough information to resolve a project, while the
database lives under `POOMSAE_HOME` unless `POOMSAE_DB` points somewhere else.

## Initialize

Bind a workspace to a project by running:

```sh
poomsae init
```

In a Git workspace, `poomsae init` derives the project name from the Git remote.
To choose the name yourself, pass `--project`:

```sh
poomsae init --project product
```

`poomsae init` writes `.poomsae.toml` and makes sure `.poomsae.local.toml` is ignored.
You can use `.poomsae.local.toml` for per-machine settings such as a remote daemon
URL.

Inside a Git repository, Poomsae searches for those files only as far as the
nearest Git root, including when you enter the workspace through a symlink. A
`.poomsae.toml` above that boundary cannot bind a nested repository or redirect
`poomsae init` output outside it. Non-Git workspaces still use normal ancestor
discovery.

## Bind many workspaces to one project

To point multiple workspaces at one project, pass the same project name in each
workspace:

```sh
cd ~/code/product
poomsae init --project product

cd ~/code/product-worktree
poomsae init --project product
```

Both workspaces now resolve to the same project in the same local Poomsae
database, sharing issue short IDs, labels, links, and events.

## Run from outside a workspace

To run commands from another directory, pass the global `--workspace` flag:

```sh
poomsae --workspace ~/code/product ready --unowned
poomsae --workspace ~/code/product show abc4
```

This helps with scripts, cron jobs, and agents that keep their own working
directory.

## Project commands

List and inspect projects:

```sh
poomsae projects list
poomsae projects show product
```

Create a project by name without binding the current directory:

```sh
poomsae projects create research
```

`projects create` talks to the daemon only. It creates or returns the active
project named `research` without writing `.poomsae.toml`, `.gitignore`, or agent
guidance files, and without attaching a Git alias. Use it for projects that do
not map one-to-one to a repository workspace, then run
`poomsae init --project research` later from any workspace that should resolve to
the same project.

Rename a project:

```sh
poomsae projects rename product platform
```

Merge accidental duplicates:

```sh
poomsae projects merge old-repo new-repo --rename-target new-repo
```

Archive and restore a project:

```sh
poomsae projects remove old-lab
poomsae projects restore old-lab
```

`projects remove` hides the project from normal resolution while preserving its
events for audit. The project name stays reserved, and `poomsae init` rejects the
same name while the project is archived.

Permanently delete an archived project:

```sh
poomsae projects purge old-lab --force --confirm "PURGE old-lab"
```

`projects purge` is irreversible. It deletes all project data and frees the
name so `poomsae init --project old-lab` works again afterward.

Archive the project first with `poomsae projects remove`. Purging an active
project fails with `project_not_archived`.

Two safeguards protect against accidental data loss:

- `--force` is always required.
- `--confirm` must be the exact string `PURGE <project>` (for example
  `"PURGE old-lab"`), and Poomsae rejects any wrong value. If you omit `--confirm`
  at a terminal, Poomsae asks you to type `PURGE <project>` and rejects anything
  else. Without a terminal, Poomsae refuses and tells you to pass `--confirm`.

Pass an optional `--reason` to record a free-text note in the audit tombstone:

```sh
poomsae projects purge old-lab --force --confirm "PURGE old-lab" \
  --reason "decommissioned after team reorg"
```

Pass `--json` to receive the audit tombstone with row counts as JSON.

**Federation.** You cannot purge a project that has a federation binding:

- If the project is a **spoke**, run `poomsae federation leave old-lab` first.
- If the project is a **hub**, purge is not currently supported. Handle hub
  teardown manually before purging the project.

!!! warning
    `projects purge` cannot be undone. Confirm you have the right project with
    `poomsae projects show <project>` *before* you run `poomsae projects remove`,
    because `projects show` resolves active projects only. Once the project is
    archived, the `PURGE <project>` confirmation string is your last check
    before deletion.

To detach an alias when a workspace identity was attached to the wrong project,
run:

```sh
poomsae projects detach github.com/example/wrong
```

Run `poomsae projects show <project>` before destructive or structural project
operations so you know which project and aliases are affected.

## `.poomsae.toml`

The committed binding file is intentionally small:

```toml
# This repository tracks its work in poomsae.
# Coding agents: read `poomsae quickstart --format contract` before changing code here.
version = 1

[project]
name = "product"
```

If you rerun `poomsae init` on a binding that lacks the contract command, Poomsae
adds those two lines above the existing contents and changes nothing else in
the file. If Poomsae cannot write the header, `poomsae init` reports a warning and
continues setup.

Do not put tokens or host-specific daemon URLs in `.poomsae.toml`.

## `.poomsae.local.toml`

Git ignores `.poomsae.local.toml`. A common use is routing one workspace to a
remote daemon:

```toml
version = 1

[server]
url = "http://100.64.0.5:7777"
```

When both are set, `POOMSAE_SERVER` takes precedence over `.poomsae.local.toml`.

## Non-git workspaces

Poomsae works without Git. Pass an explicit project name:

```sh
mkdir ~/scratch/research
cd ~/scratch/research
poomsae init --project research
```

The issue model does not depend on Git commits. Git is only one way to derive a
default project name and one possible source of close evidence.
