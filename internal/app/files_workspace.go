package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/sftp"
)

type fileConnectState uint8

const (
	filesConnecting fileConnectState = iota
	filesAuthRequired
	filesWaitingAuth
	filesReady
	filesFailed
	filesCancelled
)

const (
	cmFilesRetry uint16 = 0xF100 + iota
	cmFilesAuthenticate
	cmFilesTerminal
	cmFilesCancel
)

type fileWindowState struct {
	ID        session.WindowID
	Number    int
	Frame     *views.Window
	Alias     string
	RemoteCWD string
	LocalCWD  string
	FocusSide string
	// OriginPaneID binds this Files window to the terminal pane it was opened
	// from. FollowTerminal is deliberately per window, so several browsers for
	// one SSH alias can independently follow different terminal panes.
	OriginPaneID   session.PaneID
	FollowTerminal bool

	State       fileConnectState
	Detail      string
	Browser     *sftp.Browser
	cancel      context.CancelFunc
	gen         uint64
	poolHeld    bool
	controlPath string
	closing     bool
}

func (fw *fileWindowState) displayTitle() string {
	if fw == nil {
		return "files"
	}
	suffix := fw.RemoteCWD
	if suffix == "" {
		suffix = "home"
	}
	switch fw.State {
	case filesConnecting:
		suffix = "Connecting…"
	case filesAuthRequired:
		suffix = "Authentication required"
	case filesWaitingAuth:
		suffix = "Waiting for authentication…"
	case filesFailed:
		suffix = "Connection failed"
	case filesCancelled:
		suffix = "Cancelled"
	}
	relation := ""
	if fw.FollowTerminal {
		relation = " ↔ Terminal"
	}
	return fmt.Sprintf("[%s] Files%s — %s", fw.Alias, relation, suffix)
}

func (fw *fileWindowState) stateLabel() string {
	if fw == nil {
		return "unknown"
	}
	switch fw.State {
	case filesConnecting:
		return "connecting"
	case filesAuthRequired:
		return "authentication required"
	case filesWaitingAuth:
		return "waiting for authentication"
	case filesReady:
		return "ready"
	case filesFailed:
		return "failed"
	case filesCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

func (m *Mux) currentFileWindow() *fileWindowState {
	if m == nil || m.App == nil {
		return nil
	}
	return m.fileWindows[m.App.Desktop.Current()]
}

func (m *Mux) workspaceWindowExists(key views.View) bool {
	if key == nil {
		return false
	}
	return m.windows[key] != nil || m.fileWindows[key] != nil
}

func (m *Mux) workspaceWindowNumber(key views.View) int {
	if ws := m.windows[key]; ws != nil {
		return ws.Number
	}
	if fw := m.fileWindows[key]; fw != nil {
		return fw.Number
	}
	return 0
}

func (m *Mux) workspaceWindowTitle(key views.View) string {
	if ws := m.windows[key]; ws != nil {
		return ws.displayTitle()
	}
	if fw := m.fileWindows[key]; fw != nil {
		return fw.displayTitle()
	}
	return ""
}

func (m *Mux) filesForAlias(alias string) *fileWindowState {
	for i := len(m.windowOrder) - 1; i >= 0; i-- {
		if fw := m.fileWindows[m.windowOrder[i]]; fw != nil && fw.Alias == alias {
			return fw
		}
	}
	return nil
}

func (m *Mux) openFilesHere(forceNew bool) {
	leaf := focusedPane(m.currentWindow())
	if leaf == nil || leaf.Pane == nil || leaf.Pane.SSHAlias == "" {
		return
	}
	alias, cwd := leaf.Pane.SSHAlias, leaf.Pane.CWD
	if !forceNew {
		if fw := m.filesForAlias(alias); fw != nil {
			fw.OriginPaneID = leaf.Pane.ID
			if cwd != "" {
				fw.RemoteCWD = cwd
				if fw.Browser != nil {
					fw.Browser.NavigateRemote(cwd)
				} else {
					m.beginFilesConnect(fw)
				}
			}
			m.focusWindowView(fw.Frame.Self())
			m.warnUnknownTerminalCWD(cwd, true)
			return
		}
	}
	fw := m.openFilesWindow(alias, cwd, "", "remote", nil, 0)
	if fw != nil {
		fw.OriginPaneID = leaf.Pane.ID
	}
	m.warnUnknownTerminalCWD(cwd, false)
}

func (m *Mux) warnUnknownTerminalCWD(cwd string, reused bool) {
	if cwd != "" {
		return
	}
	message := "Terminal has not reported its directory (OSC 7); Files opened at remote home"
	if reused {
		message = "Terminal has not reported its directory (OSC 7); Files kept its current folder"
	}
	m.setFlash(message, 4*time.Second, flashPrioCommand)
	m.refreshStatusBar()
}

// paneByID resolves the current terminal behind a runtime-only Files link.
// Pane IDs survive splits, joins, and break-outs, unlike window/leaf pointers.
func (m *Mux) paneByID(id session.PaneID) *session.Pane {
	if id == 0 {
		return nil
	}
	for _, ws := range m.windows {
		if ws == nil || ws.Root == nil {
			continue
		}
		if leaf := ws.Root.FindByID(id); leaf != nil {
			return leaf.Pane
		}
	}
	return nil
}

func (m *Mux) followFilesForPane(pane *session.Pane, cwd string) {
	if pane == nil || cwd == "" {
		return
	}
	for _, fw := range m.fileWindows {
		if fw == nil || fw.closing || !fw.FollowTerminal || fw.OriginPaneID != pane.ID {
			continue
		}
		fw.RemoteCWD = cwd
		if fw.Browser != nil {
			fw.Browser.NavigateRemote(cwd)
		}
		if fw.Frame != nil {
			fw.Frame.SetTitle(fw.displayTitle())
		}
	}
}

// detachFilesFromPane preserves companion Files windows when their terminal is
// removed, but clears a link that can no longer receive directory updates.
// The browser remains usable at its current folder.
func (m *Mux) detachFilesFromPane(id session.PaneID) {
	if id == 0 {
		return
	}
	for _, fw := range m.fileWindows {
		if fw == nil || fw.OriginPaneID != id {
			continue
		}
		fw.OriginPaneID = 0
		fw.FollowTerminal = false
		if fw.Frame != nil {
			fw.Frame.SetTitle(fw.displayTitle())
		}
	}
}

// rebindFilesFromPane treats Respawn as replacement rather than removal. Each
// Files window keeps its own follow toggle, including when several originated
// from the same pane.
func (m *Mux) rebindFilesFromPane(oldID session.PaneID, replacement *session.Pane) {
	if oldID == 0 || replacement == nil {
		return
	}
	for _, fw := range m.fileWindows {
		if fw != nil && fw.OriginPaneID == oldID {
			fw.OriginPaneID = replacement.ID
		}
	}
}

func (m *Mux) toggleFilesFollowTerminal() {
	fw := m.currentFileWindow()
	if fw == nil {
		return
	}
	pane := m.paneByID(fw.OriginPaneID)
	if pane == nil {
		return
	}
	fw.FollowTerminal = !fw.FollowTerminal
	if fw.FollowTerminal && pane.CWD != "" {
		m.followFilesForPane(pane, pane.CWD)
	}
	if fw.Frame != nil {
		fw.Frame.SetTitle(fw.displayTitle())
	}
	message := "Files directory follow disabled"
	if fw.FollowTerminal {
		message = "Files now follows this terminal's directory"
		if pane.CWD == "" {
			message += " (waiting for OSC 7)"
		}
	}
	m.setFlash(message, 3*time.Second, flashPrioCommand)
	m.refreshStatusBar()
}

// openFilesWindow creates the numbered workspace surface immediately, before
// connecting. Connection/auth failures therefore remain visible and actionable
// instead of disappearing into a status flash or appearing later in another
// session. A non-nil savedBounds/positive savedNumber are used by restore.
func (m *Mux) openFilesWindow(alias, remoteCWD, localCWD, focusSide string, savedBounds *geom.Rect, savedNumber int) *fileWindowState {
	if alias == "" {
		return nil
	}
	bounds := m.cascadedBoundsFor(110, 32)
	if savedBounds == nil {
		if source := m.currentWindow(); source != nil && source.Frame != nil {
			desk := m.App.Desktop.BaseView().Size
			w, h := bounds.Width(), bounds.Height()
			x, y := source.Frame.BaseView().Origin.X+3, source.Frame.BaseView().Origin.Y+2
			if x+w > desk.X {
				x = desk.X - w
			}
			if y+h > desk.Y {
				y = desk.Y - h
			}
			if x < 0 {
				x = 0
			}
			if y < 0 {
				y = 0
			}
			bounds = geom.NewRect(x, y, x+w, y+h)
		}
	}
	if savedBounds != nil {
		bounds = *savedBounds
	}
	num := savedNumber
	if num <= 0 || m.windowNumberInUse(num) {
		num = m.nextWindowNumber()
	}
	w := views.NewWindow(bounds, "", num)
	w.SetSizeLimits(geom.Point{X: 80, Y: 18}, geom.Point{})
	fw := &fileWindowState{
		ID: session.NewWindowID(), Number: num, Frame: w, Alias: alias,
		RemoteCWD: remoteCWD, LocalCWD: localCWD, FocusSide: focusSide,
		State: filesConnecting,
	}
	if fw.FocusSide == "" {
		fw.FocusSide = "remote"
	}
	if cfg := m.Opts.Config; cfg != nil && !cfg.Appearance.WindowShadow {
		w.State &^= consts.SfShadow
	}
	m.fileWindows[w.Self()] = fw
	m.windowOrder = append(m.windowOrder, w.Self())
	w.OnCloseRequest = func() bool { return m.confirmFileWindowClose(fw) }
	w.OnClose = func() { m.cleanupFileWindow(fw) }
	previous := m.App.Desktop.Current()
	m.App.Desktop.InsertWindow(w)
	if m.workspaceWindowExists(previous) {
		m.lastFocused = previous
	}
	m.raiseMouseListener()
	m.renderFileWindow(fw)
	m.focusWindowView(w.Self())
	m.beginFilesConnect(fw)
	m.setFlash(fmt.Sprintf("Files for [%s] opened in window %d", alias, num), 2500*time.Millisecond, flashPrioCommand)
	return fw
}

func (m *Mux) beginFilesConnect(fw *fileWindowState) {
	if fw == nil || fw.closing {
		return
	}
	if fw.cancel != nil {
		fw.cancel()
	}
	if !fw.poolHeld {
		fw.controlPath = m.sshPool.Acquire(fw.Alias)
		fw.poolHeld = true
	}
	fw.gen++
	gen := fw.gen
	ctx, cancel := context.WithCancel(context.Background())
	fw.cancel = cancel
	fw.State, fw.Detail = filesConnecting, ""
	m.renderFileWindow(fw)
	host := m.hostByAlias(fw.Alias)
	sock := fw.controlPath
	sftp.ShowAsync(ctx, m.App, fw.Frame, fw.Alias, sock, host.ConnectOpts(), m.Opts.Config.SFTP.Parallel,
		sftp.OpenOptions{
			RemoteCWD: fw.RemoteCWD, LocalCWD: fw.LocalCWD, FocusSide: fw.FocusSide,
			OnTerminal: func() { m.goToTerminalForAlias(fw.Alias) },
			OnRemoteCWD: func(cwd string) {
				if !fw.closing {
					fw.RemoteCWD = cwd
					fw.Frame.SetTitle(fw.displayTitle())
					m.refreshStatusBar()
				}
			},
		},
		func(browser *sftp.Browser, err error) {
			if fw.closing || m.fileWindows[fw.Frame.Self()] != fw || fw.gen != gen {
				if browser != nil {
					browser.Close(nil)
				}
				return
			}
			cancel()
			fw.cancel = nil
			if err != nil {
				if errors.Is(err, context.Canceled) {
					fw.State = filesCancelled
				} else if errors.Is(err, sftp.ErrAuthRequired) {
					fw.State = filesAuthRequired
				} else {
					fw.State, fw.Detail = filesFailed, err.Error()
				}
				m.renderFileWindow(fw)
				m.refreshStatusBar()
				return
			}
			fw.Browser = browser
			fw.State = filesReady
			fw.RemoteCWD, fw.LocalCWD, fw.FocusSide = browser.RemoteCWD(), browser.LocalCWD(), browser.FocusSide()
			// The terminal may have changed directory while the connection or
			// authentication handoff was in flight. A following Files window
			// catches up to the newest OSC-7 value as soon as it becomes ready.
			if fw.FollowTerminal {
				if pane := m.paneByID(fw.OriginPaneID); pane != nil && pane.CWD != "" && pane.CWD != fw.RemoteCWD {
					fw.RemoteCWD = pane.CWD
					browser.NavigateRemote(pane.CWD)
				}
			}
			fw.Frame.SetTitle(fw.displayTitle())
			m.setFlash(fmt.Sprintf("Files for [%s] are ready in window %d", fw.Alias, fw.Number), 2500*time.Millisecond, flashPrioCommand)
			m.refreshStatusBar()
		})
}

func (m *Mux) renderFileWindow(fw *fileWindowState) {
	if fw == nil || fw.Frame == nil || fw.State == filesReady {
		return
	}
	clearWindowBody(fw.Frame)
	fw.Frame.SetTitle(fw.displayTitle())
	w, h := fw.Frame.BaseView().Size.X, fw.Frame.BaseView().Size.Y
	detail := fw.Detail
	if len(detail) > 240 {
		detail = detail[:240] + "…"
	}
	var body string
	var buttons []*dialogs.Button
	switch fw.State {
	case filesConnecting:
		body = fmt.Sprintf("Connecting Files to [%s]…\n\nRemote folder: %s\n\nYou can cancel this attempt without closing the SSH terminal.", fw.Alias, emptyAs(fw.RemoteCWD, "account home"))
		buttons = append(buttons, dialogs.NewButton(geom.NewRect(3, h-4, 15, h-3), "~C~ancel", cmFilesCancel, dialogs.BfDefault))
	case filesAuthRequired:
		body = fmt.Sprintf("[%s] needs interactive SSH authentication.\n\nAuthenticate opens or focuses its terminal, then this window visibly waits and retries.", fw.Alias)
		buttons = append(buttons,
			dialogs.NewButton(geom.NewRect(3, h-4, 20, h-3), "~A~uthenticate", cmFilesAuthenticate, dialogs.BfDefault),
			dialogs.NewButton(geom.NewRect(23, h-4, 34, h-3), "~R~etry", cmFilesRetry, 0))
	case filesWaitingAuth:
		body = fmt.Sprintf("Waiting for [%s] authentication…\n\nFinish signing in in the terminal. Files will retry as soon as the shared SSH connection is ready.", fw.Alias)
		buttons = append(buttons, dialogs.NewButton(geom.NewRect(3, h-4, 20, h-3), "Show ~T~erminal", cmFilesTerminal, dialogs.BfDefault))
	case filesFailed:
		body = fmt.Sprintf("Could not connect Files to [%s].\n\n%s", fw.Alias, detail)
		buttons = append(buttons,
			dialogs.NewButton(geom.NewRect(3, h-4, 14, h-3), "~R~etry", cmFilesRetry, dialogs.BfDefault),
			dialogs.NewButton(geom.NewRect(17, h-4, 35, h-3), "Show ~T~erminal", cmFilesTerminal, 0))
	case filesCancelled:
		body = fmt.Sprintf("The Files connection to [%s] was cancelled.", fw.Alias)
		buttons = append(buttons, dialogs.NewButton(geom.NewRect(3, h-4, 14, h-3), "~R~etry", cmFilesRetry, dialogs.BfDefault))
	}
	text := dialogs.NewStaticText(geom.NewRect(3, 3, w-3, h-6), body)
	text.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	fw.Frame.Insert(text)
	for _, b := range buttons {
		b.GrowMode = consts.GfGrowAll
		fw.Frame.Insert(b)
	}
	actions := newFilesActionView(m, fw)
	fw.Frame.Insert(actions)
	if len(buttons) > 0 && m.App.Desktop.Current() == fw.Frame.Self() {
		fw.Frame.Focus(buttons[0])
	}
	views.MarkDirty()
}

func clearWindowBody(w *views.Window) {
	if w == nil {
		return
	}
	for _, child := range append([]views.View(nil), w.Children...) {
		if child != w.Frame.Self() {
			w.Delete(child)
		}
	}
}

func emptyAs(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

type filesActionView struct {
	views.Base
	m  *Mux
	fw *fileWindowState
}

func newFilesActionView(m *Mux, fw *fileWindowState) *filesActionView {
	v := &filesActionView{Base: views.NewBase(geom.NewRect(0, 0, 0, 0)), m: m, fw: fw}
	v.SetSelf(v)
	v.Options |= consts.OfPreProcess
	return v
}
func (v *filesActionView) GetTypeID() string { return "filesactions" }
func (v *filesActionView) Draw()             {}
func (v *filesActionView) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvKeyDown && ev.KeyCode == consts.KbEsc {
		v.fw.Frame.Close()
		ev.What = consts.EvNothing
		return
	}
	if ev.What != consts.EvCommand {
		return
	}
	switch ev.Command {
	case cmFilesRetry:
		v.m.beginFilesConnect(v.fw)
	case cmFilesAuthenticate:
		v.m.authenticateFilesWindow(v.fw)
	case cmFilesTerminal:
		v.m.goToTerminalForAlias(v.fw.Alias)
	case cmFilesCancel:
		v.fw.gen++
		if v.fw.cancel != nil {
			v.fw.cancel()
			v.fw.cancel = nil
		}
		v.fw.State = filesCancelled
		v.m.renderFileWindow(v.fw)
	default:
		return
	}
	ev.What = consts.EvNothing
}

func (m *Mux) authenticateFilesWindow(fw *fileWindowState) {
	if fw == nil || fw.closing {
		return
	}
	if !m.goToTerminalForAlias(fw.Alias) {
		fw.State, fw.Detail = filesFailed, "Could not open an SSH terminal for authentication."
		m.renderFileWindow(fw)
		return
	}
	if fw.cancel != nil {
		fw.cancel()
	}
	fw.gen++
	gen := fw.gen
	ctx, cancel := context.WithCancel(context.Background())
	fw.cancel = cancel
	fw.State, fw.Detail = filesWaitingAuth, ""
	m.renderFileWindow(fw)
	sock := fw.controlPath
	go func() {
		ready := pollForMasterContext(ctx, fw.Alias, sock, 30*time.Second)
		views.CallSoon(func() {
			if fw.closing || fw.gen != gen || m.fileWindows[fw.Frame.Self()] != fw {
				return
			}
			fw.cancel = nil
			if ready {
				m.beginFilesConnect(fw)
				return
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				fw.State = filesCancelled
			} else {
				fw.State, fw.Detail = filesFailed, "Authentication did not complete within 30 seconds. You can keep the terminal open and retry."
			}
			m.renderFileWindow(fw)
		})
	}()
}

func pollForMasterContext(ctx context.Context, alias, sock string, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		if masterAlive(alias, sock) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-tick.C:
		}
	}
}

func (m *Mux) goToTerminalForAlias(alias string) bool {
	if m.focusTerminalForAlias(alias) {
		return true
	}
	prof := m.sshProfile(m.hostByAlias(alias), alias)
	if _, err := m.openWindowFromProfile(prof); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error, "ssh %s failed:\n%s", []any{alias, err.Error()}, msgbox.OKOnly)
		return false
	}
	return true
}

func (m *Mux) focusTerminalForAlias(alias string) bool {
	if pane := m.lastSSHPane[alias]; pane != nil && !pane.Dead && pane.SSHAlias == alias {
		if ws := m.windowContainingPane(pane); ws != nil {
			if leaf := ws.Root.FindByID(pane.ID); leaf != nil && leaf.Pane == pane {
				m.focusWindowView(ws.Frame.Self())
				m.setPaneFocus(ws, leaf)
				return true
			}
		}
		delete(m.lastSSHPane, alias)
	}
	for i := len(m.windowOrder) - 1; i >= 0; i-- {
		ws := m.windows[m.windowOrder[i]]
		if ws == nil || ws.Root == nil {
			continue
		}
		leaves := ws.Root.CollectLeaves()
		for j := len(leaves) - 1; j >= 0; j-- {
			if p := leaves[j].Pane; p != nil && !p.Dead && p.SSHAlias == alias {
				m.focusWindowView(ws.Frame.Self())
				m.setPaneFocus(ws, leaves[j])
				return true
			}
		}
	}
	return false
}

func (m *Mux) confirmFileWindowClose(fw *fileWindowState) bool {
	if fw == nil || fw.Browser == nil {
		return true
	}
	n := fw.Browser.ActiveOperations()
	if n == 0 {
		return true
	}
	body := fmt.Sprintf("Close [%s] Files and cancel %d active operation", fw.Alias, n)
	if n != 1 {
		body += "s"
	}
	body += "?"
	if m.confirmFilesClosePrompt != nil {
		return m.confirmFilesClosePrompt(body)
	}
	return msgbox.Show(&m.App.Desktop.Group, msgbox.Question, body, msgbox.YesNo) == consts.CmYes
}

func (m *Mux) cleanupFileWindow(fw *fileWindowState) {
	if fw == nil || fw.closing {
		return
	}
	fw.closing = true
	fw.gen++
	if fw.cancel != nil {
		fw.cancel()
		fw.cancel = nil
	}
	key := fw.Frame.Self()
	delete(m.fileWindows, key)
	m.removeWindowOrderKey(key)
	if m.lastFocused == key {
		m.lastFocused = nil
	}
	held := fw.poolHeld
	fw.poolHeld = false
	release := func() {
		if held && m.sshPool != nil {
			m.sshPool.Release(fw.Alias)
		}
	}
	if fw.Browser != nil {
		fw.Browser.Close(release)
	} else {
		release()
	}
	m.refreshStatusBar()
}

func (m *Mux) removeWindowOrderKey(key views.View) {
	for i, k := range m.windowOrder {
		if k == key {
			m.windowOrder = append(m.windowOrder[:i], m.windowOrder[i+1:]...)
			return
		}
	}
}

func (m *Mux) closeAllFileWindows() {
	for _, key := range append([]views.View(nil), m.windowOrder...) {
		if fw := m.fileWindows[key]; fw != nil {
			// Session replacement is already authorized by its aggregate
			// confirmation; suppress the per-window transfer prompt.
			fw.Frame.OnCloseRequest = nil
			fw.Frame.Close()
		}
	}
}
