package app

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/cheatsheet"
	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/copymode"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/prefix"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/splash"
	"github.com/oldwired/fvmux/internal/sshmgr"
	"github.com/oldwired/fvmux/internal/statusbar"
	muxtheme "github.com/oldwired/fvmux/internal/theme"
	"github.com/oldwired/fvmux/internal/ui"
	"github.com/oldwired/fvmux/internal/whimsy"
)

// windowState is fvmux's per-window bookkeeping: the fv-go Window, the
// pane-tree root, the focused leaf, an optional zoomed pane, and the
// window's id/number/title for snapshotting.
//
// SyncInput, when true, broadcasts typed input from the focused pane to
// every other pane in the window — used for parallel ssh sessions.
//
// Title precedence — when rendering the window caption, UserTitle wins
// stickily but the shell-set title (OSC 0/1/2) still shows after it:
//
//	composeTitle("",       "",       "shell") = "shell"            (fallback)
//	composeTitle("",       "vim x",  "shell") = "vim x"            (shell only)
//	composeTitle("logs",   "",       "shell") = "[logs]"           (user only)
//	composeTitle("logs",   "tail -f","shell") = "[logs] tail -f"   (both)
//
// UserTitle is set by CmdRenameWindow and survives across shell title
// churn until the user re-renames (passing empty input clears it).
type windowState struct {
	ID     session.WindowID
	Number int
	Title  string // profile-derived fallback caption.

	UserTitle  string // empty = no user override.
	ShellTitle string // last OSC-set title from the focused pane.

	Frame  *views.Window
	Root   *layout.PaneNode
	Focus  *layout.PaneNode // leaf
	Zoomed *session.PaneID

	SyncInput bool
}

// composeTitle is windowState's caption formula. See the type comment.
// paneTitle, when distinct from what the base caption already shows,
// is appended as " · paneTitle" so the focused-pane name surfaces in
// the title bar — useful when a window holds a split tree where each
// pane has its own identity (e.g., rename-pane gave it a custom name
// that the focused shell hasn't echoed back as OSC yet).
func composeTitle(userTitle, shellTitle, fallback, paneTitle string) string {
	var base string
	switch {
	case userTitle != "" && shellTitle != "":
		base = "[" + userTitle + "] " + shellTitle
	case userTitle != "":
		base = "[" + userTitle + "]"
	case shellTitle != "":
		base = shellTitle
	default:
		base = fallback
	}
	if paneTitle != "" && paneTitle != shellTitle && !strings.Contains(base, paneTitle) {
		base += " · " + paneTitle
	}
	return base
}

func (m *Mux) refreshWindowTitle(ws *windowState) {
	if ws == nil || ws.Frame == nil {
		return
	}
	pt := ""
	if ws.Focus != nil && ws.Focus.Pane != nil {
		pt = ws.Focus.Pane.DisplayTitle()
	}
	title := composeTitle(ws.UserTitle, ws.ShellTitle, ws.Title, pt)
	if ws.Focus != nil && ws.Focus.Pane != nil && ws.Focus.Pane.SSHAlias != "" {
		alias := ws.Focus.Pane.SSHAlias
		if title == "" || title == alias {
			title = fmt.Sprintf("[%s] Terminal", alias)
		} else {
			title = fmt.Sprintf("[%s] Terminal — %s", alias, title)
		}
	}
	ws.Frame.SetTitle(title)
}

// Options bundles everything Mux needs at construction time. Each
// field has a sensible zero value, so callers can populate only what
// they need.
type Options struct {
	Paths       config.Paths
	Config      *config.Config
	Profiles    []*profile.Profile
	Themes      []*muxtheme.Theme
	StatusBar   *statusbar.Bar
	SessionName string // empty ⇒ ephemeral session, no autosave
	Version     string // build version, recorded in state.toml

	// RefreshUI is invoked after any Mux action that changes the
	// surface bindings (e.g., ApplyPrefix rewrites every chord — the
	// caller rebuilds the menu bar with the new shortcut strings).
	RefreshUI func()
}

// Mux is fvmux's top-level runtime state.
type Mux struct {
	App  *fvapp.Application
	Reg  *commands.Registry
	Opts Options

	windows     map[views.View]*windowState
	fileWindows map[views.View]*fileWindowState
	windowOrder []views.View // insertion order, used by Next/Prev navigation.
	prefix      *prefix.View
	lastFocused views.View // for Ctrl-G Tab MRU toggle.
	lastSSHPane map[string]*session.Pane

	resizeMode bool
	resizeView *prefix.ResizeView
	syncView   *prefix.SyncView
	mouseView  *prefix.MouseView
	copyMode   *copymode.Driver // active copy-mode session, nil/inactive otherwise

	// syncSend, when non-nil, replaces the default per-pane delivery in
	// broadcastIfSync (terminal.HandleEvent). Production leaves it nil; tests
	// inject a recorder to observe exactly which panes the sync gate reaches.
	syncSend func(ev *drivers.Event, t *terminal.Terminal)
	// pasteTo similarly lets command-target tests observe the selected terminal
	// without depending on a process-global operating-system clipboard.
	pasteTo func(*terminal.Terminal) error
	// confirmKillPrompt isolates confirmation policy from modal UI in tests.
	// Production leaves it nil and uses the fv-go message box.
	confirmKillPrompt func(string) bool

	layoutPreset layout.Preset

	hideClock bool // Ctrl-G t suppresses the right-side clock.

	flashUntil time.Time // Ctrl-G q numbers overlay deadline.
	flashText  string
	flashPrio  int // priority of the current flash; see setFlash.

	tickerStop chan struct{}

	sshPool *sshmgr.Pool

	dynamic *dispatchTable // dynamic menu Cm → action; rebuilt each refresh.

	konamiOn bool // :konami flips the CPU sparkline upside-down.

	eggMu     sync.Mutex
	eggTimers []*time.Timer // pending easter-egg AfterFunc handles.

	confirmFilesClosePrompt func(string) bool

	// signalQuit is set by the OS-signal handler so OnQuitRequested skips
	// the interactive confirm-kill prompt (a modal is impossible during
	// signal-driven shutdown) and goes straight to save-and-exit.
	signalQuit atomic.Bool
}

// NewMux builds a Mux around the given Application, registry, and options.
func NewMux(a *fvapp.Application, reg *commands.Registry, opts Options) *Mux {
	if opts.Config == nil {
		opts.Config = config.Defaults()
	}
	if len(opts.Profiles) == 0 {
		opts.Profiles = profile.Defaults()
	}
	if len(opts.Themes) == 0 {
		opts.Themes = muxtheme.All(opts.Paths.ThemesDir())
	}
	// Reap stale ControlMaster sockets left behind by a crashed prior
	// run before standing up the pool (safe: only dead sockets are
	// removed; live masters shared with other instances are untouched).
	sshmgr.SweepStale(opts.Paths.ControlSocketDir())
	m := &Mux{
		App:         a,
		Reg:         reg,
		Opts:        opts,
		windows:     map[views.View]*windowState{},
		fileWindows: map[views.View]*fileWindowState{},
		lastSSHPane: map[string]*session.Pane{},
		sshPool:     sshmgr.NewPool(opts.Paths.ControlSocket),
	}
	m.wireActions()
	return m
}

// ShutdownSSHPool tears down every active ControlMaster process. Called
// from the cmd/fvmux deferred shutdown so we don't leak orphan ssh
// children when fvmux exits before ControlPersist expires.
func (m *Mux) ShutdownSSHPool() {
	m.closeAllFileWindows()
	if m.sshPool != nil {
		m.sshPool.Shutdown()
	}
}

func (m *Mux) wireActions() {
	bind := func(id uint16, fn func()) {
		if c := m.Reg.ByID(id); c != nil {
			c.Action = func(*commands.Ctx) { fn() }
		}
	}
	bind(commands.CmdNewWindow, func() { _, _ = m.NewWindow("") })
	bind(commands.CmdNewWindowFromProfile, m.showProfilePicker)
	bind(commands.CmdQuit, func() {
		// Route via CmQuitApp so OnQuitRequest fires (auto-save, etc.).
		m.App.PostEvent(drivers.Event{What: consts.EvCommand, Command: consts.CmQuitApp})
	})
	bind(commands.CmdCommandPalette, func() {
		m.openPalette()
	})
	bind(commands.CmdOpenMenu, func() {
		m.App.PostEvent(drivers.Event{What: consts.EvCommand, Command: consts.CmMenu})
	})
	bind(commands.CmdCheatsheet, m.ShowCheatsheet)
	bind(commands.CmdLiteralPrefix, func() {
		// Derive the byte from the live spec — after a rebind to Ctrl-B
		// the double-tap must forward 0x02, not a hardcoded Ctrl-G/BEL
		// (which aborts readline edits instead).
		spec := prefix.Lookup(m.Opts.Config.General.PrefixKey)
		if m.prefix != nil {
			spec = m.prefix.Spec()
		}
		if b := spec.LiteralByte(); b != 0 {
			m.LiteralForward(b)
		}
	})
	bind(commands.CmdSplitH, func() { m.doSplit(false) })
	bind(commands.CmdSplitV, func() { m.doSplit(true) })
	bind(commands.CmdClosePane, m.doClose)
	bind(commands.CmdZoomPane, m.doZoom)
	bind(commands.CmdFocusLeft, func() { m.doFocusDir(layout.Left) })
	bind(commands.CmdFocusDown, func() { m.doFocusDir(layout.Down) })
	bind(commands.CmdFocusUp, func() { m.doFocusDir(layout.Up) })
	bind(commands.CmdFocusRight, func() { m.doFocusDir(layout.Right) })
	bind(commands.CmdSwapNext, func() { m.doSwap(+1) })
	bind(commands.CmdSwapPrev, func() { m.doSwap(-1) })
	bind(commands.CmdBreakOut, m.doBreakOut)
	bind(commands.CmdNextWindow, func() { m.cycleWindow(+1) })
	bind(commands.CmdPrevWindow, func() { m.cycleWindow(-1) })
	bind(commands.CmdKillWindow, m.killWindow)
	bind(commands.CmdSaveSession, m.SaveSession)
	bind(commands.CmdThemePicker, m.showThemePicker)
	bind(commands.CmdEditThemes, m.editThemes)

	bind(commands.CmdEnterCopyMode, func() {
		if m.copyMode.Active() {
			return // single-instance: never stack a second driver
		}
		t := m.FocusedTerminal()
		if t == nil {
			return
		}
		m.copyMode = copymode.Show(m.App, t)
		if m.copyMode == nil {
			return
		}
		// Suspend the prefix listener for the mode's duration (same
		// discipline as resize mode) so chords can't fire — and, e.g.,
		// kill the very pane copy mode is reading — mid-session.
		if m.prefix != nil {
			m.prefix.SetSuspended(true)
		}
		m.copyMode.OnDone = func() {
			if m.prefix != nil {
				m.prefix.SetSuspended(false)
			}
		}
	})
	bind(commands.CmdPaste, m.pasteClipboard)
	bind(commands.CmdFindScrollback, func() {
		// fv-go's terminal owns the search UI (input prompt in the
		// status row, n/N jump, match highlighting). We just hand it
		// the keys via StartScrollbackSearch and step back — the
		// terminal becomes interactive for the user's query.
		if t := m.FocusedTerminal(); t != nil {
			t.StartScrollbackSearch()
		}
	})

	bind(commands.CmdConnectHost, m.connectHost)
	bind(commands.CmdEditHosts, m.openHostsEditor)
	bind(commands.CmdActiveConnections, m.showActiveConnections)
	bind(commands.CmdReloadHosts, m.reloadHosts)
	bind(commands.CmdSFTPBrowser, m.sftpBrowser)
	bind(commands.CmdSFTPHere, func() { m.openFilesHere(false) })
	bind(commands.CmdSFTPNewHere, func() { m.openFilesHere(true) })
	bind(commands.CmdToggleFilesFollow, m.toggleFilesFollowTerminal)
	bind(commands.CmdUploadFile, m.transferHintUpload)
	bind(commands.CmdDownloadFile, m.transferHintDownload)
	bind(commands.CmdActiveTransfers, m.showActiveTransfers)
	bind(commands.CmdClearCompleted, m.clearCompletedTransfers)
	bind(commands.CmdOpenConfig, m.openConfig)
	bind(commands.CmdOpenProfiles, m.openProfiles)
	bind(commands.CmdOpenKeybindings, m.openKeybindings)
	bind(commands.CmdRenameWindow, m.renameWindow)
	bind(commands.CmdResetFirstRun, m.resetFirstRunWizard)
	bind(commands.CmdReloadConfig, m.ReloadConfig)
	bind(commands.CmdLogViewer, m.showLogViewer)
	bind(commands.CmdAbout, m.showAbout)

	bind(commands.CmdNewSession, m.newSession)
	bind(commands.CmdOpenSession, m.openSessionPicker)
	bind(commands.CmdRenameSession, m.renameSession)
	bind(commands.CmdDeleteSession, m.deleteSession)
	bind(commands.CmdSaveSessionAs, m.saveSessionAs)
	bind(commands.CmdRenamePane, m.renamePane)
	bind(commands.CmdToggleClock, m.toggleClock)
	bind(commands.CmdToggleStatusBar, m.toggleStatusBar)
	bind(commands.CmdToggleMenuBar, m.toggleMenuBar)
	bind(commands.CmdRedraw, m.redraw)
	bind(commands.CmdFlashNumbers, m.flashNumbers)
	bind(commands.CmdJoinFrom, m.joinFrom)
	bind(commands.CmdLayoutEvenH, func() { m.applyLayoutPreset(layout.PresetEvenHorizontal) })
	bind(commands.CmdLayoutEvenV, func() { m.applyLayoutPreset(layout.PresetEvenVertical) })
	bind(commands.CmdLayoutMainH, func() { m.applyLayoutPreset(layout.PresetMainHorizontal) })
	bind(commands.CmdLayoutMainV, func() { m.applyLayoutPreset(layout.PresetMainVertical) })
	bind(commands.CmdLayoutTiled, func() { m.applyLayoutPreset(layout.PresetTiled) })

	bind(commands.CmdLastWindow, m.lastWindow)
	bind(commands.CmdWindowList, m.showWindowList)
	bind(commands.CmdFindWindow, m.showWindowList)
	bind(commands.CmdFocusWindow1, func() { m.focusWindowByNumber(1) })
	bind(commands.CmdFocusWindow2, func() { m.focusWindowByNumber(2) })
	bind(commands.CmdFocusWindow3, func() { m.focusWindowByNumber(3) })
	bind(commands.CmdFocusWindow4, func() { m.focusWindowByNumber(4) })
	bind(commands.CmdFocusWindow5, func() { m.focusWindowByNumber(5) })
	bind(commands.CmdFocusWindow6, func() { m.focusWindowByNumber(6) })
	bind(commands.CmdFocusWindow7, func() { m.focusWindowByNumber(7) })
	bind(commands.CmdFocusWindow8, func() { m.focusWindowByNumber(8) })
	bind(commands.CmdFocusWindow9, func() { m.focusWindowByNumber(9) })

	bind(commands.CmdFocusNext, func() { m.focusNextPaneInTree(+1) })
	bind(commands.CmdFocusPrev, func() { m.focusNextPaneInTree(-1) })

	bind(commands.CmdEnterResize, m.enterResizeMode)
	bind(commands.CmdCycleLayout, m.cycleLayout)
	bind(commands.CmdToggleSyncInput, m.toggleSyncInput)
	bind(commands.CmdSendSIGINT, m.sendSIGINT)
	bind(commands.CmdSendSIGQUIT, m.sendSIGQUIT)
	bind(commands.CmdSendEOF, m.sendEOF)
	bind(commands.CmdSendSIGTERM, m.sendSIGTERM)
	bind(commands.CmdRespawnPane, m.respawnPane)
	bind(commands.CmdRun, m.runDialog)

	bind(commands.CmdTile, m.tile)
	bind(commands.CmdTileHorizontal, m.tileHorizontal)
	bind(commands.CmdTileVertical, m.tileVertical)
	bind(commands.CmdCascade, m.cascade)
	bind(commands.CmdCascadeNoResize, m.cascadeNoResize)

	m.wireTickerAction()
	m.wireCommandAvailability()
	m.wireTmuxAction()
}

// cycleLayout advances to the next preset and rebuilds the focused
// window's tree from its current panes. Focused pane is preserved if
// still present in the rebuilt tree.
func (m *Mux) cycleLayout() {
	next := layout.Preset((int(m.layoutPreset) + 1) % layout.PresetCount)
	m.applyLayoutPreset(next)
}

// applyLayoutPreset rebuilds the focused window's tree under p. Shared
// implementation behind Ctrl-G Space (cycle) and the View → Layout
// Preset direct-pick menu entries.
func (m *Mux) applyLayoutPreset(p layout.Preset) {
	ws := m.currentWindow()
	if ws == nil || ws.Root == nil {
		return
	}
	panes := make([]*session.Pane, 0, 8)
	ws.Root.Leaves(func(l *layout.PaneNode) {
		if l.Pane != nil {
			panes = append(panes, l.Pane)
		}
	})
	if len(panes) < 2 {
		return
	}
	m.layoutPreset = p
	focusedID := session.PaneID(0)
	if ws.Focus != nil && ws.Focus.Pane != nil {
		focusedID = ws.Focus.Pane.ID
	}
	ws.Root = layout.ApplyPreset(p, panes)
	var next *layout.PaneNode
	if found := ws.Root.FindByID(focusedID); found != nil {
		next = found
	} else if leaves := ws.Root.CollectLeaves(); len(leaves) > 0 {
		next = leaves[0]
	}
	if ws.Zoomed != nil && ws.Root.FindByID(*ws.Zoomed) == nil {
		ws.Zoomed = nil
	}
	m.setPaneFocus(ws, next)
	m.rerender(ws)
}

func (m *Mux) renameWindow() {
	ws := m.currentWindow()
	if ws == nil {
		return
	}
	text, ok := promptString(m.App, "Rename Window",
		"Custom title (empty to clear):", ws.UserTitle)
	if !ok {
		return
	}
	ws.UserTitle = text
	m.refreshWindowTitle(ws)
}

func (m *Mux) sftpBrowser() {
	if leaf := focusedPane(m.currentWindow()); leaf != nil && leaf.Pane != nil && leaf.Pane.SSHAlias != "" {
		alias := leaf.Pane.SSHAlias
		if existing := m.filesForAlias(alias); existing != nil {
			m.focusWindowView(existing.Frame.Self())
			return
		}
		fw := m.openFilesWindow(alias, leaf.Pane.CWD, "", "remote", nil, 0)
		if fw != nil {
			fw.OriginPaneID = leaf.Pane.ID
		}
		m.warnUnknownTerminalCWD(leaf.Pane.CWD, false)
		return
	}
	hosts, _ := sshmgr.Load(m.Opts.Paths.HostsFile())
	if len(hosts) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No hosts found. Add entries to ~/.ssh/config or hosts.toml.",
			msgbox.OKOnly)
		return
	}
	h := sshmgr.PickHost(m.App, hosts)
	if h == nil {
		return
	}
	if existing := m.filesForAlias(h.Alias); existing != nil {
		m.focusWindowView(existing.Frame.Self())
		return
	}
	m.openFilesWindow(h.Alias, "", "", "remote", nil, 0)
}

func (m *Mux) connectHost() {
	hosts, _ := sshmgr.Load(m.Opts.Paths.HostsFile())
	if len(hosts) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No hosts found. Add entries to ~/.ssh/config or hosts.toml.",
			msgbox.OKOnly)
		return
	}
	h := sshmgr.PickHost(m.App, hosts)
	if h == nil {
		return
	}
	// ControlMaster=auto means this connection becomes the master if
	// none is up yet, else it piggy-backs. Either way the auth flow
	// runs inside this PTY pane — the password prompt (if any) lands
	// in the pane rather than corrupting fvmux's display.
	prof := m.sshProfile(h, h.Alias)
	// connect_split decides how the connection lands: split the focused
	// pane — legacy value "vertical" stacks the new pane below (like Split
	// Top/Bottom, C-g "), while "horizontal" places it beside (like Split
	// Left/Right, C-g %) — or open a floating window ("window", also the fallback
	// when there's no focused pane to divide).
	switch m.Opts.Config.General.ConnectSplit {
	case "vertical", "horizontal":
		if ws := m.currentWindow(); ws != nil && ws.Focus != nil && ws.Focus.IsLeaf() {
			m.doSplitWith(prof, m.Opts.Config.General.ConnectSplit == "vertical")
			return
		}
	}
	_, err := m.openWindowFromProfile(prof)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"ssh %s failed:\n%s", []any{h.Alias, err.Error()}, msgbox.OKOnly)
	}
}

// openWindowFromProfile is THE window-spawn path: NewWindow, the
// profile picker, and every ad-hoc spawn (ssh hosts) funnel through
// it, and it funnels the assembly tail through finishWindow — the
// ritual used to exist in three hand-copied variants that had already
// drifted (doBreakOut forgot fridayShipIt/refreshStatusBar).
func (m *Mux) openWindowFromProfile(prof *profile.Profile) (*views.Window, error) {
	title := prof.Title
	if title == "" {
		title = prof.Name
	}
	bounds := m.cascadedBoundsFor(prof.WindowWidth, prof.WindowHeight)
	num := m.nextWindowNumber()
	w := views.NewWindow(bounds, title, num)
	interior := windowInterior(w)
	root, err := m.buildWindowRoot(prof, interior)
	if err != nil {
		return nil, err
	}
	m.finishWindow(w, num, title, root, interior)
	return w, nil
}

// buildWindowRoot materializes prof into a pane tree: a single leaf
// normally, or the profile's optional pre-split `layout` DSL (the same
// grammar session snapshots use; each leaf's profile name resolves
// against profiles.toml, falling back to the default shell). A layout
// that fails to parse degrades to a single pane with a warning rather
// than blocking the window.
func (m *Mux) buildWindowRoot(prof *profile.Profile, interior geom.Rect) (*layout.PaneNode, error) {
	if prof.Layout != "" {
		var spawned []*session.Pane
		spawn := func(spec layout.LeafSpec) (*session.Pane, error) {
			leafProf := profile.Find(m.Opts.Profiles, spec.Profile)
			if leafProf == nil {
				leafProf = profile.Defaults()[0]
			}
			pane, err := m.instantiateProfile(leafProf, interior)
			if err != nil {
				return nil, err
			}
			spawned = append(spawned, pane)
			if spec.Title != "" {
				pane.UserTitle = spec.Title
			}
			return pane, nil
		}
		root, err := layout.Unmarshal(prof.Layout, spawn)
		if err == nil && root != nil {
			return root, nil
		}
		// Bad DSL: reap whatever spawned before the parse failed, warn,
		// and open the plain single pane the user can still work in.
		for _, p := range spawned {
			m.stopPane(p)
		}
		slog.Warn("profile layout invalid; opening a single pane",
			"profile", prof.Name, "err", err)
		views.CallSoon(func() {
			msgbox.Showf(&m.App.Desktop.Group, msgbox.Warning,
				"Profile %q has an invalid layout (%v) — opened a single pane instead.",
				[]any{prof.Name, err}, msgbox.OKOnly)
		})
	}
	pane, err := m.instantiateProfile(prof, interior)
	if err != nil {
		return nil, err
	}
	return layout.Leaf(pane), nil
}

// finishWindow is the shared window-assembly tail: state bookkeeping,
// materialize, register, and the post-open refreshes. Callers create
// the frame first (panes' terminal callbacks need it) and hand
// everything here so no copy of the ritual can forget a step.
func (m *Mux) finishWindow(w *views.Window, num int, title string, root *layout.PaneNode, interior geom.Rect) *windowState {
	state := &windowState{
		ID:     session.NewWindowID(),
		Number: num,
		Title:  title,
		Frame:  w,
		Root:   root,
	}
	w.Insert(layout.Materialize(root, interior, nil))
	m.registerWindow(w, state)
	m.setPaneFocus(state, root.CollectLeaves()[0])
	m.fridayShipIt()
	return state
}

func (m *Mux) showThemePicker() {
	themes := m.Opts.Themes
	if len(themes) == 0 {
		return
	}
	idx := muxtheme.PickLive(m.App, themes, m.Opts.Config.Appearance.Theme)
	if idx < 0 {
		return
	}
	themes[idx].Apply()
	m.Opts.Config.Appearance.Theme = themes[idx].Name
	_ = config.UpdateKeys(m.Opts.Paths.ConfigFile(),
		config.KV{Section: "appearance", Key: "theme", Value: themes[idx].Name})
}

// NewWindow opens a fresh terminal window. If profileName == "" the
// default profile from config is used.
func (m *Mux) NewWindow(profileName string) (*views.Window, error) {
	var prof *profile.Profile
	var missing string // requested profile that doesn't exist, for the warning
	switch {
	case profileName != "":
		prof = profile.Find(m.Opts.Profiles, profileName)
		if prof == nil {
			missing = profileName
		}
	case m.Opts.Config.General.NewWindowCommand != "":
		// Ad-hoc shell command override; the user wants Ctrl-G c to
		// run something other than the configured default profile.
		sh, args := profile.ShellCommand(m.Opts.Config.General.NewWindowCommand)
		prof = &profile.Profile{
			Name:    "command",
			Command: sh,
			Args:    args,
		}
	default:
		name := m.Opts.Config.General.DefaultProfile
		prof = profile.Find(m.Opts.Profiles, name)
		if prof == nil && name != "" {
			missing = name
		}
	}
	if prof == nil {
		prof = profile.Defaults()[0]
	}
	if missing != "" {
		// A typo'd -profile flag or stale default_profile must not
		// silently open a generic shell the user mistakes for their
		// profile. Deferred via CallSoon: at startup this runs before
		// the event loop.
		slog.Warn("profile not found, using default shell", "profile", missing)
		views.CallSoon(func() {
			msgbox.Showf(&m.App.Desktop.Group, msgbox.Warning,
				"Profile %q not found — opened the default shell instead.",
				[]any{missing}, msgbox.OKOnly)
		})
	}

	w, err := m.openWindowFromProfile(prof)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't start %s:\n%s",
			[]any{prof.Command, err.Error()},
			msgbox.OKOnly)
		return nil, err
	}
	return w, nil
}

// Flash priorities for the shared status-bar slot. Higher wins: a
// still-active higher-priority flash is not cut short by a lower one.
const (
	flashPrioNumbers = 1 // Ctrl-G q window-number overlay.
	flashPrioShipIt  = 2 // Friday "ship it" whimsy — protected from the overlay.
	flashPrioResize  = 3 // operational feedback must remain visible over whimsy.
	flashPrioCommand = 3 // unavailable/unknown command feedback is operational too.
)

// setFlash writes the shared status-bar flash slot. It refuses to
// overwrite a flash that is still on screen with one of lower priority,
// so the brief Ctrl-G q number overlay can't prematurely wipe the Friday
// "ship it" moment. Equal-or-higher priority (or an expired slot) wins.
func (m *Mux) setFlash(text string, d time.Duration, prio int) {
	if time.Now().Before(m.flashUntil) && prio < m.flashPrio {
		return
	}
	m.flashText = text
	m.flashUntil = time.Now().Add(d)
	m.flashPrio = prio
}

// fridayShipIt flashes "ship it" in the status-bar focused-pane slot
// for 4 s when a new window opens on a Friday after 17:00.
func (m *Mux) fridayShipIt() {
	if !whimsy.FridayAfterFive(time.Now()) {
		return
	}
	m.setFlash("ship it", 4*time.Second, flashPrioShipIt)
}

func (m *Mux) wireTerminalCallbacks(pane *session.Pane) {
	t := pane.Term
	// profile.Instantiate invokes this as a pre-start configure hook. Every
	// callback is therefore installed before the reader/wait goroutines can
	// observe it; callbacks are never reassigned when a pane changes windows.
	// The dynamic pane lookup below follows joins and break-outs instead.
	t.OnFeed = func(in []byte) []byte {
		if pane.Rot13.Load() {
			return whimsy.Rot13(in)
		}
		return in
	}
	t.OnTitle = func(s string) {
		if s == "" {
			return
		}
		pane.ShellTitle = s
		// Only the focused pane's shell title participates in the
		// window caption; the user-set name (if any) brackets it.
		ws := m.windowContainingPane(pane)
		if ws == nil || ws.Focus == nil || ws.Focus.Pane != pane {
			return
		}
		ws.ShellTitle = s
		m.refreshWindowTitle(ws)
	}
	t.OnCWDChange = func(cwd string) {
		pane.CWD = cwd
		m.followFilesForPane(pane, cwd)
		m.refreshStatusBar()
	}
	t.OnActivity = func() { pane.Activity = time.Now() }
	t.OnBell = func() { m.flashOnBell(pane) }
	t.OnExit = func(err error) {
		pane.Dead = true
		pane.ExitErr = err
		if m.copyMode.Active() && m.copyMode.Term() == pane.Term {
			m.copyMode.Close()
		}
		m.refreshStatusBar()
		if pane.CloseOnExit {
			// fv-go marshals terminal callbacks onto the UI goroutine, so
			// close directly. Bouncing through a posted command only
			// deferred the close to a later loop iteration, widening the
			// race window against a concurrent user-initiated close.
			m.AutoClosePane(pane)
		}
	}
}

// windowContainingPane resolves a pane's current owner at callback-delivery
// time. Panes can move through Join and Break Out, so capturing a frame when
// callbacks are installed would require unsafe callback reassignment later.
func (m *Mux) windowContainingPane(pane *session.Pane) *windowState {
	if pane == nil {
		return nil
	}
	for _, key := range m.windowOrder {
		ws := m.windows[key]
		if ws == nil || ws.Root == nil {
			continue
		}
		leaf := ws.Root.FindByID(pane.ID)
		if leaf != nil && leaf.Pane == pane {
			return ws
		}
	}
	return nil
}

// AutoClosePane finds the window containing pane and tears it down —
// used by close-on-exit profiles after the child process dies. Bypasses
// confirm-kill (the process already exited).
func (m *Mux) AutoClosePane(pane *session.Pane) {
	if pane == nil {
		return
	}
	for _, ws := range m.windows {
		if ws == nil || ws.Root == nil {
			continue
		}
		leaf := ws.Root.FindByID(pane.ID)
		if leaf == nil {
			continue
		}
		var preferred *layout.PaneNode
		if ws.Focus == leaf {
			preferred = layout.RemovalSuccessor(leaf)
		}
		m.stopPane(leaf.Pane)
		removedPaneID := leaf.Pane.ID
		var removed bool
		ws.Root, removed = layout.Close(ws.Root, leaf)
		m.detachFilesFromPane(removedPaneID)
		if removed {
			m.removeWindow(ws)
			return
		}
		if ws.Zoomed != nil && ws.Root.FindByID(*ws.Zoomed) == nil {
			ws.Zoomed = nil
		}
		m.setPaneFocus(ws, recoverFocus(ws, preferred))
		m.rerender(ws)
		return
	}
}

// FocusedTerminal returns the terminal in the currently focused
// window's focused pane.
func (m *Mux) FocusedTerminal() *terminal.Terminal {
	ws := m.currentWindow()
	leaf := focusedPane(ws)
	if leaf == nil || leaf.Pane == nil {
		return nil
	}
	return leaf.Pane.Term
}

// LiteralForward writes b directly to the focused terminal's PTY.
func (m *Mux) LiteralForward(b byte) {
	if t := m.FocusedTerminal(); t != nil {
		_, _ = t.Write([]byte{b})
	}
}

// InstallPrefixListener inserts the prefix-key OfPreProcess view onto
// the Desktop, armed to the configured prefix (defaulting to Ctrl-G).
// Also installs the sync-input fallthrough listener — both fire before
// the focused window's content sees the event.
//
// Must be called after at least one window exists so the listeners
// aren't themselves the current desktop child.
func (m *Mux) InstallPrefixListener() {
	spec := prefix.Lookup(m.Opts.Config.General.PrefixKey)
	m.prefix = prefix.New(m.Reg, &commands.Ctx{App: m.App}, spec)
	m.prefix.OnUnknown = func(chord string) {
		m.setFlash("Unknown command: "+chord, 1800*time.Millisecond, flashPrioCommand)
		m.refreshStatusBar()
	}
	m.prefix.OnUnavailable = m.showUnavailableCommand
	m.App.Desktop.Insert(m.prefix)
	m.installSyncListener()
	m.installMouseListener()
}

// ApplyPrefix rebinds the entire registry's chords from the current
// prefix to newConfigKey ("C-g", "C-b", …), updates the prefix
// listener to arm on the new key, persists the choice to config.toml,
// and asks the host to refresh any chord-displaying UI (menu bar).
//
// No-op when newConfigKey is empty or matches the current setting.
func (m *Mux) ApplyPrefix(newConfigKey string) {
	if newConfigKey == "" {
		return
	}
	oldSpec := prefix.Lookup(m.Opts.Config.General.PrefixKey)
	newSpec := prefix.Lookup(newConfigKey)
	if oldSpec.ConfigKey == newSpec.ConfigKey {
		return
	}
	m.Reg.RebindPrefix(oldSpec.ChordToken, newSpec.ChordToken)
	if m.prefix != nil {
		m.prefix.SetSpec(newSpec)
	}
	m.Opts.Config.General.PrefixKey = newSpec.ConfigKey
	if err := config.UpdateKeys(m.Opts.Paths.ConfigFile(),
		config.KV{Section: "general", Key: "prefix_key", Value: newSpec.ConfigKey}); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Warning,
			"Couldn't persist prefix change:\n%s",
			[]any{err.Error()}, msgbox.OKOnly)
	}
	if m.Opts.RefreshUI != nil {
		m.Opts.RefreshUI()
	}
}

// RunFirstRunWizard drives the splash + welcome + prefix picker flow
// and applies whatever the user chose. Called once on initial launch
// (when state.FirstRunDone == false) and on demand via Help → Reset
// First-Run Wizard.
func (m *Mux) RunFirstRunWizard() {
	currentPrefix := m.Opts.Config.General.PrefixKey
	currentShell := m.Opts.Config.Terminal.Shell
	pickTheme := func() string {
		if len(m.Opts.Themes) == 0 {
			return ""
		}
		idx := muxtheme.PickLive(m.App, m.Opts.Themes, m.Opts.Config.Appearance.Theme)
		if idx < 0 {
			return ""
		}
		return m.Opts.Themes[idx].Name
	}
	result := splash.Run(m.App, currentPrefix, currentShell, m.Opts.Paths.Root, pickTheme)
	if result.QuitRequested {
		// User picked "Quit fvmux" from the welcome dialog. Route
		// via CmQuitApp so OnQuitRequest fires and graceful save runs.
		m.App.PostEvent(drivers.Event{What: consts.EvCommand, Command: consts.CmQuitApp})
		return
	}
	if result.PrefixKey != "" {
		m.ApplyPrefix(result.PrefixKey)
	}
	var updates []config.KV
	if result.Shell != "" && result.Shell != currentShell {
		m.Opts.Config.Terminal.Shell = result.Shell
		updates = append(updates, config.KV{Section: "terminal", Key: "shell", Value: result.Shell})
	}
	if result.Theme != "" && result.Theme != m.Opts.Config.Appearance.Theme {
		m.Opts.Config.Appearance.Theme = result.Theme
		updates = append(updates, config.KV{Section: "appearance", Key: "theme", Value: result.Theme})
		// PickLive already applied the palette live; nothing extra
		// to do here beyond persisting the choice.
	}
	if len(updates) > 0 {
		_ = config.UpdateKeys(m.Opts.Paths.ConfigFile(), updates...)
	}
	if result.InstallGlue {
		m.installGlue()
	}
	// Mark first-run as done — persists across restarts. The caller
	// may already have set this; SaveState is cheap and idempotent.
	_ = config.WithStateLock(m.Opts.Paths.StateFile(), func() error {
		state, _ := config.LoadState(m.Opts.Paths.StateFile())
		state.FirstRunDone = true
		state.LastVersion = m.Opts.Version
		state.WelcomeShownAt = time.Now()
		return config.SaveState(m.Opts.Paths.StateFile(), state)
	})
}

// resetFirstRunWizard is the Help → Reset First-Run action: confirms,
// then re-runs the wizard. The current prefix is offered as the
// preselected choice.
func (m *Mux) resetFirstRunWizard() {
	if got := msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
		"Re-run the welcome wizard now?\n\n"+
			"You'll see the splash again and can pick a new prefix key.",
		msgbox.YesNo); got != consts.CmYes {
		return
	}
	m.RunFirstRunWizard()
}

// ShowCheatsheet renders a markdown cheatsheet of every registered
// command, generated live from the registry.
func (m *Mux) ShowCheatsheet() {
	cheatsheet.Show(m.App, m.Reg)
	m.raiseMouseListener()
}

func (m *Mux) currentWindow() *windowState {
	cur := m.App.Desktop.Current()
	if cur == nil {
		return nil
	}
	return m.windows[cur]
}

// nextWindowNumber returns the lowest positive number no live window
// holds. The old len(windowOrder)+1 scheme duplicated numbers: close
// window 2 of 3, open a new one, and two windows both badge "3" — the
// newer unreachable via Ctrl-G 3, and the duplicates persisted into
// session snapshots.
func (m *Mux) nextWindowNumber() int {
	for n := 1; ; n++ {
		if !m.windowNumberInUse(n) {
			return n
		}
	}
}

// windowNumberInUse reports whether any live window carries number n.
func (m *Mux) windowNumberInUse(n int) bool {
	for _, ws := range m.windows {
		if ws != nil && ws.Number == n {
			return true
		}
	}
	for _, fw := range m.fileWindows {
		if fw != nil && fw.Number == n {
			return true
		}
	}
	return false
}

func (m *Mux) doSplit(vertical bool) {
	prof := profile.Find(m.Opts.Profiles, m.Opts.Config.General.DefaultProfile)
	if prof == nil {
		prof = profile.Defaults()[0]
	}
	m.doSplitWith(m.effectiveSplitProfile(prof), vertical)
}

// effectiveSplitProfile returns the per-spawn profile for a normal split.
// A local focused pane may donate its OSC-7 cwd, but the configured profile
// remains immutable and an SSH pane's remote path is never handed to a local
// process. Ad-hoc connect_split profiles bypass this helper entirely.
func (m *Mux) effectiveSplitProfile(prof *profile.Profile) *profile.Profile {
	if prof == nil {
		return nil
	}
	effective := *prof
	if m.Opts.Config == nil || !m.Opts.Config.General.InheritSplitCWD {
		return &effective
	}
	leaf := focusedPane(m.currentWindow())
	if leaf == nil || leaf.Pane == nil || leaf.Pane.SSHAlias != "" || leaf.Pane.CWD == "" {
		return &effective
	}
	effective.CWD = leaf.Pane.CWD
	return &effective
}

// doSplitWith divides the focused pane, spawning the new sibling from
// prof. Shared by the plain split chords (default profile) and by
// connect_split, which splits with an ad-hoc ssh profile.
func (m *Mux) doSplitWith(prof *profile.Profile, vertical bool) {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil || !ws.Focus.IsLeaf() {
		return
	}
	newPane, err := m.instantiateProfile(prof, geom.NewRect(0, 0, 40, 12))
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't start %s:\n%s",
			[]any{prof.Command, err.Error()},
			msgbox.OKOnly)
		return
	}
	if vertical {
		ws.Root = layout.SplitV(ws.Root, ws.Focus, newPane)
	} else {
		ws.Root = layout.SplitH(ws.Root, ws.Focus, newPane)
	}
	// A structural split exits zoom mode so both the old pane and the new,
	// focused sibling are immediately visible.
	ws.Zoomed = nil
	m.setPaneFocus(ws, ws.Root.FindByID(newPane.ID))
	m.rerender(ws)
}

func (m *Mux) doClose() {
	ws := m.currentWindow()
	target := focusedPane(ws)
	if target == nil {
		return
	}
	if paneIsAlive(target.Pane) && !m.confirmKill(
		"Close pane and kill its process?") {
		return
	}
	preferred := layout.RemovalSuccessor(target)

	m.stopPane(target.Pane)
	removedPaneID := target.Pane.ID

	var removed bool
	ws.Root, removed = layout.Close(ws.Root, target)
	m.detachFilesFromPane(removedPaneID)
	if removed {
		m.removeWindow(ws)
		return
	}
	if ws.Zoomed != nil && ws.Root.FindByID(*ws.Zoomed) == nil {
		ws.Zoomed = nil
	}
	m.setPaneFocus(ws, recoverFocus(ws, preferred))
	m.rerender(ws)
}

func (m *Mux) doZoom() {
	ws := m.currentWindow()
	focus := focusedPane(ws)
	if focus == nil {
		return
	}
	if ws.Zoomed != nil {
		ws.Zoomed = nil
	} else {
		id := focus.Pane.ID
		ws.Zoomed = &id
	}
	m.rerender(ws)
}

func (m *Mux) doFocusDir(dir layout.Direction) {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return
	}
	if next := layout.FocusDir(ws.Root, ws.Focus, dir); next != nil {
		m.setPaneFocus(ws, next)
	}
}

func (m *Mux) doSwap(direction int) {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return
	}
	leaves := ws.Root.CollectLeaves()
	idx := -1
	for i, l := range leaves {
		if l == ws.Focus {
			idx = i
			break
		}
	}
	if idx == -1 || len(leaves) < 2 {
		return
	}
	target := (idx + direction + len(leaves)) % len(leaves)
	layout.Swap(ws.Focus, leaves[target])
	// Focus follows the moved pane (it now lives at target's old slot).
	m.setPaneFocus(ws, leaves[target])
	m.rerender(ws)
}

func (m *Mux) doBreakOut() {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return
	}
	if len(ws.Root.CollectLeaves()) < 2 {
		return
	}
	preferred := layout.RemovalSuccessor(ws.Focus)
	newSrcRoot, newWinRoot := layout.BreakOut(ws.Root, ws.Focus)
	ws.Root = newSrcRoot
	// The moved leaf is no longer in ws.Root; recoverFocus chooses a live
	// candidate, which the canonical focus transition applies below.
	if ws.Zoomed != nil && (ws.Root == nil || ws.Root.FindByID(*ws.Zoomed) == nil) {
		ws.Zoomed = nil
	}
	m.setPaneFocus(ws, recoverFocus(ws, preferred))
	m.rerender(ws)

	// Open a new window with the detached pane as its only leaf, via
	// the shared assembly tail (this copy of the ritual used to forget
	// fridayShipIt/refreshStatusBar).
	bounds := m.cascadedBounds()
	num := m.nextWindowNumber()
	title := newWinRoot.Pane.DisplayTitle()
	w := views.NewWindow(bounds, title, num)
	interior := windowInterior(w)
	m.finishWindow(w, num, title, newWinRoot, interior)
}

func (m *Mux) cycleWindow(direction int) {
	if len(m.windowOrder) < 2 {
		return
	}
	cur := m.App.Desktop.Current()
	idx := -1
	for i, k := range m.windowOrder {
		if k == cur {
			idx = i
			break
		}
	}
	if idx == -1 {
		idx = 0
	}
	next := (idx + direction + len(m.windowOrder)) % len(m.windowOrder)
	m.focusWindowView(m.windowOrder[next])
}

// activeWindowKey returns the windowOrder key for the most recently
// focused fvmux window. Walks back through Desktop.Children when
// Current() is a modal dialog (for example a msgbox), so the snapshot
// captures the window the user was actually working in rather than
// the popup that happens to be on top. nil ⇒ no windows.
func (m *Mux) activeWindowKey() views.View {
	if cur := m.App.Desktop.Current(); cur != nil {
		if m.workspaceWindowExists(cur) {
			return cur
		}
	}
	// Topmost fvmux window in z-order (skip dialogs and the mouse
	// listener).
	children := m.App.Desktop.Children
	for i := len(children) - 1; i >= 0; i-- {
		if m.workspaceWindowExists(children[i]) {
			return children[i]
		}
	}
	if m.lastFocused != nil {
		if m.workspaceWindowExists(m.lastFocused) {
			return m.lastFocused
		}
	}
	if len(m.windowOrder) > 0 {
		return m.windowOrder[0]
	}
	return nil
}

// focusWindowView is the single point that activates a desktop window.
// It raises the target as well as moving keyboard focus: Group.Focus alone
// intentionally leaves z-order unchanged, which can focus an obscured window.
// Records the previous focus into m.lastFocused so the Ctrl-G Tab MRU toggle
// has something to swap back to. Refreshes the status bar.
func (m *Mux) focusWindowView(target views.View) {
	if target == nil {
		return
	}
	cur := m.App.Desktop.Current()
	if cur != target && m.workspaceWindowExists(cur) {
		m.lastFocused = cur
	}
	// MakeFirst is a no-op when target is already topmost, so Focus must remain
	// explicit. Re-raise the transparent mouse listener afterward; it needs to
	// receive clicks before windows but must never become the focused child.
	m.App.Desktop.MakeFirst(target)
	m.App.Desktop.Focus(target)
	m.raiseMouseListener()
	m.refreshStatusBar()
}

func (m *Mux) killWindow() {
	if fw := m.currentFileWindow(); fw != nil {
		fw.Frame.Close()
		return
	}
	ws := m.currentWindow()
	if ws == nil {
		return
	}
	if hasLivePanes(ws) && !m.confirmKill(
		"Kill window and all its panes?") {
		return
	}
	m.removeWindow(ws)
}

// CanQuit is the OnQuitRequest hook. Returns true if quit may proceed.
// When ConfirmKill is set and at least one window has a live pane, the
// user is asked first.
func (m *Mux) CanQuit() bool {
	activeTransfers := 0
	for _, fw := range m.fileWindows {
		if fw != nil && fw.Browser != nil {
			activeTransfers += fw.Browser.ActiveOperations()
		}
	}
	if activeTransfers > 0 {
		body := fmt.Sprintf("Quit and cancel %d active file operation(s)?", activeTransfers)
		if m.confirmFilesClosePrompt != nil {
			return m.confirmFilesClosePrompt(body)
		}
		return msgbox.Show(&m.App.Desktop.Group, msgbox.Question, body, msgbox.YesNo) == consts.CmYes
	}
	if !m.anyLivePanes() {
		return true
	}
	return m.confirmKill("Kill all running panes and quit?")
}

func (m *Mux) confirmKill(message string) bool {
	if m.Opts.Config == nil || !m.Opts.Config.General.ConfirmKill {
		return true
	}
	if m.confirmKillPrompt != nil {
		return m.confirmKillPrompt(message)
	}
	return msgbox.Show(&m.App.Desktop.Group, msgbox.Info, message,
		msgbox.YesNo) == consts.CmYes
}

func (m *Mux) anyLivePanes() bool {
	for _, ws := range m.windows {
		if hasLivePanes(ws) {
			return true
		}
	}
	return false
}

func hasLivePanes(ws *windowState) bool {
	if ws == nil || ws.Root == nil {
		return false
	}
	alive := false
	ws.Root.Leaves(func(l *layout.PaneNode) {
		if paneIsAlive(l.Pane) {
			alive = true
		}
	})
	return alive
}

func paneIsAlive(p *session.Pane) bool {
	return p != nil && !p.Dead
}

// stopPane stops a pane's terminal, first tearing down copy mode if it
// is running on that terminal — otherwise the copy-mode driver would
// stay installed desktop-wide, eating keys on behalf of a stopped pane.
// Every path that stops a pane's PTY must come through here.
func (m *Mux) stopPane(pane *session.Pane) {
	if pane == nil {
		return
	}
	// The pane owns one pool ref when it was spawned through sshProfile
	// — release exactly once (stopPane can run twice for the same pane:
	// doClose stops it, then cleanupWindow's leaf walk sees it again).
	if pane.SSHAlias != "" && m.sshPool != nil {
		if m.lastSSHPane[pane.SSHAlias] == pane {
			delete(m.lastSSHPane, pane.SSHAlias)
		}
		m.sshPool.Release(pane.SSHAlias)
		pane.SSHAlias = ""
	}
	if pane.Term == nil {
		return
	}
	if m.copyMode.Active() && m.copyMode.Term() == pane.Term {
		m.copyMode.Close()
	}
	pane.Term.Stop()
}

// removeWindow tears down a window from fvmux's side and detaches it
// from the desktop. Used by command paths that initiate the close
// themselves (Ctrl-G &, last-pane Close, etc.).
func (m *Mux) removeWindow(ws *windowState) {
	m.cleanupWindow(ws)
	if ws.Frame != nil && ws.Frame.BaseView().Owner != nil {
		m.App.Desktop.Delete(ws.Frame.Self())
	}
}

// cleanupWindow drops a window from m.windows / windowOrder and stops
// every PTY underneath it, but does NOT detach the frame from the
// desktop. Safe to call when fv-go is about to detach the frame itself
// (the Window.OnClose hook): we synchronously walk the live subtree
// to call Stop on each Terminal before the parent's Delete drops it.
func (m *Mux) cleanupWindow(ws *windowState) {
	if ws == nil {
		return
	}
	if ws.Root != nil {
		ws.Root.Leaves(func(l *layout.PaneNode) {
			m.stopPane(l.Pane)
			if l.Pane != nil {
				m.detachFilesFromPane(l.Pane.ID)
			}
		})
	}
	if ws.Frame == nil {
		return
	}
	key := ws.Frame.Self()
	delete(m.windows, key)
	m.removeWindowOrderKey(key)
	if m.lastFocused == key {
		m.lastFocused = nil
	}
	m.refreshStatusBar()
}

// recoverFocus returns a live focus candidate after Close / BreakOut /
// AutoClose. The caller applies it through setPaneFocus so every focus
// transition performs the same validation and UI refreshes.
func recoverFocus(ws *windowState, preferred ...*layout.PaneNode) *layout.PaneNode {
	if ws == nil || ws.Root == nil {
		return nil
	}
	for _, candidate := range preferred {
		if candidate != nil && candidate.Pane != nil &&
			ws.Root.FindByID(candidate.Pane.ID) == candidate {
			return candidate
		}
	}
	if ws.Focus != nil && ws.Focus.Pane != nil &&
		ws.Root.FindByID(ws.Focus.Pane.ID) == ws.Focus {
		return ws.Focus
	}
	leaves := ws.Root.CollectLeaves()
	if len(leaves) == 0 {
		return nil
	}
	return leaves[0]
}

// rerender rebuilds the fv-go view tree under ws.Frame to match the
// current layout. Terminals are detached and reparented; SplitGroup
// instances are fresh each call.
func (m *Mux) rerender(ws *windowState) {
	if ws == nil || ws.Frame == nil {
		return
	}
	for i := len(ws.Frame.Children) - 1; i >= 0; i-- {
		child := ws.Frame.Children[i]
		if child == views.View(ws.Frame.Frame) {
			continue
		}
		ws.Frame.Delete(child)
	}
	interior := windowInterior(ws.Frame)
	body := layout.Materialize(ws.Root, interior, ws.Zoomed)
	if body == nil {
		return
	}
	ws.Frame.Insert(body)
	if ws.Focus != nil && ws.Focus.Pane != nil {
		focusTerminalPath(ws.Frame, ws.Focus.Pane.Term)
	}
	m.refreshWindowTitle(ws)
	m.refreshStatusBar()
}

// showProfilePicker opens a fuzzy picker over m.Opts.Profiles, the
// same shape as sshmgr.PickHost. Picking a row spawns a new window
// from that profile.
func (m *Mux) showProfilePicker() {
	if len(m.Opts.Profiles) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No profiles defined. Edit ~/.config/fvmux/profiles.toml to add some.",
			msgbox.OKOnly)
		return
	}
	items := make([]string, len(m.Opts.Profiles))
	for i, p := range m.Opts.Profiles {
		items[i] = profilePickerRow(p)
	}
	desk := m.App.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 70, 14, 4)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	ff := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), items)
	idx := ff.Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(m.Opts.Profiles) {
		return
	}
	if _, err := m.NewWindow(m.Opts.Profiles[idx].Name); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't spawn %s:\n%s",
			[]any{m.Opts.Profiles[idx].Name, err.Error()}, msgbox.OKOnly)
	}
}

// profilePickerRow formats one Profile for the fuzzyfinder. Mirrors
// sshmgr.Host.DisplayRow's shape so the two pickers feel the same.
func profilePickerRow(p *profile.Profile) string {
	name := p.Name
	desc := p.Title
	if desc == "" {
		desc = p.Command
		if len(p.Args) > 0 {
			desc += " " + strings.Join(p.Args, " ")
		}
	}
	if desc == "" {
		return name
	}
	return name + "  —  " + desc
}

func windowInterior(w *views.Window) geom.Rect {
	sz := w.Size
	if sz.X < 4 {
		sz.X = 4
	}
	if sz.Y < 3 {
		sz.Y = 3
	}
	return geom.NewRect(1, 1, sz.X-1, sz.Y-1)
}

func focusTerminalPath(top *views.Window, t *terminal.Terminal) {
	if t == nil {
		return
	}
	var v views.View = t
	topSelf := top.Self()
	for {
		bv := v.BaseView()
		if bv == nil || bv.Owner == nil {
			return
		}
		bv.Owner.Focus(v)
		next := bv.Owner.Self()
		if next == nil || next == v || next == topSelf {
			return
		}
		v = next
	}
}

func (m *Mux) cascadedBounds() geom.Rect {
	return m.cascadedBoundsFor(0, 0)
}

// cascadedBoundsFor returns the next-window placement. Width / height
// resolve as: explicit args (when non-zero) override; otherwise the
// config [appearance] default_window_width / _height; otherwise 80×24.
// Bounds are clamped to fit the desktop.
func (m *Mux) cascadedBoundsFor(reqW, reqH int) geom.Rect {
	db := m.App.Desktop.BaseView()
	w, h := 80, 24
	if a := m.Opts.Config.Appearance; a.DefaultWindowWidth > 0 {
		w = a.DefaultWindowWidth
	}
	if a := m.Opts.Config.Appearance; a.DefaultWindowHeight > 0 {
		h = a.DefaultWindowHeight
	}
	if reqW > 0 {
		w = reqW
	}
	if reqH > 0 {
		h = reqH
	}
	return cascadedRect(db.Size, len(m.windowOrder), w, h)
}

// cascadedRect fits one requested window wholly inside a desktop-local
// rectangle. The offset wraps independently on each axis; size is chosen
// before position so late windows never shrink and then get forced back past
// the edge by a nominal minimum. Tiny desktops simply use all available cells.
func cascadedRect(size geom.Point, n, requestedW, requestedH int) geom.Rect {
	if size.X <= 0 || size.Y <= 0 {
		return geom.NewRect(0, 0, 0, 0)
	}
	w, h := requestedW, requestedH
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w > size.X {
		w = size.X
	}
	if h > size.Y {
		h = size.Y
	}

	maxX, maxY := size.X-w, size.Y-h
	baseX, baseY := 2, 1
	if baseX > maxX {
		baseX = maxX
	}
	if baseY > maxY {
		baseY = maxY
	}
	x, y := baseX, baseY
	if span := maxX - baseX + 1; span > 1 {
		x += (n * 3) % span
	}
	if span := maxY - baseY + 1; span > 1 {
		y += (n * 2) % span
	}
	return geom.NewRect(x, y, x+w, y+h)
}
