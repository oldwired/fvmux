# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current repo state

The v1 multiplexer is **implemented** — ~15k LOC of Go across `cmd/fvmux/` and `internal/*` (app, layout, commands, config, sshmgr, sftp, prefix, session, menus, palette, statusbar, theme, splash, …), with unit tests per package plus golden-frame integration tests under `test/headless/`. It builds and `go test ./...` is green. `implementation-plan.md` remains the authoritative design spec — read the relevant section before changing a subsystem, because it pins the cross-cutting decisions (prefix key, config format, layout algebra, command-registry design) that the code conforms to. When the plan and the code disagree, the code is newer; reconcile rather than blindly following either.

## Project: fvmux

`fvmux` is a terminal multiplexer built on the `fv-go` TUI framework. Unlike tmux's tiled panes in one rectangle, fvmux provides **draggable floating windows that each contain a nested split tree of panes** — a multiplexer with a window manager. v1 ships as a foreground process intended to run inside an outer tmux session (detach/reattach is deferred to v2 via a future `fvmuxd` daemon split).

Target module path: `github.com/oldwired/fvmux`. v1 scope = multiplexer core + SSH connection manager + SFTP browser + hex preview for binaries.

## Relationship to fv-go

fvmux is a *consumer* of `github.com/oldwired/fv-go` (located at `/Users/apfau/GolandProjects/fv-go/`, module `github.com/oldwired/fv-go`). The dependency is one-way: fvmux imports `pkg/fv/...` and treats it as a stable API. Used surface includes `app.Application`/`Desktop`, `views.{Window, SplitGroup, Group, View, Base}`, the `widgets/*` family (terminal, fuzzyfinder, popupmenu, treeview, markdown, taskprogress, hexedit, imageview, cpucore, ramview, notification), `menus.{MenuBar, StatusLine}`, `dialogs.*`, `consts.OfPreProcess`, `geom.Rect`, `drivers.Event`, and `sixel`.

Dual-repo development uses a **Go workspace**, not a `replace` directive. `go.mod` pins a real fv-go pseudo-version (so CI and releases fetch it like any other module); a gitignored `go.work` at the repo root sits a `use ../fv-go` on top of that pin, so local builds see the working tree:

```
go 1.25.0

use (
	.
	../fv-go
)
```

When fv-go changes land and need to flow into fvmux: commit + push fv-go, then `go get github.com/oldwired/fv-go@main` here to bump the pseudo-version in `go.mod`. The `go.work` keeps the local edits visible in the meantime. CI never sees `go.work`, so it builds against whatever's pinned in `go.mod`.

**When you hit a gap or bug in fv-go: stop and call it out.** Do not patch fv-go from inside this session, and do not work around it in fvmux. Surface the missing API or broken behaviour to the user in plain terms — what you needed, where, and why fvmux can't proceed without it — then wait. The fix lands in a separate fv-go session immediately afterward; you resume here against the updated tree. This is the same discipline as Stage 0: every fv-go change is its own small, reviewable PR rather than a drive-by edit smuggled inside an fvmux feature. Divergence between the two repos is the failure mode to avoid.

## Architecture overview (read the plan for details)

A few load-bearing design choices that span multiple future files:

- **Command registry is the single source of truth** (`internal/commands/`). One `Command` struct per action carries the ID, category, name, menu label with Borland `~X~` hotkey markers, chord, action, and enabled-predicate. The registry feeds **menus**, the **Ctrl-G P command palette**, and the **auto-generated cheatsheet** — they cannot drift apart by construction. When adding a feature, add the command entry first; the three surfaces follow.

- **Layout is a strict binary tree** (`internal/layout/`). A `PaneNode` is either `Leaf{Pane}` or `Split{Orientation, Ratio, A, B, Parent}`. All layout operations (SplitH/V, Close with single-child collapse, Swap, Zoom, BreakOut, JoinFrom, FocusDir) preserve invariants asserted by `CheckInvariants`. `FocusDir` uses geometric nearest-centre matching, not tree traversal — this matters for nested-split UX. Property tests with seeded random op sequences are part of the design, not optional.

- **Sessions vs profiles**: profiles are **read-only spawn templates** (command, args, env, cwd); sessions are **mutable live state** (windows, layout, focus). A pane carries `Profile string` but is otherwise independent once spawned.

- **SSH connection pool is shared** between the connection manager (Ctrl-G H) and the SFTP browser (Ctrl-G F). Both call into `internal/sshmgr/pool.go`, which keeps a `ControlPath` master per alias via `ssh -M -N -o ControlPersist=600`. The same alias used twice reuses the master; refcounted release.

- **TOML config under XDG** (`~/.config/fvmux/`): `config.toml`, `profiles.toml`, `keybindings.toml` (overrides only — `command = ""` removes a default), `sessions/<name>.toml`, `hosts.toml` (merged with `~/.ssh/config`), `state.toml` (managed first-run/version state).

- **Prefix key**: `Ctrl-G` by default, configurable. Implemented via fv-go's `OfPreProcess` hook (`internal/prefix/`). Double-tap `Ctrl-G Ctrl-G` sends a literal `Ctrl-G` to the focused pane.

- **Status line** layers left/middle/right composition (`internal/statusbar/`) on top of fv-go's native `LeftItems`/`RightItems` slots (added in Stage 0 PR C2). A 1s ticker posts a synthetic redraw event via `app.PostEvent` rather than racing on `MarkDirty`.

## Implementation order

The plan's bottom section lists the Stage 1 bootstrap order; follow it. Don't, for example, start on the SFTP browser before the command registry exists, because every UI surface routes through registered commands.

1. `cmd/fvmux/main.go` + `internal/app/` + `internal/prefix/` — minimal multiplexer responding to `Ctrl-G c` and `Ctrl-G ?`.
2. `internal/commands/` — registry, then populate `defaults.go`.
3. `internal/layout/` — algebra + invariant + property tests before any window has >1 pane.
4. `internal/session/` + `internal/config/` + `internal/profile/`.
5. `internal/keys/` + `internal/menus/` (builder) + `internal/palette/`.
6. `internal/statusbar/` + `internal/theme/`.
7. `internal/clipboard/` (wraps `atotto/clipboard` or `golang.design/x/clipboard` — **not** added to fv-go; the plan is explicit that fv-go's clipboard surface stays OSC-52 only).
8. `internal/splash/` + `internal/cheatsheet/` (auto-generated from the registry).
9. `internal/sshmgr/` (parses `~/.ssh/config` via `kevinburke/ssh_config`).
10. `internal/sftp/` (browser + preview dispatch + hex view for binaries).
11. `test/headless/` against fv-go's `pkg/fv/term/headless.go` backend.

## Build/test commands (once bootstrapped)

The Makefile per the plan should expose `build`, `test`, `lint`, `race`, `install`. The acceptance criteria from the plan's Verification section:

```bash
go build ./...
GOOS=linux   go build ./...
GOOS=windows go build ./...
go test -race ./...
go test ./test/headless/ -update=false   # golden-frame integration
```

Smoke tests under `test/smoke/*.sh` target real terminals (kitty, ghostty, alacritty, iTerm2, tmux-inside-tmux); SFTP smoke uses a docker-compose'd OpenSSH on `:2222`.

## Things worth knowing while building

- **Don't hardcode terminal attrs** — Stage 0 PR D1 added `pkg/fv/theme/theme.go` with a `Palette` struct and `theme.Get()` / `theme.Set()`. Everything fvmux renders should pull colours from the active palette so the theme picker (Ctrl-G T) actually works.

- **Stoppable cleanup** is generic in fv-go (`app.go` walks the view tree post-order calling `Stop()` on anything implementing the `Stoppable` interface, including from a `recover()` path). Don't add bespoke PTY cleanup to fvmux's quit handler — make panes Stoppable and trust the walk.

- **Window numbers** only go 1–9 in fv-go's badge rendering. The plan calls for a small fvmux-side override rendering `+` for window 10+; the `Ctrl-G w` window-list dialog handles the long tail.

- **Bell debounce ≥ 500ms** between consecutive `OnBell` flashes — the upstream callback is already debounced for activity but bell-flash strobing is a UX hazard the plan calls out explicitly.

- **SIGINT / signals**: `Ctrl-G Ctrl-C` forwards `SIGINT` to the focused pane's PTY (writes `0x03`); the once-mooted `Ctrl-G Ctrl-C Ctrl-C` quit chord was dropped (overloading the universal interrupt with "kill the whole multiplexer" is a footgun, and explicit Quit already exists). `internal/app/signals.go` also installs an OS-signal handler: SIGTERM/SIGHUP/SIGINT trigger a graceful, session-saving shutdown via the normal `OnQuitRequest` path (a second signal hard-exits if the loop is wedged). SIGWINCH is handled by the fv-go backend, not here.

- **Whimsy budget is capped** — see the plan's "Whimsy budget" section. Splash, named themes with taglines, and a small itemized easter-egg list are in scope; everything else stays quiet and professional.
