# fvmux — Implementation Plan

## Context

`fvmux` is a new terminal multiplexer built on the `fv-go` framework. Where tmux gives you tiled panes in one rectangle, fvmux gives you **draggable floating windows that each contain a split tree of panes** — a multiplexer with a window manager. v1 ships as a regular foreground process intended to be run inside an outer tmux session (so detach/reattach is deferred), but it includes a proper session/profile model, an SSH connection manager that drives the system `ssh` client, and a full SFTP browser with hex preview for binaries. The point of v1 is to be the multiplexer the user would actually adopt for daily work, not a tech demo.

It lives in a **new separate repo** (`github.com/oldwired/fvmux`) and depends on `github.com/oldwired/fv-go` via go.mod, using a local `replace` directive during dual-repo development.

## Architectural anchors (decided)

| Decision | Choice |
|---|---|
| Layout | Floating windows containing nested split trees |
| Prefix key | `Ctrl-G` (configurable) |
| Sessions / profiles | Both: profiles = read-only spawn templates; sessions = mutable live state |
| Whimsy | Tasteful by default; playful first-run splash; itemized easter eggs |
| Repo location | Separate (`github.com/oldwired/fvmux`) |
| Config format | TOML under `~/.config/fvmux/` (XDG) |
| Status line | Tmux baseline (session + window list) + clock + CPU sparkline + RAM bar |
| v1 features | Multiplexer core + SSH connection manager + SFTP browser + HexEdit for binaries |

## Repo skeleton

```
github.com/oldwired/fvmux
├── go.mod                          # requires github.com/oldwired/fv-go
├── Makefile                        # build, test, lint, race, install
├── cmd/fvmux/
│   ├── main.go                     # wires Application, Desktop, prefix dispatcher, status bar
│   ├── flags.go                    # -config, -session, -profile, -no-splash, -log
│   └── version.go                  # build-time version info
├── internal/
│   ├── app/                        # OnCommand chain, signals, command IDs
│   ├── prefix/                     # OfPreProcess prefix-key listener + overlay
│   ├── session/                    # Session, WindowState, Pane; lifecycle, persistence
│   ├── layout/                     # PaneNode tree, algebra, focus dir, render, tests
│   ├── config/                     # TOML schemas, XDG paths, defaults, roundtrip tests
│   ├── keys/                       # binding parser, default chord table
│   ├── profile/                    # Profile struct, instantiation
│   ├── sshmgr/                     # ssh_config parser, hosts.toml merger, pool, spawn, UI
│   ├── sftp/                       # client, two-pane browser, transfer queue, preview, binary sniff
│   ├── statusbar/                  # left/middle/right composition over menus.StatusLine
│   ├── theme/                      # Theme struct, builtin themes, palette application
│   ├── splash/                     # first-run splash, welcome dialog, prefix picker
│   ├── cheatsheet/                 # MarkdownView wrapper for assets/cheatsheet.md
│   └── logs/                       # file-backed slog + ring buffer for LogViewer
├── assets/
│   ├── cheatsheet.md               # embedded via go:embed; shown by Ctrl-G ?
│   ├── splash.six                  # SIXEL logo
│   ├── splash.txt                  # ASCII fallback
│   └── themes/{slate,tokyonight-ish,solarbeach}.toml
├── docs/{design,keys,config,development}.md
└── test/{headless,smoke}/
```

## fv-go module boundary

fvmux imports only `pkg/fv/...` and treats it as a stable API. Used surface: `app.Application`/`Desktop`, `views.{Window, SplitGroup, Group, View, Base}`, `widgets/{terminal, fuzzyfinder, popupmenu, treeview, markdown, taskprogress, hexedit, imageview, cpucore, ramview, notification}`, `menus.{MenuBar, StatusLine}`, `dialogs.*`, `consts.OfPreProcess`, `geom.Rect`, `drivers.Event`, `sixel`.

## Stage 0 — fv-go upstream prerequisites

**Status (verified 2026-05-13): Stage 0 complete. Every blocker, important, and v2-deferred item is merged in fv-go.** Detailed verification follows each item. **fvmux Stage 1 work can begin immediately against the current fv-go tree.**

The gap audit found that fvmux needs more from fv-go than initially specified. The implementation **starts in `fv-go`** with the PR train below, tags a release, then bootstraps `fvmux` against the tag. Each PR is small, self-contained, and either adds a new field/method or wraps an existing internal mechanism — no behaviour change at default settings.

### Blocker PRs (v1 cannot ship without these)

These touch real correctness or UX table-stakes — without them, fvmux is either broken or unusably crippled.

**Group A — Terminal lifecycle & I/O** (can land as one PR or three small ones)

- **A1 ✅ done** — `Terminal.ScrollbackLines int` field at `terminal.go:33`, honoured in `Start()` at line 185.
- **A2 ✅ done** — `OnActivity func()` (`terminal.go:69`, debounced ≥500ms in the reader loop at line 338) and `OnBell func()` (`terminal.go:74`; parser-side at `vt.go:706,740`).
- **A3 ✅ done** — `pty_unix.go:38` sets `SysProcAttr{Setsid: true, Setctty: true}`. `Close()` at `pty_unix.go:57` calls `syscall.Kill(-pid, SIGHUP)` (process-group, the negative pid) before `Process.Kill()`. Belt-and-braces.

**Group B — Shell-correctness terminal features**

- **B1 ✅ done** — `OnCWDChange func(string)` at `terminal.go:63`, OSC 7 parsed and dispatched at `terminal.go:159`.
- **B2 ✅ done** — `BracketedPaste() bool` at `terminal.go:242`, `Paste(text string) error` at `terminal.go:252`. (User added an `error` return where I'd specified none — better; surfaces PTY write errors.)

**Group C — Layout & status-line plumbing**

- **C1 ✅ done** — `GetRatio() float64` at `splitter.go:208`, `SetRatio(r float64)` at `splitter.go:222`. `MinPanel1`, `MinPanel2` are exported fields (`splitter.go:31`); no setter needed since callers can mutate them directly.
- **C2 ✅ done** — `LeftItems []*StatusItem` and `RightItems []*StatusItem` on `StatusLineDefs` (`statusline.go:32-33`); rendering at line 90 (left) and 95 (right-justified). Legacy `Items` slot preserved.

**Group D — Theming**

- **D1 ✅ done** — new `pkg/fv/theme/theme.go` (~480 lines) defines `Palette` struct (line 25), `Default` palette (line 269), `Get()` (line 480), and `Set(*Palette)` (line 489). Frame/menubox use it (`menubox.go:243` reads `pal := theme.Get()` per draw).

### Important PRs (UX degraded without; should ride the same release)

- **E1 ✅ done** — `ScrollbackText() string` at `terminal.go:275`.
- **E2 ✅ done** — `SuspendMouseForwarding(v bool)` at `terminal.go:233`.
- **E3 ✅ done** — `SetFocused(v bool)` at `terminal.go:218`.
- **E4 ✅ done** — `OnMove func(geom.Point)` and `OnResize func(geom.Point)` on `Window` (`window.go:161,165`); fired from the resize/move loops at `window.go:397` and `window.go:447`.
- **E5 ✅ done** — `SetNumber(n int)` at `window.go:287`.
- **E6 ✅ done** — `OnQuitRequest func() (proceed bool)` at `app.go:49`; consulted via `acceptQuit()` at `app.go:335-342`.
- **E7 ✅ done — and better than specified.** Instead of hardcoding Terminal cleanup, the implementation introduced a generic `Stoppable` interface (`app.go:64`) that any view can satisfy. `Done()` walks the tree via `walkStop` (`app.go:583`) calling `Stop()` post-order on every Stoppable descendant. There's also a `recover()`-driven cleanup path (`app.go:270-279`) that fires even if `Done()` was never called — covers the panic case I'd listed as a separately-deferred PR. This is the better design.
- **E8 ✅ done** — `Shortcut string` field at `menu.go:30`; menu-width measurement at `menubox.go:62` (`utf8.StringDisplayWidth(it.Shortcut)`); right-aligned render at `menubox.go:297-309` with separate `pal.MenuBoxShortcut` / `pal.MenuBoxShortcutSelected` palette roles for unselected/selected rows.

### Originally-deferred items (now also merged — verified 2026-05-13)

These were originally tagged "defer to v2 with workarounds in v1." All landed early; fvmux can use them in Stage 1 directly.

- `app.Application: OnPanic` ✅ done — `OnPanic func(recovered any)` at `app.go:56`, called from the recover() block at `app.go:273-276`. fvmux can post a `Notification` toast or write a crash log here.
- `app.Application: PostEvent` ✅ done — `PostEvent(ev drivers.Event)` at `app.go:349`. fvmux's status-bar ticker can post a synthetic redraw event instead of relying on `MarkDirty` race-windows.
- `widgets/terminal: SetEnv / SetWorkingDir` ✅ done — `SetEnv(env)` at `terminal.go:209`, `SetWorkingDir(dir)` at `terminal.go:213`, with exported `Env` / `WorkingDir` fields. Useful for profile-driven late-bind.
- **Headless test backend** ✅ done — `pkg/fv/term/headless.go` + `headless_test.go` ship a fake Backend implementation. fvmux's `test/headless/` can drive the full `app.Application` event loop without a real terminal — golden-frame integration tests are now feasible in v1.

### Decision: OS clipboard `Get`

fv-go's `pkg/fv/clipboard` is about OSC 52 cross-terminal *set*; OSC 52 *get* is async and unreliable. OS-level clipboard read (X11 selection, macOS pasteboard, Win32 clipboard) is a different concern with non-trivial deps. **Decision: fvmux integrates `atotto/clipboard` (or `golang.design/x/clipboard`) directly inside `internal/clipboard`, not as an fv-go addition.** Keeps fv-go's dependency surface minimal.

### Critical-files map for the fv-go work

| PR | Files touched |
|---|---|
| A1, A2 | `pkg/fv/widgets/terminal/vt.go`, `pkg/fv/widgets/terminal/terminal.go` |
| A3 | `pkg/fv/widgets/terminal/pty_unix.go`, `pkg/fv/widgets/terminal/pty_windows.go`, `pkg/fv/widgets/terminal/terminal.go` |
| B1, B2 | `pkg/fv/widgets/terminal/vt.go`, `pkg/fv/widgets/terminal/terminal.go` |
| C1 | `pkg/fv/views/splitter.go` |
| C2 | `pkg/fv/menus/statusline.go` |
| D1 | new `pkg/fv/theme/` package; touches `pkg/fv/views/{frame,window,scroll}.go`, `pkg/fv/menus/{menubar,statusline}.go`, and a handful of widgets that hardcode attrs |
| E1–E3 | `pkg/fv/widgets/terminal/{vt,terminal}.go` |
| E4, E5 | `pkg/fv/views/window.go` |
| E6, E7 | `pkg/fv/app/app.go` |
| E8 | `pkg/fv/menus/menu.go`, `pkg/fv/menus/menubox.go` |

### Sequencing — complete

All Stage 0 PRs have landed. **Next step: tag the fv-go release (e.g. `v0.2.0`) and bootstrap `github.com/oldwired/fvmux` against it**, with a `replace github.com/oldwired/fv-go => ../fv-go` line in `fvmux/go.mod` for local dual-repo development. CI guard checks for the `replace` directive before tagging fvmux releases.

Stage 1 fvmux work proceeds per the **Implementation order** section near the bottom of this plan.

## Data model

### Core Go types

```go
// internal/session
type Pane struct {
    ID       PaneID
    Term     *terminal.Terminal
    Title    string             // profile-derived fallback title
    ShellTitle string           // latest OSC-set title
    UserTitle  string           // optional sticky pane rename
    CWD      string             // updated from OSC7 when seen
    Profile  string             // empty for ad-hoc
    Dead     bool
    ExitErr  error
    Activity time.Time          // last OnActivity tick
    Bell     bool
}

type WindowState struct {
    ID      WindowID
    Number  int                 // 1..9 maps to views.Window number badge; 0 = unnumbered
    Title   string              // user-renameable; defaults to focused pane title
    Root    *PaneNode           // layout tree (see § Layout algebra)
    Zoomed  *PaneID             // non-nil while a pane is zoomed full-window
    Frame   *views.Window
    Pos     geom.Rect           // last user-placed bounds (restored on session load)
}

type Session struct {
    Name      string
    Windows   []*WindowState
    ActiveIdx int
    Created   time.Time
    MetaPath  string             // sessions/<name>.toml
}

// internal/profile
type Profile struct {
    Name    string
    Command string
    Args    []string
    Env     map[string]string
    CWD     string                // supports ~ and $VARS
    Title   string                // initial pane title before OSC overrides
    Layout  string                // optional pre-split spec ("v:h,h")
}
```

### TOML schemas

**`~/.config/fvmux/config.toml`**

```toml
[general]
prefix_key       = "C-g"
default_profile  = "shell"
confirm_kill     = true
splash_enabled   = true

[terminal]
scrollback_lines = 10000
shell            = "/bin/zsh"

[appearance]
theme            = "slate"
bell             = "flash"          # off|flash|notify|both
status_clock     = "15:04"
window_shadow    = true
```

**`~/.config/fvmux/profiles.toml`**

```toml
[[profile]]
name = "shell"
command = "/bin/zsh"
args = ["-l"]
cwd = "~"

[[profile]]
name = "ssh-prod"
command = "ssh"
args = ["prod-1"]
title = "prod-1"
```

**`~/.config/fvmux/keybindings.toml`** — overrides only; defaults from `internal/keys/bindings.go`. Empty `command = ""` removes a default binding.

**`~/.config/fvmux/sessions/<name>.toml`** — fvmux-managed session state with embedded layout string.

**`~/.config/fvmux/hosts.toml`** — optional; merged with `~/.ssh/config`. Entries carry `alias`, `user`, `host`, `port`, `notes`, `tags[]`.

**`~/.config/fvmux/state.toml`** — managed: `first_run_done`, `last_session`, `last_version`, `welcome_shown_at`.

## Layout algebra

A `PaneNode` is either a `Leaf` (one `Pane`) or a `Split` (orientation + ratio + two children). Strict binary tree.

```go
type PaneNode struct {
    Kind        NodeKind                  // NodeLeaf | NodeSplit
    Pane        *Pane                     // set iff NodeLeaf
    Orientation views.SplitOrientation
    Ratio       float64                   // 0.0..1.0 of parent's primary axis
    A, B        *PaneNode                 // set iff NodeSplit
    Parent      *PaneNode
}
```

### Operations

- **`SplitH(target, newPane)`** — replace `target` (a leaf) with `Split{Vertical, 0.5, target, Leaf(newPane)}`. SplitH = vertical splitter, panes side-by-side (tmux convention). Materialised via `views.NewSplitGroup(bounds, SplitVertical, splitPos)`.
- **`SplitV(target, newPane)`** — orientation `SplitHorizontal`, panes stacked.
- **`Close(target)`** — root-leaf closes the window (confirm if alive). Otherwise replace `target.Parent` with `target`'s sibling and rewire grandparent (collapse single-child rule).
- **`Swap(a, b)`** — exchange `Pane` pointers; tree shape unchanged.
- **`Zoom(target)`** — set `WindowState.Zoomed = &target.Pane.ID`. Renderer fills full window bounds with that pane, ignoring siblings. Second invocation unzooms.
- **`FocusDir(current, dir)`** — geometric nearest: convert each leaf's last-rendered rect to a centre point; pick the nearest leaf whose centre lies in the requested half-plane, tie-broken by Manhattan distance. (Geometric, not tree-walk — matches user intuition across nested splits.)
- **`BreakOut(target)`** — detach from current window (Close semantics on source), create a new `WindowState` with `Root = Leaf(target.Pane)`, `desktop.InsertWindow`.
- **`JoinFrom(srcWindow, dstTarget, orientation)`** — inverse: v1 refuses if source root is non-leaf; future enhancement merges subtrees.

### Invariants (asserted by `CheckInvariants`)

1. Every `Split` has exactly two non-nil children.
2. No `Leaf` has a nil `Pane`.
3. `Parent` pointers consistent (`n.A.Parent == n && n.B.Parent == n`).
4. Each `Pane.ID` appears once across the session.
5. `0 < Ratio < 1`; renderer additionally clamps to `MinPanel1/MinPanel2`.
6. After `Close`, no `Split` with a single non-nil child.
7. `Zoomed` references a leaf still in `Root`.

Property tests build random trees with seeded op sequences and assert invariants after each step.

## Command registry — single source of truth

Every fvmux action is registered in one `internal/commands.Registry`. The registry feeds three surfaces simultaneously: the **menu bar**, the **command palette**, and the **cheatsheet** (auto-generated markdown). This guarantees menus, palette, and docs cannot drift apart.

```go
// internal/commands
type Command struct {
    ID         uint16            // fv-go OnCommand dispatch ID (≥ 1000)
    Category   string            // "Pane", "Window", "Edit", "View", … (mirrors menu names)
    Name       string            // "Split Left/Right" (clean, palette-friendly)
    Aliases    []string          // legacy keybindings.toml command names
    MenuLabel  string            // "Split Left/Ri~g~ht" (with Borland-style hotkey markers)
    Chord      string            // "C-g %"  ("" for unbound)
    Hidden     bool              // omit from menus + palette but keep ID stable (e.g. send-literal-prefix)
    Action     func(ctx *Ctx)    // direct invocation path (palette + chord call this)
    Enabled    func(ctx *Ctx) bool   // nil ⇒ always enabled
    DisabledReason string        // concise status feedback when unavailable
}

type Registry struct {
    cmds map[uint16]*Command
    byCategory map[string][]*Command
    byChord map[string]*Command
}

// All() returns commands grouped for the palette; ByCategory(cat) builds menus;
// ByID(id) drives the OnCommand chain; LookupChord(chord) drives the prefix dispatcher.
```

Adding a new command is one entry in `internal/commands/defaults.go`. Removing or rebinding it (via `keybindings.toml`) updates all three surfaces.

## Menu bar

Eight top-level menus, classic-app shape. Hotkeys via `~X~` markers. Right-aligned shortcut column on submenu items via `\t` separator in the label (verify support; see fv-go gap note below).

```
~F~ile   ~E~dit   ~V~iew   ~P~ane   ~W~indow   ~C~onnections   ~T~ransfer   ~H~elp
```

### ~F~ile
- ~N~ew Window                              Ctrl-G c
- New Window from ~P~rofile…               Ctrl-G C
- ────
- ~O~pen Session…                           Ctrl-G s
- ~S~ave Session                             Ctrl-G S
- Save Session ~A~s…
- Rename Session…                           Ctrl-G $
- ────
- Open ~C~onfig
- Open ~K~eybindings
- Open ~H~osts                              Ctrl-G B
- ~R~eload Config
- ────
- ~Q~uit fvmux                              (confirms if panes alive)

### ~E~dit
- Enter ~C~opy Mode                         Ctrl-G [
- ~P~aste                                    Ctrl-G ]
- ────
- ~F~ind in Scrollback                       Ctrl-G /
- ────
- Toggle ~S~ync-Input (broadcast)           Ctrl-G ~
- Send Si~g~nal ▸
  - Send ~I~nterrupt (SIGINT)
  - Send ~T~erminate (SIGTERM)
  - Send ~Q~uit (SIGQUIT)
  - Send ~E~OF (Ctrl-D)
  - Send ~L~iteral Ctrl-G                    Ctrl-G Ctrl-G

### ~V~iew
- ~Z~oom Focused Pane                        Ctrl-G z
- Flash Pane ~N~umbers                       Ctrl-G q
- ────
- Cycle ~L~ayout                             Ctrl-G Space
- Layout ~P~reset ▸
  - Even ~H~orizontal
  - Even ~V~ertical
  - Main H~o~rizontal
  - Main Ve~r~tical
  - Tiled
- ────
- Toggle ~M~enu Bar
- Toggle ~C~lock                            Ctrl-G t
- Toggle ~S~tatus Bar
- ────
- ~T~heme ▸                                 Ctrl-G T
  - Slate (default)
  - Tokyonight-ish
  - Solarbeach
  - Reload Themes from Disk
- ────
- ~R~edraw                                   Ctrl-G r

### ~P~ane
- Split ~H~orizontal                         Ctrl-G %
- Split ~V~ertical                           Ctrl-G "
- ────
- Focus ~L~eft                               Ctrl-G h
- Focus ~D~own                               Ctrl-G j
- Focus ~U~p                                 Ctrl-G k
- Focus ~R~ight                              Ctrl-G l
- Focus ~N~ext                               Ctrl-G o
- Focus ~P~revious                           Ctrl-G ;
- ────
- Swap with ~N~ext                           Ctrl-G }
- Swap with Pre~v~                           Ctrl-G {
- ────
- ~B~reak Out to Window                      Ctrl-G !
- ~J~oin from Window…                        Ctrl-G @
- ────
- Enter ~R~esize Mode                        Ctrl-G R
- ────
- ~K~ill Pane                                Ctrl-G x
- Re~s~pawn Dead Pane

### ~W~indow
- ~N~ew Window                              Ctrl-G c
- ~R~ename Window…                           Ctrl-G ,
- ────
- N~e~xt Window                              Ctrl-G n
- ~P~revious Window                          Ctrl-G p
- ~L~ast Window (MRU)                        Ctrl-G Tab
- ────
- ~F~ind Window…                             Ctrl-G f
- Window ~L~ist…                             Ctrl-G w
- ────
- Focus Window 1–9                           Ctrl-G 0..9
- ────
- ~K~ill Window                              Ctrl-G &

### ~C~onnections
- ~C~onnect to Host…                         Ctrl-G H
- ────
- Active Connections…
- ────
- ~E~dit hosts.toml                          Ctrl-G B
- ~R~eload Hosts

### ~T~ransfer
- ~F~ile Browser (SFTP)…                     Ctrl-G F
- ────
- ~U~pload File…
- ~D~ownload File…
- ────
- Active ~T~ransfers…
- ────
- Clear Completed

### ~H~elp
- Command ~P~alette…                         Ctrl-G P  (also Ctrl-Shift-P)
- ~C~heatsheet                              Ctrl-G ?
- ~K~eybindings Reference
- ────
- ~L~og Viewer                               Ctrl-G ~
- ────
- ~A~bout fvmux

**Notes on the menu surface**

- All menu items are commands from the registry; menus are built in `internal/menus/build.go` by walking `registry.ByCategory(...)`.
- Commands flagged `Hidden:true` (e.g., "Send Literal Ctrl-G") have a chord but no menu entry — they still appear in the palette unless you also `--hide-from-palette` them.
- Items disabled by `Enabled(ctx) == false` render greyed out (e.g., "Send SIGINT" is disabled when the focused pane is dead).
- Dynamic submenus (Theme list, Profile list, Active Connections, Active Transfers) are rebuilt each time the menu opens.

## Command palette (Ctrl-G P)

Fuzzy-searchable list over every command in the registry — fvmux's answer to VS Code's Cmd-Shift-P. Bound to `Ctrl-G P` (and the standalone chord `Ctrl-Shift-P` since it costs nothing).

**Flow:**

1. Press `Ctrl-G P`. fvmux opens a centred modal: an `InputLine` on top and a scrollable result list below. (Built from `widgets/fuzzyfinder`, which already provides the score-by-character-subsequence behaviour; we wrap it to render two-column rows.)
2. Each row renders as:
   ```
   Pane    Split Left/Right              Ctrl-G %
   Pane    Focus Left                    Ctrl-G h
   File    Open Session…                 Ctrl-G s
   View    Theme: Tokyonight-ish
   Edit    Send Interrupt (SIGINT)
   ```
   Three columns: category (dim), name (normal), chord (right-aligned, dim). Typing filters; Up/Down navigates; Enter executes; Esc cancels.
3. Filter target: a single string `"<category> <name> <chord>"` joined with spaces — so users can type `pane left`, `split horiz`, or `Ctrl-G %` and find the right entry.
4. Recently-used commands float to the top of the unfiltered list (small MRU ring stored in `state.toml`).
5. Disabled commands appear greyed and are skipped on Enter (a status-bar flash explains why: e.g., "Pane is dead; respawn first").
6. **Dynamic entries**: themes, profiles, sessions, hosts, and active transfers each contribute synthetic palette entries through plugin-style providers (`registry.RegisterProvider(func(ctx) []*Command)`), so `Ctrl-G P` "tokyonight" lands you on the theme switcher.
7. Special prefix syntax in the input:
   - `>` (default, can be omitted) — commands.
   - `:` — execute a command-prompt string (mirrors the `Ctrl-G :` flow).
   - `?` — search this category's submenu (e.g., `? pane` lists pane-only commands).
   - `@` — jump to window by title.
   - `#` — jump to session.

**Implementation notes:**

- File: `internal/palette/palette.go`. Roughly 200 lines.
- Reuses `widgets/fuzzyfinder.New(bounds, items)` for the scoring core; we customise the row renderer via a thin wrapper (`PaletteRow{Cmd *Command}` that implements a `Label()` and a `Score(query)` indirection).
- Palette is opened as a modal `views.Window` via `Desktop.ExecView`, not a floating fvmux window, so it doesn't pollute the window list.

## Keybindings (v1)

Generated from the registry; every command above also appears here. All chords are `Ctrl-G <key>` unless marked **(mode)** for sticky resize/copy modes. The asterisked bindings are **always-on** (no prefix needed) because their cost is negligible and the UX win is large.

**Sessions** — `s` picker · `S` save · `$` rename · `D` detach-not-yet · `:` command prompt
**Windows** — `c` new · `C` from profile · `n`/`p` next/prev · `0`–`9` focus by number · `w` window list · `,` rename · `&` kill · `f` find · `Tab` last-focused toggle
**Panes** — `%` split-h · `"` split-v · `h j k l` / arrows focus · `o` next pane · `;` last pane · `x` kill · `!` break-out · `@` join from · `z` zoom · `q` flash numbers · `{` `}` swap · `Space` cycle preset layouts · `r` redraw · `~` toggle sync-input
**Resize mode (R)** — `H J K L` resize 1 cell, `Shift-HJKL` 5 cells, `Esc` exits
**Copy mode (`[`)** — arrows/PgUp/PgDn scroll · `/` search · `Space` start selection · `Enter` copy · `Esc`/`q` exit
**Meta** — `?` cheatsheet · **`P` command palette** (and standalone `Ctrl-Shift-P`*) · `m` menu bar focus · `t` toggle clock · `~` log viewer · `Ctrl-G` (double) send literal `Ctrl-G`
**Connections & files** — `H` host picker → SSH · `F` SFTP browser · `B` edit hosts.toml
**Themes** — `T` theme picker
**Easter egg** — `Ctrl-G Ctrl-G Ctrl-G` within 1.5s → splash replay

~50 bindings total, all defined in `internal/commands/defaults.go` and surfaced through `Ctrl-G P` for discoverability.

## Status line composition

`menus.StatusLine` takes a flat `[]*StatusItem`. `internal/statusbar` layers left/middle/right composition above it:

```go
type Section interface { Render(width int) string; Width() int }
type Bar struct {
    Left, Middle, Right []Section
    line *menus.StatusLine     // one synthetic StatusItem owns the full string
}
```

A 1s ticker (`time.AfterFunc` posting a custom redraw event) recomputes sections, sets the single item's `Text`, calls `views.MarkDirty()`. Layout: render Left joined by `" │ "`, measure; render Right joined by `" │ "`, measure; middle gets `total − leftW − rightW` padded with spaces and centred.

**Default sections:**

- **Left** — `SessionName` (`[work]`), `WindowList` (`1:edit* 2:logs- 3:db`; `*` focused, `-` activity, `!` bell). Walks `session.Windows`, reads `Pane.Activity`/`Bell`.
- **Middle** — focused pane title + ` ▸ ` + `cwd` basename.
- **Right** — CPU sparkline (`widgets/cpucore.Sparkline`, 10 cells), RAM bar (`widgets/ramview`, 6 cells), clock (`time.Now().Format(cfg.Appearance.StatusClock)`).

While the prefix is armed, `prefix/overlay.go` paints inverse-video `── PREFIX ──` in the middle slot until the chord resolves or 2s pass.

## Connection manager flow (Ctrl-G H)

1. `Ctrl-G H` → `dispatcher` calls `sshmgr.ShowHostPicker()`.
2. `sshmgr.hosts.Load()` parses `~/.ssh/config` (via `kevinburke/ssh_config`) and merges `hosts.toml`. Result: `[]Host{Alias, User, Hostname, Port, Tags, Notes, Source}`.
3. `widgets/fuzzyfinder.New(bounds, aliases).Run(desktop)` runs modally. Items render `alias  user@host:port  [tags]` with right-aligned source badge.
4. On selection, `InputLine` dialog ("Connect command override?") pre-fills `ssh <alias>`. `Esc` cancels.
5. `sshmgr.pool.Acquire(alias)` returns a `ControlPath` (`~/.local/state/fvmux/cm/<alias>.sock`). If no master process exists, fvmux spawns a hidden `ssh -M -N -o ControlPersist=600 <alias>`.
6. `session.lifecycle.SpawnInFocusedWindow(profile.AdHoc{Command:"ssh", Args:["-S",ctlPath,"-o","ControlMaster=auto",alias]})` inserts a new `Pane`. By default it is placed below the focused pane (`[general] connect_split` overrides; its legacy `vertical` value means top/bottom and `horizontal` means left/right).
7. Pane `Title` defaults to alias; `OnTitle` overrides as remote shell emits OSC.
8. On pane exit, `pool.Release(alias)` decrements refcount; master stays alive until `ControlPersist` expires.

The same alias used twice (once in Ctrl-G H, once in SFTP browser) reuses the master — that's the shared pool.

## SFTP browser flow (Ctrl-G F)

1. Same host picker as above. Selection produces an `sftp.Client` bound to a pooled SSH client (`pool.SSHClients[alias]`, shared with subsequent calls).
2. UI: a new fvmux `views.Window` containing a `views.SplitGroup{Vertical}`:
   - **Left panel**: `SplitGroup{Horizontal}` → top = `TreeView` (local), bottom = `TaskProgress` strip.
   - **Right panel**: `SplitGroup{Horizontal}` → top = `TreeView` (remote, lazy-loaded via `OnExpand`), bottom = preview pane (swappable view).
3. TreeViews show name, human-size, mode bits, mtime. Sort: dirs first, then alpha. `Enter` on dir expands; on file opens preview.
4. **Preview dispatch** (`sftp/preview.go`):
   - **text** (`.txt .log .conf .go .py` etc., or printable-ASCII ratio > 0.95 in first 4 KiB) → read-only text view.
   - **markdown** (`.md`) → `widgets/markdown.MarkdownView`.
   - **image** (`.png .jpg .gif .webp`) → `widgets/imageview.New` (SIXEL when supported, else half-block).
   - **binary** (null byte in first 8 KiB, or magic-byte hit on ELF/Mach-O/ZIP, or unknown extension) → `widgets/hexedit.New(bounds, hexedit.NewMemorySource(buf))`. Lazy-loads first 64 KiB; key `L` in hex view loads full file.
5. **Transfer queue**: `F5` copies left→right, `F6` right→left. Pushes a `Transfer` onto a worker channel; 1 in-flight by default (`[sftp] parallel = N`). Each gets a `TaskProgress` row; cancel via `Del`. Completed transfers fade after 3s.
6. Header strip per panel: `cwd`, `du` of current dir, free-space (`StatFS`).
7. Hex auto-trigger order: (a) size > 16 MiB → ask first; (b) text-allowlist extension → text; (c) null byte or printable ratio < 0.85 → hex; (d) else text.

## Mouse behaviour

- **Title-bar drag** → inherited from `views.Window.HandleEvent` (`pkg/fv/views/window.go:316`).
- **Corner resize** → inherited from `views.Window.resizeLoop` (`pkg/fv/views/window.go:364`).
- **Splitter drag** → `views.SplitGroup` handles its own drag; fvmux's render walks the tree and updates `Ratio` after a splitter ends.
- **Click pane to focus** → fvmux's pre-process view inspects mouse-down whose target leaf differs from current focus; updates `session.focus`. Terminal's own SGR-1006 forwarding (when inner app enabled `1006`/`1000`) is unchanged.
- **Right-click pane** → `widgets/popupmenu.New(origin, items, 24).Run(desktop)`. Items by pane state:
  - Always: *Split left/right*, *Split top/bottom*, *Zoom/Unzoom*, *Rename pane*.
  - If alive: *Send SIGINT*, *Send SIGTERM*, *Send EOF*.
  - If dead: *Respawn*, *Remove*.
  - If SSH (Profile prefix matches): *Open SFTP here*, *Copy alias*.
- **Wheel** in terminal scrollback works as fv-go ships it.

## First-run UX

1. `cmd/fvmux/main.go` reads `state.toml`. If `first_run_done == false`:
   - **Splash** — `splash.Show()` renders `assets/splash.six` if `sixel.Supported()` says yes; else `assets/splash.txt` centred. Auto-dismisses after 1.5s or any keypress.
   - **Welcome dialog** — `dialogs.NewDialog` modal with `markdown` view ("Hi. fvmux is a terminal multiplexer. Press Ctrl-G ? any time for help.") and three buttons: *Take the tour*, *Skip*, *Quit*.
   - **Tour (optional)** — five overlay tooltips highlighting menu bar, status line, prefix indicator, window number, splitter.
   - **Prefix-key picker** — PopupMenu offering `Ctrl-G` (default), `Ctrl-B`, `Ctrl-A`, *Custom…*. Writes choice to `config.toml`.
2. Write `state.toml` with `first_run_done = true`, `welcome_shown_at = now`, `last_version = <build version>`.
3. Subsequent runs skip everything. On version bump, show a one-line `Notification` linking to `Ctrl-G ?` "What's new".

## Whimsy budget (capped)

- **Splash logo** — 60-cell-wide rounded box framing a stylised "fvmux" wordmark whose `v` is a splitter glyph (`├`) and `x` is the close-box (`■`). One of three subtitles cycles per launch: *"windows in your windows"* / *"a multiplexer for the rest of us"* / *"now with hex"*. SIXEL version uses a mountain-twilight palette (`#1b1d2a → #c6a0f6`); ASCII fallback uses box-drawing + ANSI 256-color gradient. Fades through five dim attribute steps.
- **Easter eggs** (specific, executable):
  - Triple `Ctrl-G` within 1.5s → splash replay.
  - `Ctrl-G :` command prompt: `:tea` → Notification "steeping…", then "🍵" exactly 180s later (cancellable).
  - `:konami` (`↑↑↓↓←→←→ba`) → flips CPU sparkline upside-down for the session.
  - Window titled "home" → tiny `🏠` glyph prepended in window list.
  - New window on Friday after 17:00 local → status middle reads "ship it" for 4s.
  - `:rot13` rotates the focused pane's incoming bytes for 10s.
  - Cheatsheet footer randomly picks one of four taglines.
- **Bell flash** — `OnBell` (upstream PR #2) inverts focused window's title bar in two 80ms ticks (320ms total). Status-line window entry gains `!` for 4s. Config: `bell = off | flash | notify | both`.
- **Named themes** — each carries a `tagline` shown next to its name in `Ctrl-G T`:
  - **slate** — *"the one you'll forget you chose, in a good way."* Cool greys + coral accent (`#e87a6f`).
  - **tokyonight-ish** — *"borrowed warmth from a city that doesn't sleep."* Indigo bg, cyan splitters, magenta prefix indicator.
  - **solarbeach** — *"solarized went on vacation and came back tan."* Cream bg, terracotta accent.

The whimsy budget caps here. Everywhere else fvmux stays quiet and professional.

## Testing strategy

**Unit (`go test ./...`)**
- `internal/layout` — full algebra: every SplitH/V position, Close from every position, Swap, Zoom, BreakOut/JoinFrom; property tests assert invariants after 100k seeded random sequences.
- `internal/config` — TOML roundtrip per schema: marshal defaults → parse → deep-equal. Golden files.
- `internal/keys` — parser accepts `"prefix |"`, `"C-x C-c"`, `"S-F1"`; rejects malformed.
- `internal/sshmgr` — merger order stability; hosts.toml overrides ssh_config when alias collides.
- `internal/sftp/binary.go` — sniff function on fixture corpus (`testdata/{text.go, binary.elf, mixed.bin, image.png, markdown.md}`).

**Headless integration (`test/headless/`)**
- Drive `app.Application` with synthetic events through a test backend. Assertions: prefix + `c` adds a window; `%` creates a SplitGroup with two children; resize mode + `Shift-L` resizes right child by 5 cells; golden frame diffs after deterministic event scripts.

**Smoke (`test/smoke/*.sh`)**
- Real terminals: kitty, ghostty, alacritty, iTerm2, tmux-inside-tmux. Each script lists exact keystrokes and expected observations. Includes SIXEL probe + bell flash. SSH/SFTP scripts run against a docker-compose'd localhost OpenSSH on `:2222`.

## Roadmap beyond v1

1. **Detach/reattach daemon split.** `cmd/fvmux` becomes a thin client; new `cmd/fvmuxd` owns PTYs and the layout tree. Unix socket with SCM_RIGHTS for pty fds. v1's layout tree is already pure data → serialisation problem, not redesign.
2. **Plugin / scripting.** Embed `risor` or `starlark-go`. Expose commands and event hooks. Plugins in `~/.config/fvmux/plugins/`. v1 already routes through `OnCommand` IDs → mechanical to expose.
3. **Mosh transport.** Host entries grow `transport = "mosh"`; spawn `mosh <host>`. Predictive echo + roaming free. Pool extends with `MoshConn`.
4. **Pane recording / replay.** Tee terminal parser input to asciicast v2; replay is a fake PTY emitting at recorded timestamps. Needs `terminal.OnFeed []byte` upstream.
5. **Layout DSL.** Promote the inline `layout = """..."""` to a five-rule PEG. Parse target = existing `PaneNode`.
6. **Theme marketplace.** Signed-manifest static-site registry; `Ctrl-G T m` browses + downloads to `~/.config/fvmux/themes/`. Themes are pure data, no code execution.

## Critical files to read/modify

**fv-go (Stage 0 upstream PRs — see § Stage 0 for the full PR train and file map)**
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/widgets/terminal/vt.go` — scrollback cap (A1), bell handling (A2), DEC modes for paste detection (B2), OSC switch for cwd (B1), mouse-mode gating (E2)
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/widgets/terminal/terminal.go` — `readLoop` wrap (A2), `Paste` helper (B2), `ScrollbackText` (E1), `SetFocused` (E3), `Stop()` process-group kill (A3)
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/widgets/terminal/pty_unix.go` — `Setsid` on spawn (A3)
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/widgets/terminal/pty_windows.go` — ConPTY job-object lifecycle (A3)
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/views/splitter.go` — `GetRatio` / `SetRatio` / `SetMinPanel` (C1)
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/views/window.go` — `OnMove` / `OnResize` callbacks (E4), `SetNumber` (E5)
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/menus/statusline.go` — `LeftItems` / `RightItems` slots (C2)
- `/Users/apfau/GolandProjects/fv-go/pkg/fv/app/app.go` — `OnQuitRequest` (E6), `Done()` PTY walk (E7)
- new `/Users/apfau/GolandProjects/fv-go/pkg/fv/theme/` package — `Palette` + `Set` (D1), with edits to `pkg/fv/views/{frame,window,scroll}.go`, `pkg/fv/menus/{menubar,statusline}.go`

**fv-go (reference / reused without modification)**
- `pkg/fv/app/desktop.go` — `InsertWindow`, `Focus`, `MakeFirst`, `SelectNext`
- `pkg/fv/views/window.go:316,364` — title-bar drag + corner resize fvmux inherits
- `pkg/fv/views/group.go:268-281` — `OfPreProcess` ordering (prefix-key plumbing)
- `cmd/fvdemo/main.go` — `OnCommand` dispatch-chain pattern that `cmd/fvmux/main.go` mirrors
- `pkg/fv/widgets/{fuzzyfinder,popupmenu,treeview,markdown,taskprogress,hexedit,imageview}` — widgets consumed verbatim. Confirmed: `hexedit.NewMemorySource(data)` with `SetReadOnly(true)` (`hexedit.go:67,125`); `treeview.OnExpand` fires lazily on first expansion (`treeview.go:100`); `markdown.SetMarkdown(string)` (`markdown.go:65-70`).

**Implementation order (end-to-end)**

**Stage 0 — fv-go gap-filling** — ✅ complete (see § Stage 0). Action remaining: tag `fv-go v0.2.0`.

**Stage 1 — fvmux bootstrap** (against the tag)
1. `cmd/fvmux/main.go` + `internal/app/` + `internal/prefix/` — boots a multiplexer with one default pane responding to `Ctrl-G c` (new window) and `Ctrl-G ?` (cheatsheet).
2. `internal/commands/` — Command type + Registry; populated from `defaults.go`. Source of truth for menus, palette, cheatsheet.
3. `internal/layout/` — algebra + tests landed before any window has more than one pane.
4. `internal/session/` + `internal/config/` + `internal/profile/` — sessions, profiles, persistence.
5. `internal/keys/` + `internal/menus/` (menu builder) + `internal/palette/` (Ctrl-G P) — the discoverable shell.
6. `internal/statusbar/` (now atop fv-go's native `LeftItems`/`RightItems`) + `internal/theme/` (atop fv-go's `theme.Palette`).
7. `internal/clipboard/` (atotto/clipboard wrapper).
8. `internal/splash/` + `internal/cheatsheet/` (auto-generated from the command registry) — first-run UX.
9. `internal/sshmgr/` + connection manager UI.
10. `internal/sftp/` + browser UI + hex preview.
11. `test/headless/` integration tests against fv-go's new headless backend (`pkg/fv/term/headless.go`).
12. Polish: easter eggs, theme tuning, smoke scripts.

## Verification

After implementation, the v1 acceptance script:

```bash
# 1. Builds cleanly on all targets
go build ./...
GOOS=linux   go build ./...
GOOS=windows go build ./...

# 2. Tests pass with race detector
go test -race ./...

# 3. Headless integration golden frames
go test ./test/headless/ -update=false

# 4. Manual smoke checklist (test/smoke/v1.md)
#    inside an outer tmux:
#    - Launch fvmux. Splash appears, welcome dialog asks for prefix.
#    - Ctrl-G c opens a second window; Ctrl-G n cycles.
#    - Ctrl-G % splits the focused pane; Ctrl-G h/l navigates.
#    - Ctrl-G z zooms; second Ctrl-G z restores.
#    - Ctrl-G ! breaks current pane to a floating window.
#    - Title-bar drag moves the window; splitter drag resizes.
#    - Ctrl-G H opens host picker; selecting a host spawns an ssh pane.
#    - Ctrl-G F opens SFTP browser; binary file opens in hex view.
#    - Ctrl-G ? opens cheatsheet, scrolls correctly.
#    - Bell from inside a pane flashes the title bar.
#    - Status bar shows CPU sparkline updating each second.
#    - Quitting prompts with running-panes confirm; on yes, all PTYs killed.
#    - State persists: re-launching skips splash, restores last session if -session=<name>.

# 5. SFTP integration test (test/smoke/sftp.md)
#    docker compose up sshd:2222
#    fvmux → Ctrl-G F → connect localhost:2222 → upload + download + hex view of /bin/ls
```

Cross-platform expectations: macOS + Linux are first-class. Windows builds clean; SFTP works; outer-tmux scenario is N/A on Windows (use Windows Terminal directly).

## Open tactical questions (surface during implementation)

These don't block the plan but will need a call as you build:

- **SIGINT routing**: capture in `internal/app/signals.go` and forward to focused pane's PTY (`pty.Write([]byte{0x03})`)? Or let it kill fvmux as users might expect? Default proposed: forward to focused pane; provide `Ctrl-G Ctrl-C Ctrl-C` to actually quit.
- **`replace` directive in `go.mod`**: kept during dual-repo development, stripped before tagging a release. CI guard checks for it.
- **Window-number badges only go 1–9** (`views.Window.number`); for window 10+, badge falls back to "+" via a small fvmux-side override. The `Ctrl-G w` window-list dialog handles the long tail.
- **Bell debounce**: ≥ 500ms between consecutive `OnBell` flashes to avoid seizure-grade strobing if a child spams BEL.
