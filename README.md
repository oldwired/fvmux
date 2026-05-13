# fvmux

> A terminal multiplexer with draggable floating windows that each
> contain a nested split tree of panes — like tmux's panes, but inside
> windows you can drag around with the mouse.

fvmux is built on the [fv-go][fvgo] TUI framework — a Go port of the
classic Free Vision / Turbo Vision desktop. Where tmux gives you
tiled panes in one rectangle, fvmux gives you **windows of split
trees**: a multiplexer with a window manager.

[fvgo]: https://github.com/oldwired/fv-go

The implementation plan is [`implementation-plan.md`](implementation-plan.md);
Claude Code-specific guidance for this repo is in [`CLAUDE.md`](CLAUDE.md).

---

## Status

**v1 alpha.** Stage 0 (fv-go prerequisites) and Stage 1 (the 12 sub-step
fvmux implementation) are complete in skeletal form: build/test/race
green on macOS and Linux, cross-compiles to Windows. Every command
surface (menus, `Ctrl-G P` palette, `Ctrl-G ?` cheatsheet) is wired
from a single `commands.Registry` so they cannot drift apart.

Detach/reattach is intentionally not built into fvmux itself — v1 leans
on an outer tmux for that, via the [`fvmuxa`](#detach--reattach-via-tmux)
wrapper.

What's solid:

- Floating windows with title-bar drag and corner-resize.
- Strict-binary-tree pane layout (`SplitH/V/Close/Swap/Zoom/FocusDir/BreakOut/JoinFrom`),
  property-tested with 100 000 random operations.
- Five layout presets cycled with `Ctrl-G Space`.
- Sticky resize mode (`Ctrl-G R`, then `h/j/k/l`).
- Sync-input broadcast across all panes in a window (`Ctrl-G ~`).
- TOML config + profiles + saved sessions with layout serialisation.
- SSH host picker reading `~/.ssh/config` + `hosts.toml`.
- SFTP browser with binary-sniffing hex preview.
- First-run wizard with prefix-key picker (Ctrl-G / Ctrl-B / Ctrl-A);
  resettable any time via Help → Reset First-Run Wizard.
- Three built-in themes (slate, tokyonight-ish, solarbeach).
- Status bar with live window list, activity/bell markers, focused
  pane title + cwd, clock.
- Confirm-kill before tearing down live panes / windows / app.
- Right-click pane context menu, click-to-focus.

What's stubbed or missing: see [Known limitations](#known-limitations).

---

## Install

fvmux depends on [fv-go][fvgo] via a local `replace` directive — clone
both repos as siblings:

```
~/code/oldwired/
├── fv-go/      # checkout of github.com/oldwired/fv-go
└── fvmux/      # this repo
```

```sh
cd fvmux
make build         # go build ./...
make install       # go install ./cmd/fvmux  (puts fvmux on $GOPATH/bin)
```

`go install` deposits the binary in `$(go env GOPATH)/bin`. If that's
not on your `PATH`, add it.

### Detach + reattach via tmux

fvmux v1 is a foreground process. To get tmux-style detach/reattach
without learning or configuring tmux, install the glue:

```sh
make install-glue   # → ~/.local/bin/fvmuxa
                    #   ~/.config/fvmux/fvmux.tmux.conf
```

Then:

```sh
fvmuxa              # start, or attach to an existing fvmux
# (work normally)
# Press F12 then d  to detach.
fvmuxa              # reattach later.
```

`fvmuxa` runs fvmux inside a tmux session on a **private socket**
(`tmux -L fvmux`) using a minimal tmux config that disables tmux's
status bar, mouse, title management, and window features. You never see
tmux. Quitting fvmux exits the tmux session and the server (the config
sets `exit-empty on`), so you're back in your outer shell.

From inside fvmux, `Ctrl-G D` (or **File → Detach from tmux** in the
menu / the command palette) triggers detach. The command is greyed out
when fvmux isn't running inside tmux.

---

## Quick start

First launch shows a three-step wizard:

1. **Splash** with the project tagline.
2. **Welcome** dialog listing the most useful chords.
3. **Prefix-key picker** — Ctrl-G (default) / Ctrl-B / Ctrl-A.

Hit Continue / OK / pick. The choice is persisted to
`~/.config/fvmux/config.toml` and applied immediately to every chord
in the registry.

Lost or want to redo the wizard? **Help → Reset First-Run Wizard…**

---

## Key bindings

Every chord goes through the configured prefix. Tables below assume
the default `Ctrl-G`. Press **`Ctrl-G ?`** inside fvmux for the full,
auto-generated cheatsheet.

### File / session
| Chord | Action |
|-------|--------|
| `Ctrl-G c` | New window (default profile) |
| `Ctrl-G C` | New window from a profile |
| `Ctrl-G S` | Save session (requires `-session=NAME`) |
| `Ctrl-G D` | Detach from tmux (when running inside fvmuxa) |
| `Ctrl-G ,` | Rename current window (sticky user title) |

### Window navigation
| Chord | Action |
|-------|--------|
| `Ctrl-G n` / `p` | Next / previous window |
| `Ctrl-G 1`…`9`   | Focus window by number |
| `Ctrl-G Tab`     | Toggle to the last-focused window |
| `Ctrl-G w` / `f` | Window list / find window (fuzzy) |
| `Ctrl-G &`       | Kill the current window (confirms) |

### Pane layout
| Chord | Action |
|-------|--------|
| `Ctrl-G %` | Split horizontal (vertical splitter, panes side-by-side) |
| `Ctrl-G "` | Split vertical (horizontal splitter, panes stacked) |
| `Ctrl-G x` | Close focused pane (confirms if alive) |
| `Ctrl-G z` | Zoom / unzoom focused pane |
| `Ctrl-G h j k l` | Focus pane in direction |
| `Ctrl-G o` / `;` | Next / previous pane (tree order) |
| `Ctrl-G { }` | Swap focused pane with neighbour |
| `Ctrl-G !` | Break focused pane out to a new window |
| `Ctrl-G Space` | Cycle layout preset |
| `Ctrl-G R` | Enter resize mode (`h/j/k/l` = 1 cell, `H/J/K/L` = 5; Esc exits) |

### Edit / signals
| Chord | Action |
|-------|--------|
| `Ctrl-G [` | Enter copy mode (scrollback viewer) |
| `Ctrl-G ]` | Paste OS clipboard into focused pane |
| `Ctrl-G /` | Find in scrollback |
| `Ctrl-G ~` | Toggle sync-input (broadcast typing to all panes in window) |
| (menu) | Send Interrupt / Quit / EOF / SIGTERM to focused pane |

### View
| Chord | Action |
|-------|--------|
| `Ctrl-G z` | Zoom |
| `Ctrl-G T` | Theme picker |

### Connections / transfer
| Chord | Action |
|-------|--------|
| `Ctrl-G H` | SSH host picker (reads `~/.ssh/config` + `hosts.toml`) |
| `Ctrl-G B` | Show `hosts.toml` path |
| `Ctrl-G F` | SFTP browser |

### Meta
| Chord | Action |
|-------|--------|
| `Ctrl-G P` | Command palette (fuzzy over every command) |
| `Ctrl-G ?` | Cheatsheet |
| `Ctrl-G Ctrl-G` | Send a literal Ctrl-G to the focused pane |

Right-click a pane for a context menu (split / zoom / rename / send
signal / respawn / kill). Left-click a non-focused pane to focus it.

---

## Configuration

All files live under `~/.config/fvmux/` (or `$XDG_CONFIG_HOME/fvmux/`).
Runtime state lives under `~/.local/state/fvmux/`. fvmux creates these
on first launch.

### `config.toml`

```toml
[general]
prefix_key       = "C-g"        # set by the first-run wizard.
default_profile  = "shell"
confirm_kill     = true         # ask before killing live panes / quitting.
splash_enabled   = true
connect_split    = "vertical"   # how SSH connect splits the focused pane.

[terminal]
scrollback_lines = 10000
shell            = ""           # empty ⇒ honour $SHELL.

[appearance]
theme            = "slate"      # see internal/theme.
bell             = "flash"      # off | flash | notify | both
status_clock     = "15:04"
window_shadow    = true

[sftp]
parallel         = 1
```

Edit by hand; the wizard writes `prefix_key` and `confirm_kill`. Other
fields are read at launch.

### `profiles.toml`

Defines what `Ctrl-G c` and `Ctrl-G C` spawn.

```toml
[[profile]]
name    = "shell"
command = "/bin/zsh"
args    = ["-l"]
cwd     = "~"

[[profile]]
name    = "prod-tail"
command = "ssh"
args    = ["prod-1", "tail", "-f", "/var/log/app.log"]
title   = "prod tail"

[[profile]]
name    = "rust"
command = "cargo"
args    = ["watch", "-x", "test"]
cwd     = "~/code/myproj"
env     = { RUST_BACKTRACE = "1" }
```

Defaults to one `shell` profile running `$SHELL`. `cwd` understands
`~` and `$VARS`.

### `hosts.toml`

Augments / overrides `~/.ssh/config` entries for the SSH picker.

```toml
[[host]]
alias = "prod-1"
user  = "deploy"
host  = "prod-1.internal"
port  = 22
tags  = ["production", "asia"]
notes = "main API box"
```

Entries with `source = "hosts.toml"` win on alias collisions.

### Sessions

`Ctrl-G S` writes the current windows / layout to
`~/.config/fvmux/sessions/<name>.toml` — but only when fvmux was
launched with `-session=NAME`. Restart with the same flag to restore.

The layout DSL is compact and human-readable:

```toml
name    = "work"
created = 2026-05-13T12:00:00Z
active  = 1

[[window]]
id         = 1
number     = 1
title      = "edit"
user_title = "vim"          # sticky from Ctrl-G ,
pos        = { x = 0, y = 0, w = 120, h = 40 }
layout     = 'split-v:0.500{leaf:profile=shell}{split-h:0.500{leaf:profile=shell}{leaf:profile=shell}}'
```

### `keybindings.toml`

Reserved (path is exposed, schema is documented in the plan) but not yet
applied to the registry. Override your prefix via the first-run wizard
or the `[general] prefix_key` field for now.

---

## Themes

Three built-ins: **slate**, **tokyonight-ish**, **solarbeach**. Switch
with `Ctrl-G T`. Each clones fv-go's default palette and tweaks accent
colours; defined in [`internal/theme/theme.go`](internal/theme/theme.go).

User themes loaded from `~/.config/fvmux/themes/*.toml` are on the
roadmap; currently only the built-ins are available.

---

## SSH and SFTP

**`Ctrl-G H`** opens a fuzzy picker over `~/.ssh/config` Host entries
merged with `hosts.toml`. Pick one → fvmux spawns a new window running
`ssh <alias>` so the system ssh handles all auth / known_hosts /
ProxyCommand setup.

**`Ctrl-G F`** opens an SFTP browser. fvmux runs
`ssh -s <alias> sftp` under the hood and pipes
[pkg/sftp](https://github.com/pkg/sftp) over the resulting stream, so
auth / host-key verification are again the system ssh's concern. The
preview area distinguishes text / markdown / image / binary via a
magic-byte sniff (`internal/sftp/binary.go`); binary files would render
in hex once the preview pane is wired (sub-step 12 polish).

---

## Architecture overview

```
cmd/fvmux/                 entry point + flag parsing.
internal/
├── app/                   Mux: runtime state, action wiring, dispatch.
├── commands/              Registry: single source of truth for every action.
├── prefix/                Prefix-key + resize + sync + mouse OfPreProcess views.
├── layout/                PaneNode tree, ops algebra, serialisation.
├── session/               Pane, IDs, snapshot persistence.
├── profile/               Spawn templates.
├── config/                TOML schemas + XDG paths + state file.
├── menus/                 Menu bar built from the registry.
├── palette/               Ctrl-G P fuzzy picker.
├── statusbar/             Bottom-row composition.
├── theme/                 Theme registry + builtins.
├── splash/                First-run wizard.
├── cheatsheet/            Markdown reference generated from the registry.
├── copymode/              Scrollback viewer + paste.
├── clipboard/             atotto wrapper.
├── sshmgr/                Host loader + picker.
├── sftp/                  Client, browser, binary sniff, hex preview helper.
├── whimsy/                Easter-egg helpers (unwired in v1).
└── keys/                  Chord parser (used for display / future overrides).

scripts/                   fvmuxa wrapper + private tmux config.
test/headless/             Unit + parity tests that don't need a TTY.
test/smoke/                Manual checklists + docker-compose for SFTP testing.
```

Three load-bearing patterns:

1. **The registry is the single source of truth.** Every action is one
   `commands.Command` entry. The menu bar, command palette, and
   cheatsheet are all derived from it on the fly — guaranteed by a
   parity test in `test/headless/menu_palette_parity_test.go`.

2. **Layout is a strict binary tree.** `PaneNode` is either `Leaf{Pane}`
   or `Split{Orientation, Ratio, A, B}`. Every operation preserves seven
   invariants asserted by `CheckInvariants`, with property tests running
   100 000 random ops with bounded tree sizes.

3. **Prefix dispatch is sticky and configurable.** `internal/prefix`
   ships four OfPreProcess views (prefix listener, resize mode, sync
   input, mouse). The registry's chord strings are rewritten in place
   on prefix change (`Registry.RebindPrefix("C-g", "C-b")`), so menus
   and palette stay consistent.

fv-go is the framework — everything fvmux draws goes through
`pkg/fv/views` and `pkg/fv/widgets`. fvmux does not vendor or fork
fv-go; the `replace` directive in `go.mod` points at `../fv-go`.

---

## Development

```sh
make build            # go build ./...
make test             # all unit tests; layout property tests run too.
make race             # everything with -race.
make lint             # go vet ./...
make install          # go install ./cmd/fvmux
make install-glue     # fvmuxa + fvmux.tmux.conf
make smoke            # print the manual smoke checklists.
```

Cross-compile sanity check:

```sh
GOOS=linux   go build ./...
GOOS=windows go build ./...
```

For a real SSH/SFTP smoke test there's a docker-compose at
[`test/smoke/docker-compose.yml`](test/smoke/docker-compose.yml).

### Working alongside fv-go

If a v1 task needs an fv-go API that doesn't exist, **stop and call
it out** — don't patch around it in fvmux. CLAUDE.md (the AI-pairing
guidance) and the implementation plan both describe this discipline.

The `replace` directive must be present during dual-repo dev and
stripped before tagging an fvmux release.

---

## Known limitations

Honest list of what's still rough in v1 alpha. None of these block
daily use but they're worth knowing:

- **Copy mode** opens the scrollback in a markdown dialog and copies
  the lot. Real selection + cursor + `/` search lands later.
- **SFTP browser** ships single-pane (remote only) with a tree, no
  live preview-on-select, no transfer queue.
- **No ControlMaster pool** — each `Ctrl-G H` / `Ctrl-G F` spawns a
  fresh ssh.
- **Dynamic menu submenus** (themes / profiles / sessions / hosts) are
  not built; the registry has `RegisterProvider` but no providers
  yet.
- **CPU sparkline / RAM bar** in the status bar — not wired; only the
  clock + window list show.
- **`keybindings.toml`** is parsed but not applied. Use the wizard or
  `[general] prefix_key` for prefix changes.
- **Themes from TOML** (user themes) not loaded; three builtins only.
- **Palette** has no `:` / `?` / `@` / `#` special prefixes, no MRU
  ring, no Ctrl-Shift-P standalone chord.
- **Splash** is ASCII; no SIXEL artwork or 5-step tour.
- **Easter eggs** are coded (`internal/whimsy`) but unwired.
- **Headless tests** cover registry parity but don't drive the
  application loop yet.

The complete audit lives in conversation history with the AI pair that
built v1 alpha; the short version is "Tier 1 done; Tier 2 + 3 + 4
mostly deferred."

---

## Roadmap (post-v1)

Per the implementation plan's "Roadmap beyond v1":

1. **Detach/reattach daemon split.** `cmd/fvmuxd` owns PTYs; `cmd/fvmux`
   becomes a thin client over a Unix socket with SCM_RIGHTS. Replaces
   the tmux glue.
2. **Plugin / scripting.** Embed `risor` or `starlark-go`; expose
   commands and event hooks via `RegisterProvider`.
3. **Mosh transport** in the connection manager.
4. **Pane recording / replay** as asciicast v2.
5. **Layout DSL** as a five-rule PEG (replaces the inline form).
6. **Theme marketplace.**

---

## Licence

See [`LICENSE`](LICENSE) (TBD; will match fv-go's licence).
