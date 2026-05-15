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

**v1 alpha — Stage 1 complete.** Stage 0 (fv-go prerequisites) and
Stage 1 (the 13-phase fvmux implementation, A through M) are shipped:
build/test/race green on macOS and Linux, cross-compiles to Windows
and Linux/arm64. Every command surface (menus, `Ctrl-G P` palette,
`Ctrl-G ?` cheatsheet) is wired from a single `commands.Registry` so
they cannot drift apart.

Detach/reattach is intentionally not built into fvmux itself — v1 leans
on an outer tmux for that, via the [`fvmuxa`](#detach--reattach-via-tmux)
wrapper. A `cmd/fvmuxd` daemon split is on the v2 roadmap.

What's solid:

- Floating windows with title-bar drag and corner-resize.
- Strict-binary-tree pane layout
  (`SplitH/V/Close/Swap/Zoom/FocusDir/BreakOut/JoinFrom`),
  property-tested with 100 000 random operations.
- Five layout presets cycled with `Ctrl-G Space`, or named directly
  via the View → Layout Preset submenu.
- Sticky resize mode (`Ctrl-G R`, then `h/j/k/l`).
- Sync-input broadcast across all panes in a window (`Ctrl-G ~`).
- TOML config + profiles + saved sessions with layout serialisation.
- Live keybinding overrides via `~/.config/fvmux/keybindings.toml` +
  Help → Reload Config (no restart).
- SSH host picker reading `~/.ssh/config` + `hosts.toml`.
- **SSH ControlMaster pool** — second `Ctrl-G H` / `Ctrl-G F` to the
  same alias reuses the master socket (no re-auth).
- **Dual-pane SFTP browser** — remote tree above local tree, shared
  preview pane (markdown / hex / image), F5/F6 transfer queue with a
  live progress strip.
- Real **copy mode**: arrows/PgUp/PgDn/Home/End to move a cell
  cursor, Space to anchor, Enter to copy the selection to the OS
  clipboard, `/` to drop into scrollback search, Esc to exit.
- First-run wizard: 3-button welcome (tour / skip / quit), 5-step
  tour, prefix picker, shell picker (parses `/etc/shells`, modern
  `pwsh` on Windows, custom path entry), live-preview theme picker.
- **Themes from disk**: drop `~/.config/fvmux/themes/<name>.toml`,
  override built-ins by name. In-app **theme editor** — saves
  trigger a live reload so palette edits land in the same frame.
- **Status bar widgets**: session + window list with activity/bell
  markers + focused pane title, **CPU sparkline (10 cells)** + **RAM
  bar (6 cells)** + clock. Bell flashes the title bar (320 ms,
  debounced).
- **Command palette** (`Ctrl-G P`) with MRU bubble-to-top, disabled
  commands shown but greyed, and synthetic top-of-list entries:
  `: run command`, `@ jump to window`, `# open session`,
  `? show cheatsheet`.
- **Log viewer** (`Ctrl-G L`) backed by a ring buffer and `-log=path`
  for file output.
- Confirm-kill before tearing down live panes / windows / app.
- Right-click pane context menu, click-to-focus.
- Eight tasteful easter eggs (see [Whimsy](#whimsy)).

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

First launch shows the wizard:

1. **Splash** with the project tagline.
2. **Welcome** — three buttons:
   - *Take the tour* → 5 step-by-step tooltips covering the menu bar,
     status line, prefix key, window-number badges, splitters.
   - *Skip* → straight to picker.
   - *Quit fvmux* → graceful exit (state is recorded).
3. **Prefix-key picker** — Ctrl-G (default) / Ctrl-B / Ctrl-A.
4. **Shell picker** — parsed from `/etc/shells` on Unix (PowerShell 7+
   first on Windows), filtered to existing executables, plus a
   "Custom path…" entry.
5. **Theme picker** — live preview on each arrow move; Enter accepts,
   Esc reverts to whatever was active before.

Choices are persisted to `~/.config/fvmux/config.toml` and applied
immediately. The triple-`Ctrl-G` chord (within 1.5 s) re-runs the
wizard from anywhere; **Help → Reset First-Run Wizard…** does the
same from the menu.

A version-bump notification fires on the first launch after the binary
version changes — quick toast in the top-right pointing at `Ctrl-G ?`.

---

## Key bindings

Every chord goes through the configured prefix. Tables below assume
the default `Ctrl-G`. Press **`Ctrl-G ?`** inside fvmux for the full,
auto-generated cheatsheet (also baked into [`assets/cheatsheet.md`](assets/cheatsheet.md)).

### File / session
| Chord | Action |
|-------|--------|
| `Ctrl-G c` | New window (default profile) |
| `Ctrl-G C` | New window from a profile |
| `Ctrl-G s` | Open saved session… |
| `Ctrl-G S` | Save current session |
| `Ctrl-G $` | Rename current session file |
| `Ctrl-G :` | Run command… (free-form `sh -c`, or `:tea` / `:rot13` / `:konami`) |
| `Ctrl-G D` | Detach from tmux (when running inside fvmuxa) |
| `Ctrl-G ,` | Rename current window (sticky user title) |

### Window navigation
| Chord | Action |
|-------|--------|
| `Ctrl-G n` / `p` | Next / previous window |
| `Ctrl-G 1`…`9`   | Focus window by number |
| `Ctrl-G Tab`     | Toggle to the last-focused window |
| `Ctrl-G w` / `f` | Window list / find window (fuzzy) |
| `Ctrl-G q`       | Flash window numbers (1.5 s overlay) |
| `Ctrl-G &`       | Kill the current window (confirms) |

### Pane layout
| Chord | Action |
|-------|--------|
| `Ctrl-G %` | Split horizontal (vertical splitter, panes side-by-side) |
| `Ctrl-G "` | Split vertical (horizontal splitter, panes stacked) |
| `Ctrl-G x` | Kill focused pane (confirms if alive) |
| `Ctrl-G z` | Zoom / unzoom focused pane |
| `Ctrl-G h j k l` | Focus pane in direction |
| `Ctrl-G o` / `;` | Next / previous pane (tree order) |
| `Ctrl-G { }` | Swap focused pane with neighbour |
| `Ctrl-G !` | Break focused pane out to a new window |
| `Ctrl-G @` | Join a single-leaf window into this one (picks orientation) |
| `Ctrl-G Space` | Cycle layout preset |
| `Ctrl-G R` | Enter resize mode (`h/j/k/l` = 1 cell, `H/J/K/L` = 5; Esc exits) |

Direct preset entries are also in **View → Layout Preset** (Even-H,
Even-V, Main-H, Main-V, Tiled).

### Edit / signals
| Chord | Action |
|-------|--------|
| `Ctrl-G [` | Enter copy mode (arrows + Space anchor + Enter copy; Esc revert) |
| `Ctrl-G ]` | Paste OS clipboard into focused pane |
| `Ctrl-G /` | Find in scrollback |
| `Ctrl-G ~` | Toggle sync-input (broadcast typing to all panes in window) |
| (menu) | Send Interrupt / Quit / EOF / SIGTERM to focused pane |

### View
| Chord | Action |
|-------|--------|
| `Ctrl-G T` | Theme picker (live preview) |
| `Ctrl-G t` | Toggle status-bar clock |
| `Ctrl-G r` | Force redraw |
| (menu) | View → Edit Themes… (in-app TOML editor; saves auto-reload) |
| (menu) | View → Toggle Status Bar / Menu Bar |

### Connections / transfer
| Chord | Action |
|-------|--------|
| `Ctrl-G H` | SSH host picker (`~/.ssh/config` + `hosts.toml`) |
| `Ctrl-G B` | Edit `hosts.toml` |
| `Ctrl-G F` | SFTP browser (dual-pane: remote + local) |
| (menu) | Connections → Active Connections… (ControlMaster pool) |
| (menu) | Connections → Reload hosts.toml |
| (menu) | Transfer → Active Transfers… / Clear Completed |

Inside the SFTP browser:
- **Tab** switches focus between remote (top) and local (bottom) trees.
- **F5 / F6** copy the focused tree's current file to the other panel's cwd.
- **Del** cancels the most recent in-flight transfer.
- **Esc** closes the browser.

### Meta
| Chord | Action |
|-------|--------|
| `Ctrl-G P` | Command palette (fuzzy over every command, MRU, prefixes) |
| `Ctrl-G ?` | Cheatsheet |
| `Ctrl-G L` | Log viewer (ring buffer; `-log=path` for file output) |
| `Ctrl-G Ctrl-G` | Send a literal Ctrl-G to the focused pane |
| (triple `Ctrl-G` within 1.5 s) | Re-run the first-run wizard |

Right-click a pane for a context menu (split / zoom / rename / send
signal / respawn / kill). Left-click a non-focused pane to focus it.

### Command palette polish

`Ctrl-G P` opens a fuzzyfinder with:

- **MRU** — recently-picked commands bubble to the top when the query
  is empty; ring of 10, persisted to `state.toml`.
- **Disabled commands** — present but prefixed `[disabled]` so users
  can see what exists but isn't applicable right now.
- **Synthetic prefix entries** (always at the top of the list):
  `: run command…`, `@ jump to window…`, `# open session…`,
  `? show cheatsheet`. Pick or fuzzy-type to dispatch.

---

## Configuration

All files live under `~/.config/fvmux/` (or `$XDG_CONFIG_HOME/fvmux/`).
Runtime state lives under `~/.local/state/fvmux/`. fvmux creates these
on first launch and seeds annotated templates.

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
theme            = "slate"
bell             = "flash"      # off | flash | notify | both
status_clock     = "15:04"
window_shadow    = true
palette_position = "center"     # center | top-left | top-center | top-right

[sftp]
parallel         = 1
```

The wizard writes `prefix_key`, `terminal.shell`, and
`appearance.theme`. Hand-edit other fields. **Help → Reload Config**
re-reads everything (including themes and keybindings) without
restart.

### `profiles.toml`

Defines what `Ctrl-G c` and `Ctrl-G C` spawn.

```toml
[[profile]]
name    = "shell"
command = ""               # empty ⇒ falls back to [terminal] shell → $SHELL → /bin/sh
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

`cwd` understands `~` and `$VARS`. Per-profile fields also include
`close_on_exit`, `window_w`, `window_h`, `scrollback_lines`.

### `keybindings.toml`

Override or unbind chords without recompiling. Use the command's
`Name` (not its menu label) to identify it.

```toml
# Rebind Ctrl-G x → Ctrl-G X.
[[binding]]
chord   = "C-g X"
command = "Kill Pane"

# Unbind a chord — empty command removes whatever currently owns it.
[[binding]]
chord   = "C-g x"
command = ""
```

Help → Reload Config picks changes up immediately. Unknown command
names are skipped silently.

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
Connections → Reload hosts.toml re-parses without restart.

### Sessions

`Ctrl-G S` writes the current windows / layout to
`~/.config/fvmux/sessions/<name>.toml` — but only when fvmux was
launched with `-session=NAME`. Restart with the same flag to restore,
or use `Ctrl-G s` (lowercase) to open the picker over every saved
session at any time.

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

---

## Themes

Three builtins ship in code: **slate**, **tokyonight-ish**,
**solarbeach**. Switch with `Ctrl-G T` — the live-preview picker
applies each highlighted theme as you arrow through, reverting on
Esc.

### User themes

Drop TOMLs into `~/.config/fvmux/themes/<name>.toml`. A starter
template is seeded at
`~/.config/fvmux/themes/example.toml.disabled` — rename to a `.toml`
extension to activate.

Every overlay field is optional; missing fields inherit fv-go's
default palette. Values are uint16 attributes (low byte = fg, high
byte = bg):

```toml
name = "indigo"
tagline = "for late-night sessions"

[palette]
frame_normal       = 0x0107
frame_active       = 0x010D
window_background  = 0x0107
splitter_bar       = 0x010B
splitter_handle    = 0x010E
desktop_background = 0x0008
status_bar_normal  = 0x0F01
menu_bar_hot       = 0x0E01
```

A user theme whose `name` matches a builtin overrides the builtin
(so a custom `slate.toml` replaces the shipped one).

### Editing in-app

**View → Edit Themes…** opens a fuzzyfinder over every TOML in the
themes dir plus a `+ Create new theme…` entry. Picking a theme opens
it in the embedded TOML editor; saving triggers a theme reload and
re-applies the active palette so edits land in the same frame.

Creating a new theme prompts for a name (`[A-Za-z0-9_-]+`), copies
the example template body in with the `name = "example"` line
patched to the chosen name, then drops you in the editor.

---

## SSH and SFTP

### Connections

**`Ctrl-G H`** opens a fuzzy picker over `~/.ssh/config` Host entries
merged with `hosts.toml`. Pick one → fvmux warms a ControlMaster
(`ssh -M -N -o ControlPersist=600`) for that alias, then spawns the
interactive session through it. Subsequent connects to the same alias
skip re-authentication.

**ControlMaster pool** state is visible via Connections → Active
Connections… (alias, ref count, uptime). Pool cleanup runs on fvmux
exit.

### SFTP

**`Ctrl-G F`** opens a four-pane browser:

- **Upper-left tree**: remote folders, rooted at the SSH session's
  cwd. Folders only — files are not shown in the tree.
- **Upper-left listing**: remote *current folder*'s contents (`../`,
  folders, files) with `Name | Size | Mtime` columns.
- **Lower-left tree**: local folders, rooted at `$HOME`. Folders
  only.
- **Lower-left listing**: local *current folder*'s contents.
- **Right**: preview pane — markdown / text / hex / image
  (PNG/JPG/GIF decoded full; SIXEL when the host terminal supports
  it, half-block otherwise). Driven by whichever listing just
  highlighted a file.
- **Bottom**: TaskProgress strip with one row per active or recent
  transfer (caption + spinner + bar + percent + ETA).

Navigation:

- **Tree highlight** changes that side's *current folder*; the
  listing on the right of the tree rebuilds to show its contents.
- **Listing highlight** of a file → preview updates.
- **Enter** on a folder in a listing → that side's current folder
  changes to that subfolder.
- **Enter** on `../` → up one folder on that side.
- **Enter** on a file in a listing → no-op (preview is already current).

Key map inside the browser:

- **Tab** — cycle focus across the four panes (remote tree → remote
  listing → local tree → local listing → …).
- **F5 / F6** — copy the file highlighted in the focused listing to
  the other side's current folder. Direction is derived from which
  side has focus.
- **Del** — cancel the most recent in-flight transfer.
- **Esc / Close** — dismiss the browser.

Transfers run as goroutines updating an atomic byte counter; the
TaskProgress widget is rebuilt from a snapshot every 200 ms by the
anim loop, so the renderer and the goroutine never share mutable
widget state. The SFTP session itself piggy-backs on the alias's
ControlMaster (`ssh -S socket -s alias sftp`) so opening the browser
to a host you're already connected to skips auth.

The classification rules live in
[`internal/sftp/binary.go`](internal/sftp/binary.go) — extension
whitelist, then magic-byte sniff (ELF / Mach-O / ZIP / PDF) and
printable-ASCII ratio.

---

## Logs

A ring buffer of the last 4 096 slog entries is always live in
memory; press **`Ctrl-G L`** to open the viewer (search via `/`,
scroll with arrows, Esc to close).

The optional `-log=path` flag writes the same entries to a file
(`-log=fvmux.log`, `-log=/var/log/fvmux.log`).

Two diagnostic hooks from fv-go land here automatically:
- Backend errors (Flush write failures) → `slog.Warn("backend error", ...)`.
- Event-queue overflows → `slog.Warn("event dropped", ...)`.
- Main-loop panics → `slog.Error("panic in main loop", ...)`.

---

## Whimsy

A small list of deliberate easter eggs, each chosen to be polite
(non-disruptive, single-firing or short-lived).

- A window literally titled `home` gets a `🏠` glyph prepended in the
  status-line window list.
- Opening a new window on a Friday at or after 17:00 local flashes
  "ship it" in the focused-pane slot of the status bar for 4 s.
- The cheatsheet footer carries a daily-rotating tagline.
- Triple-`Ctrl-G` within 1.5 s replays the first-run wizard.
- `Ctrl-G :` then `:tea` schedules a notification after 180 s
  (cancellable by closing the notification or quitting).
- `Ctrl-G :` then `:rot13` runs the focused pane's terminal output
  through a rot13 filter for 10 seconds. Output-path only.
- `Ctrl-G :` then `:konami` flips the CPU sparkline upside-down for
  the session; run again to restore.
- The cheatsheet itself is regenerated from the live registry on
  every `Ctrl-G ?` open — surface drift is impossible.

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
├── menus/                 Menu bar built from the registry + dynamic submenus.
├── palette/               Ctrl-G P fuzzy picker with MRU + special entries.
├── statusbar/             Bottom-row composition incl. CPU/RAM widgets.
├── theme/                 Theme registry + builtins + LoadDir + live-preview picker.
├── splash/                First-run wizard (welcome + tour + prefix + shell + theme).
├── cheatsheet/            Markdown reference generated from the registry.
├── copymode/              Keyboard-driven cell selection + clipboard copy.
├── clipboard/             atotto wrapper.
├── sshmgr/                Host loader + picker + ControlMaster pool.
├── sftp/                  Client, dual-pane browser, preview, transfer manager.
├── logs/                  slog file sink + ring buffer for the log viewer.
├── sysmon/                CPU + RAM sampling (gopsutil) for the status bar.
├── whimsy/                Easter-egg predicates and filters.
└── keys/                  Chord parser (used for keybindings.toml + display).

assets/                    SIXEL splash + baked cheatsheet (go generate).
scripts/                   fvmuxa wrapper + private tmux config.
test/headless/             Unit + parity tests + golden snapshots.
test/smoke/                Manual checklists + docker-compose for SFTP testing.
.github/workflows/         CI (build/test/race/cross) + release (5 OS/arch combos).
```

Three load-bearing patterns:

1. **The registry is the single source of truth.** Every action is one
   `commands.Command` entry. The menu bar, command palette, and
   cheatsheet are all derived from it on the fly — guaranteed by a
   parity test in `test/headless/menu_palette_parity_test.go`.

2. **Layout is a strict binary tree.** `PaneNode` is either
   `Leaf{Pane}` or `Split{Orientation, Ratio, A, B}`. Every operation
   preserves seven invariants asserted by `CheckInvariants`, with
   property tests running 100 000 random ops with bounded tree sizes.

3. **Prefix dispatch is sticky and configurable.** `internal/prefix`
   ships four OfPreProcess views (prefix listener, resize mode, sync
   input, mouse). The registry's chord strings are rewritten in place
   on prefix change (`Registry.RebindPrefix("C-g", "C-b")`), so menus
   and palette stay consistent.

Dynamic submenus (Profiles, Sessions, Active Connections, Active
Transfers) use a per-rebuild dispatch table over Cm codes
`0x8000..0x8FFF` — fresh closures every menu rebuild, no stale
references. Themes deliberately bypass the dynamic submenu path so
View → Theme Picker (Ctrl-G T) is the single source for theme
selection.

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

go generate ./internal/cheatsheet      # regenerate assets/cheatsheet.md
go test ./test/headless/ -update=true  # rewrite golden snapshots
```

Cross-compile sanity check:

```sh
GOOS=linux   go build ./...
GOOS=windows go build ./...
```

For a real SSH/SFTP smoke test there's a docker-compose at
[`test/smoke/docker-compose.yml`](test/smoke/docker-compose.yml).

### CI

`.github/workflows/ci.yml` runs build + test + race + cross-compile on
PR and main. `.github/workflows/release.yml` triggers on tag push,
strips the local `replace github.com/oldwired/fv-go => ../fv-go` from
`go.mod`, runs `go mod tidy`, and builds binaries for
linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
to attach to the GitHub release.

### Working alongside fv-go

If an fvmux task needs an fv-go API that doesn't exist, **stop and
call it out** — don't patch around it in fvmux. CLAUDE.md (the
AI-pairing guidance) and the implementation plan both describe this
discipline.

The `replace` directive must be present during dual-repo dev. CI's
release workflow strips it automatically before publishing release
binaries.

---

## Known limitations

What's still rough in v1 alpha — none block daily use:

- **Move semantics** in the SFTP browser. F5/F6 always copy; there
  is no rename / move primitive yet. Directory copy isn't supported
  either (file-at-a-time only).
- **Image preview** uses Go's `image.Decode` with PNG/JPG/GIF support
  via blank imports. WebP / TIFF / BMP would need additional
  decoders; for now they fall back to hex.
- **Themes from TOML** support a curated subset of `fvtheme.Palette`
  fields (frame / window / splitter / desktop / status / menu).
  Adding more is one edit in `internal/theme/load.go`.
- **Ctrl-Shift-P** as a standalone palette chord isn't wired — would
  need a non-prefix dispatch path. `Ctrl-G P` is the only way in.

The list above is the complete remaining v1 backlog after Phases
A–M. Anything not listed is shipped.

---

## Roadmap (post-v1)

Per the implementation plan's "Roadmap beyond v1":

1. **Detach/reattach daemon split.** `cmd/fvmuxd` owns PTYs;
   `cmd/fvmux` becomes a thin client over a Unix socket with
   SCM_RIGHTS. Replaces the tmux glue.
2. **Plugin / scripting.** Embed `risor` or `starlark-go`; expose
   commands and event hooks.
3. **Mosh transport** in the connection manager.
4. **Pane recording / replay** as asciicast v2.
5. **Layout DSL** as a five-rule PEG (replaces the inline form).
6. **Theme marketplace.**

---

## Licence

fvmux is released under the [MIT licence](LICENSE).

It depends on [fv-go][fvgo], which is licensed
`LGPL-2.1-or-later WITH FPC-modified-LGPL-exception` — the FPC
exception explicitly permits linking under different terms, so binaries
built from this repository can be redistributed under MIT while the
fv-go portion remains under its own licence.
