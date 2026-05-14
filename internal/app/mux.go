package app

import (
	"strings"
	"time"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/cheatsheet"
	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/copymode"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/prefix"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/sftp"
	"github.com/oldwired/fvmux/internal/splash"
	"github.com/oldwired/fvmux/internal/sshmgr"
	"github.com/oldwired/fvmux/internal/statusbar"
	muxtheme "github.com/oldwired/fvmux/internal/theme"
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
func composeTitle(userTitle, shellTitle, fallback string) string {
	switch {
	case userTitle != "" && shellTitle != "":
		return "[" + userTitle + "] " + shellTitle
	case userTitle != "":
		return "[" + userTitle + "]"
	case shellTitle != "":
		return shellTitle
	default:
		return fallback
	}
}

func (m *Mux) refreshWindowTitle(ws *windowState) {
	if ws == nil || ws.Frame == nil {
		return
	}
	ws.Frame.SetTitle(composeTitle(ws.UserTitle, ws.ShellTitle, ws.Title))
}

// Options bundles everything Mux needs at construction time. Each
// field has a sensible zero value, so callers can populate only what
// they need.
type Options struct {
	Paths           config.Paths
	Config          *config.Config
	Profiles        []*profile.Profile
	Themes          []*muxtheme.Theme
	StatusBar       *statusbar.Bar
	SessionName     string // empty ⇒ ephemeral session, no autosave
	StartingProfile string // empty ⇒ honour config.General.DefaultProfile
	Version         string // build version, recorded in state.toml

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
	windowOrder []views.View // insertion order, used by Next/Prev navigation.
	prefix      *prefix.View
	lastFocused views.View // for Ctrl-G Tab MRU toggle.

	resizeMode bool
	resizeView *prefix.ResizeView
	syncView   *prefix.SyncView
	mouseView  *prefix.MouseView

	layoutPreset layout.Preset

	hideClock bool // Ctrl-G t suppresses the right-side clock.

	flashUntil time.Time // Ctrl-G q numbers overlay deadline.
	flashText  string

	tickerStop chan struct{}

	sshPool *sshmgr.Pool

	dynamic *dispatchTable // dynamic menu Cm → action; rebuilt each refresh.

	konamiOn bool // :konami flips the CPU sparkline upside-down.
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
	m := &Mux{
		App:     a,
		Reg:     reg,
		Opts:    opts,
		windows: map[views.View]*windowState{},
		sshPool: sshmgr.NewPool(opts.Paths.ControlSocket),
	}
	m.wireActions()
	return m
}

// ShutdownSSHPool tears down every active ControlMaster process. Called
// from the cmd/fvmux deferred shutdown so we don't leak orphan ssh
// children when fvmux exits before ControlPersist expires.
func (m *Mux) ShutdownSSHPool() {
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
	bind(commands.CmdCheatsheet, m.ShowCheatsheet)
	bind(commands.CmdLiteralPrefix, func() { m.LiteralForward(0x07) })
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
		if t := m.FocusedTerminal(); t != nil {
			copymode.Show(m.App, t)
		}
	})
	bind(commands.CmdPaste, func() {
		_ = copymode.Paste(m.FocusedTerminal())
	})
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

	bind(commands.CmdOpenSession, m.openSessionPicker)
	bind(commands.CmdSaveSessionAs, m.saveSessionAs)
	bind(commands.CmdRenameSession, m.renameSessionFile)
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
	if found := ws.Root.FindByID(focusedID); found != nil {
		ws.Focus = found
	} else if leaves := ws.Root.CollectLeaves(); len(leaves) > 0 {
		ws.Focus = leaves[0]
	}
	if ws.Zoomed != nil && ws.Root.FindByID(*ws.Zoomed) == nil {
		ws.Zoomed = nil
	}
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
	sock, _ := m.sshPool.Acquire(h.Alias)
	if sock != "" {
		defer m.sshPool.Release(h.Alias)
	}
	sftp.Show(m.App, h.Alias, sock)
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
	// Try to warm a ControlMaster so subsequent connects / SFTP skip
	// re-auth. If the pool fails (bad alias, ssh missing, etc.) fall
	// back to a direct `ssh alias` — the connection still happens, it
	// just re-authenticates next time.
	args := []string{h.Alias}
	if sock, err := m.sshPool.Acquire(h.Alias); err == nil && sock != "" {
		args = []string{"-S", sock, h.Alias}
	}
	prof := &profile.Profile{
		Name:    h.Alias,
		Command: "ssh",
		Args:    args,
		Title:   h.Alias,
	}
	_, err := m.openWindowFromProfile(prof)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"ssh %s failed:\n%s", []any{h.Alias, err.Error()}, msgbox.OKOnly)
	}
}

// openWindowFromProfile shares the bulk of NewWindow but accepts an
// arbitrary Profile (used for ad-hoc spawns like ssh hosts).
func (m *Mux) openWindowFromProfile(prof *profile.Profile) (*views.Window, error) {
	bounds := m.cascadedBoundsFor(prof.WindowWidth, prof.WindowHeight)
	w := views.NewWindow(bounds, prof.Title, len(m.windowOrder)+1)
	interior := windowInterior(w)
	pane, err := profile.Instantiate(prof, interior, m.Opts.Config.Terminal.ScrollbackLines, m.Opts.Config.Terminal.Shell)
	if err != nil {
		return nil, err
	}
	m.wireTerminalCallbacks(pane, w)
	root := layout.Leaf(pane)
	state := &windowState{
		ID:     session.NewWindowID(),
		Number: len(m.windowOrder) + 1,
		Title:  prof.Title,
		Frame:  w,
		Root:   root,
		Focus:  root,
	}
	body := layout.Materialize(root, interior, nil)
	w.Insert(body)
	m.registerWindow(w, state)
	return w, nil
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
	_ = config.Save(m.Opts.Paths.ConfigFile(), m.Opts.Config)
}

// NewWindow opens a fresh terminal window. If profileName == "" the
// default profile from config is used.
func (m *Mux) NewWindow(profileName string) (*views.Window, error) {
	var prof *profile.Profile
	switch {
	case profileName != "":
		prof = profile.Find(m.Opts.Profiles, profileName)
	case m.Opts.Config.General.NewWindowCommand != "":
		// Ad-hoc shell command override; the user wants Ctrl-G c to
		// run something other than the configured default profile.
		prof = &profile.Profile{
			Name:    "command",
			Command: "/bin/sh",
			Args:    []string{"-c", m.Opts.Config.General.NewWindowCommand},
		}
	default:
		prof = profile.Find(m.Opts.Profiles, m.Opts.Config.General.DefaultProfile)
	}
	if prof == nil {
		prof = profile.Defaults()[0]
	}

	bounds := m.cascadedBoundsFor(prof.WindowWidth, prof.WindowHeight)
	w := views.NewWindow(bounds, prof.Name, len(m.windowOrder)+1)
	interior := windowInterior(w)

	pane, err := profile.Instantiate(prof, interior, m.Opts.Config.Terminal.ScrollbackLines, m.Opts.Config.Terminal.Shell)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't start %s:\n%s",
			[]any{prof.Command, err.Error()},
			msgbox.OKOnly)
		return nil, err
	}
	m.wireTerminalCallbacks(pane, w)

	root := layout.Leaf(pane)
	ws := &windowState{
		ID:     session.NewWindowID(),
		Number: len(m.windowOrder) + 1,
		Title:  prof.Name,
		Frame:  w,
		Root:   root,
		Focus:  root,
	}

	body := layout.Materialize(root, interior, nil)
	w.Insert(body)

	m.registerWindow(w, ws)
	m.fridayShipIt()
	m.refreshStatusBar()
	return w, nil
}

// fridayShipIt flashes "ship it" in the status-bar focused-pane slot
// for 4 s when a new window opens on a Friday after 17:00. Uses the
// same flashUntil/flashText slot that Ctrl-G q drives for window
// numbers — they'd only collide if both fire within 4 s.
func (m *Mux) fridayShipIt() {
	if !whimsy.FridayAfterFive(time.Now()) {
		return
	}
	m.flashText = "ship it"
	m.flashUntil = time.Now().Add(4 * time.Second)
}

func (m *Mux) wireTerminalCallbacks(pane *session.Pane, w *views.Window) {
	t := pane.Term
	t.OnTitle = func(s string) {
		if s == "" {
			return
		}
		pane.Title = s
		// Only the focused pane's shell title participates in the
		// window caption; the user-set name (if any) brackets it.
		ws := m.windows[w.Self()]
		if ws == nil || ws.Focus == nil || ws.Focus.Pane != pane {
			return
		}
		ws.ShellTitle = s
		m.refreshWindowTitle(ws)
	}
	t.OnCWDChange = func(cwd string) { pane.CWD = cwd }
	t.OnActivity = func() { pane.Activity = time.Now() }
	t.OnBell = func() { m.flashOnBell(pane) }
	t.OnExit = func(err error) {
		pane.Dead = true
		pane.ExitErr = err
		if pane.CloseOnExit {
			// OnExit runs on the PTY-wait goroutine; do the actual
			// tree mutation on the main loop by posting a command.
			m.App.PostEvent(drivers.Event{
				What:    consts.EvCommand,
				Command: commands.CmdAutoClosePane,
				InfoPtr: pane,
			})
		}
	}
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
		if leaf.Pane != nil && leaf.Pane.Term != nil {
			leaf.Pane.Term.Stop()
		}
		var removed bool
		ws.Root, removed = layout.Close(ws.Root, leaf)
		if removed {
			m.removeWindow(ws)
			return
		}
		if ws.Focus == leaf {
			if leaves := ws.Root.CollectLeaves(); len(leaves) > 0 {
				ws.Focus = leaves[0]
			}
		}
		if ws.Zoomed != nil && ws.Root.FindByID(*ws.Zoomed) == nil {
			ws.Zoomed = nil
		}
		m.rerender(ws)
		return
	}
}

// FocusedTerminal returns the terminal in the currently focused
// window's focused pane.
func (m *Mux) FocusedTerminal() *terminal.Terminal {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil || ws.Focus.Pane == nil {
		return nil
	}
	return ws.Focus.Pane.Term
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
	m.prefix.OnTriplePress = func() { m.RunFirstRunWizard() }
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
	if err := config.Save(m.Opts.Paths.ConfigFile(), m.Opts.Config); err != nil {
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
	result := splash.Run(m.App, currentPrefix, currentShell, pickTheme)
	if result.QuitRequested {
		// User picked "Quit fvmux" from the welcome dialog. Route
		// via CmQuitApp so OnQuitRequest fires and graceful save runs.
		m.App.PostEvent(drivers.Event{What: consts.EvCommand, Command: consts.CmQuitApp})
		return
	}
	if result.PrefixKey != "" {
		m.ApplyPrefix(result.PrefixKey)
	}
	dirty := false
	if result.Shell != "" && result.Shell != currentShell {
		m.Opts.Config.Terminal.Shell = result.Shell
		dirty = true
	}
	if result.Theme != "" && result.Theme != m.Opts.Config.Appearance.Theme {
		m.Opts.Config.Appearance.Theme = result.Theme
		dirty = true
		// PickLive already applied the palette live; nothing extra
		// to do here beyond persisting the choice.
	}
	if dirty {
		_ = config.Save(m.Opts.Paths.ConfigFile(), m.Opts.Config)
	}
	// Mark first-run as done — persists across restarts. The caller
	// may already have set this; SaveState is cheap and idempotent.
	state, _ := config.LoadState(m.Opts.Paths.StateFile())
	state.FirstRunDone = true
	state.LastVersion = m.Opts.Version
	state.WelcomeShownAt = time.Now()
	_ = config.SaveState(m.Opts.Paths.StateFile(), state)
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
}

func (m *Mux) currentWindow() *windowState {
	cur := m.App.Desktop.Current()
	if cur == nil {
		return nil
	}
	return m.windows[cur]
}

func (m *Mux) doSplit(vertical bool) {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil || !ws.Focus.IsLeaf() {
		return
	}
	prof := profile.Find(m.Opts.Profiles, m.Opts.Config.General.DefaultProfile)
	if prof == nil {
		prof = profile.Defaults()[0]
	}
	newPane, err := profile.Instantiate(prof, geom.NewRect(0, 0, 40, 12), m.Opts.Config.Terminal.ScrollbackLines, m.Opts.Config.Terminal.Shell)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't start %s:\n%s",
			[]any{prof.Command, err.Error()},
			msgbox.OKOnly)
		return
	}
	m.wireTerminalCallbacks(newPane, ws.Frame)
	if vertical {
		ws.Root = layout.SplitV(ws.Root, ws.Focus, newPane)
	} else {
		ws.Root = layout.SplitH(ws.Root, ws.Focus, newPane)
	}
	ws.Focus = ws.Root.FindByID(newPane.ID)
	m.rerender(ws)
}

func (m *Mux) doClose() {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return
	}
	target := ws.Focus
	if paneIsAlive(target.Pane) && !m.confirmKill(
		"Close pane and kill its process?") {
		return
	}
	sibling := target.Sibling()

	if target.Pane != nil && target.Pane.Term != nil {
		target.Pane.Term.Stop()
	}

	var removed bool
	ws.Root, removed = layout.Close(ws.Root, target)
	if removed {
		m.removeWindow(ws)
		return
	}
	if sibling != nil {
		leaves := sibling.CollectLeaves()
		if len(leaves) > 0 {
			ws.Focus = leaves[0]
		}
	}
	if ws.Zoomed != nil && ws.Root.FindByID(*ws.Zoomed) == nil {
		ws.Zoomed = nil
	}
	m.rerender(ws)
}

func (m *Mux) doZoom() {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return
	}
	if ws.Zoomed != nil {
		ws.Zoomed = nil
	} else {
		id := ws.Focus.Pane.ID
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
		ws.Focus = next
		if next.Pane != nil {
			focusTerminalPath(ws.Frame, next.Pane.Term)
		}
		m.refreshStatusBar()
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
	ws.Focus = leaves[target]
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
	newSrcRoot, newWinRoot := layout.BreakOut(ws.Root, ws.Focus)
	ws.Root = newSrcRoot
	// Pick a new focus in the source.
	if newSrcRoot != nil {
		leaves := newSrcRoot.CollectLeaves()
		if len(leaves) > 0 {
			ws.Focus = leaves[0]
		}
	}
	if ws.Zoomed != nil && (ws.Root == nil || ws.Root.FindByID(*ws.Zoomed) == nil) {
		ws.Zoomed = nil
	}
	m.rerender(ws)

	// Open a new window with the detached pane as its only leaf.
	bounds := m.cascadedBounds()
	w := views.NewWindow(bounds, newWinRoot.Pane.Title, len(m.windowOrder)+1)
	interior := windowInterior(w)
	m.wireTerminalCallbacks(newWinRoot.Pane, w)
	newWs := &windowState{
		ID:     session.NewWindowID(),
		Number: len(m.windowOrder) + 1,
		Title:  newWinRoot.Pane.Title,
		Frame:  w,
		Root:   newWinRoot,
		Focus:  newWinRoot,
	}
	body := layout.Materialize(newWinRoot, interior, nil)
	w.Insert(body)
	m.registerWindow(w, newWs)
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

// focusWindowView is the single point that updates desktop focus.
// Records the previous focus into m.lastFocused so the Ctrl-G Tab
// MRU toggle has something to swap back to. Refreshes the status bar.
func (m *Mux) focusWindowView(target views.View) {
	if target == nil {
		return
	}
	cur := m.App.Desktop.Current()
	if cur == target {
		return
	}
	if _, ok := m.windows[cur]; ok {
		m.lastFocused = cur
	}
	m.App.Desktop.Focus(target)
	m.refreshStatusBar()
}

func (m *Mux) killWindow() {
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
	if !m.anyLivePanes() {
		return true
	}
	return m.confirmKill("Kill all running panes and quit?")
}

func (m *Mux) confirmKill(message string) bool {
	if m.Opts.Config == nil || !m.Opts.Config.General.ConfirmKill {
		return true
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
			if l.Pane != nil && l.Pane.Term != nil {
				l.Pane.Term.Stop()
			}
		})
	}
	if ws.Frame == nil {
		return
	}
	key := ws.Frame.Self()
	delete(m.windows, key)
	for i, k := range m.windowOrder {
		if k == key {
			m.windowOrder = append(m.windowOrder[:i], m.windowOrder[i+1:]...)
			break
		}
	}
	if m.lastFocused == key {
		m.lastFocused = nil
	}
	m.refreshStatusBar()
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
	m.refreshStatusBar()
}

// showProfilePicker stub: opens a message box listing available
// profiles. Sub-step 5 replaces this with a proper picker dialog.
func (m *Mux) showProfilePicker() {
	var b strings.Builder
	b.WriteString("Available profiles (pick a name, then re-run with -profile=NAME):\n\n")
	for _, p := range m.Opts.Profiles {
		b.WriteString("• ")
		b.WriteString(p.Name)
		if p.Command != "" {
			b.WriteString(" — ")
			b.WriteString(p.Command)
		}
		b.WriteByte('\n')
	}
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info, b.String(), msgbox.OKOnly)
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
	n := len(m.windowOrder)
	x := 2 + n*3
	y := 1 + n*2

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

	if db.Size.X > 0 && x+w > db.Size.X-1 {
		w = db.Size.X - x - 1
	}
	if db.Size.Y > 0 && y+h > db.Size.Y-1 {
		h = db.Size.Y - y - 1
	}
	if w < 20 {
		w = 20
	}
	if h < 8 {
		h = 8
	}
	return geom.NewRect(x, y, x+w, y+h)
}
