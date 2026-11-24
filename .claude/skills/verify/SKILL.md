---
name: verify
description: Build and drive the poomsae CLI/TUI to verify changes at the real terminal surface
---

# Verifying poomsae changes

## Build

```bash
go build -o /tmp/poomsae-verify ./cmd/poomsae
```

## CLI surface (cmd/poomsae)

- Piped (must be ANSI-free): `/tmp/poomsae-verify list --limit 3 | cat -v`
- TTY-colored: run inside the tmux session below and capture with `-e`.

## TUI surface (internal/tui)

The TUI requires a real pty; use an isolated tmux socket. A local
daemon must be reachable (any workspace-bound directory works; the
repo itself is bound to the `poomsae` project).

```bash
tmux -L poomsaeverify new-session -d -s v -x 120 -y 32
tmux -L poomsaeverify send-keys -t v "cd <repo> && /tmp/poomsae-verify tui" Enter
sleep 3
tmux -L poomsaeverify capture-pane -t v -p        # plain frame
tmux -L poomsaeverify capture-pane -t v -e -p     # with ANSI, for color checks
```

Flows worth driving: Enter (detail), Esc (back), L at ≥140 cols
(split layout), resize below 80 cols (too-narrow hint), `/` + text
(search bar), q (quit modal) then n, ctrl+c (fast quit — pane must
return to shell with no leftover alt-screen garbage).

Teardown: `tmux -L poomsaeverify kill-server`

## Gotchas

- `tmux resize-window -t v -x <cols> -y <rows>` exercises
  WindowSizeMsg handling live.
- Golden snapshots (`testdata/golden/`) are plain text; the test
  harness strips ANSI, so style-only changes don't churn them.
