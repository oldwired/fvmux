package app

import (
	"fmt"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/debug"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/prefix"
)

// installMouseListener inserts the click-to-focus / right-click-context
// listener as a desktop-sized topmost child. fv-go's mouse dispatch
// walks children topmost-first and skips views whose MouseInView is
// false (i.e., views with empty Size), so the bounds here are what
// make the listener visible to mouse-down events at all.
//
// Wheel forwarding is no longer this view's concern: fv-go now emits
// EvMouseWheel as a separate event kind, and the terminal handles its
// own wheel scroll. We keep the listener focused on click events.
func (m *Mux) installMouseListener() {
	db := m.App.Desktop.BaseView()
	bounds := geom.NewRect(0, 0, db.Size.X, db.Size.Y)
	m.mouseView = prefix.NewMouseView(bounds, m.handleMouseDown)
	m.App.Desktop.Insert(m.mouseView)
	m.raiseMouseListener()
}

// registerWindow installs a freshly-built window:
//
//   - records ws in m.windows / windowOrder so it's discoverable;
//   - wires w.OnClose so fv-go's frame close box (and any other path
//     that calls Window.Close()) flows through cleanupWindow — that
//     stops the PTYs and drops the entry from our maps before fv-go
//     detaches the frame;
//   - inserts on the desktop and raises the mouse listener back to
//     topmost.
//
// Single point of truth — every window-creation path goes through
// here so the bookkeeping stays in lock-step with the view tree.
func (m *Mux) registerWindow(w *views.Window, ws *windowState) {
	// window_shadow=false is honoured here — the single chokepoint all
	// creation paths pass through (fv-go sets SfShadow by default).
	// Nil-tolerant: unit tests build a bare Mux without Options.Config.
	if cfg := m.Opts.Config; cfg != nil && !cfg.Appearance.WindowShadow {
		w.State &^= consts.SfShadow
	}
	m.windows[w.Self()] = ws
	m.windowOrder = append(m.windowOrder, w.Self())
	// Window-frame close requests enter through fv-go before OnClose or
	// detachment. Explicit fvmux kill commands already confirm and remove the
	// frame directly, so they bypass this hook and cannot double-prompt.
	w.OnCloseRequest = func() bool {
		return !hasLivePanes(ws) || m.confirmKill("Kill window and all its panes?")
	}
	w.OnClose = func() { m.cleanupWindow(ws) }
	// A mouse-drag resize stretches the body live through fv-go's GrowMode
	// propagation, but it never rebuilds our tree — so Pane.LastRect (the
	// click hit-map, refreshed only inside layout.Materialize) goes stale
	// and clicks stop landing on the right pane. OnResize fires once when
	// the drag ends; rerender there to resync the hit-map and re-apply the
	// ratio-based split positions, matching the keyboard-resize path.
	w.OnResize = func(geom.Point) { m.rerender(ws) }
	previous := m.App.Desktop.Current()
	m.App.Desktop.InsertWindow(w)
	if m.workspaceWindowExists(previous) {
		m.lastFocused = previous
	}
	m.raiseMouseListener()
}

// raiseMouseListener moves the mouse view to the topmost slot in the
// Desktop's child list without changing which child is focused. Mouse
// dispatch is topmost-first, so this guarantees mouseView intercepts
// every click before windows / splitters / dialogs do. The currently
// focused child is preserved across the reshuffle by re-focusing it.
//
// Call this after any operation that inserts a new desktop child (new
// fvmux window, modal dialog close, …).
func (m *Mux) raiseMouseListener() {
	if m.mouseView == nil {
		return
	}
	desk := &m.App.Desktop.Group
	curFocus := desk.Current()
	last := len(desk.Children) - 1
	if last < 0 {
		return
	}
	before := m.childrenOrder()
	// Already topmost?
	if desk.Children[last] == views.View(m.mouseView) {
		if debug.Mouse() {
			debug.Logf("mouse", "raiseMouseListener: already topmost order=%s", before)
		}
		return
	}
	// Remove from its current position; append at the end.
	for i, c := range desk.Children {
		if c == views.View(m.mouseView) {
			desk.Children = append(desk.Children[:i], desk.Children[i+1:]...)
			break
		}
	}
	desk.Children = append(desk.Children, m.mouseView)
	// Restore focus to whatever was current before the shuffle. The
	// mouse view isn't selectable so we never want it to become current.
	if curFocus != nil && curFocus != views.View(m.mouseView) {
		desk.Focus(curFocus)
	}
	if debug.Mouse() {
		debug.Logf("mouse", "raiseMouseListener: before=%s after=%s",
			before, m.childrenOrder())
	}
}

// childrenOrder returns a short descriptor of the desktop's child list,
// used in debug traces to confirm topmost-first ordering of mouseView.
func (m *Mux) childrenOrder() string {
	var b strings.Builder
	b.WriteByte('[')
	for i, c := range m.App.Desktop.Children {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch v := c.(type) {
		case *prefix.MouseView:
			b.WriteString("MouseView")
		case *prefix.View:
			b.WriteString("PrefixView")
		case *prefix.SyncView:
			b.WriteString("SyncView")
		case *prefix.ResizeView:
			b.WriteString("ResizeView")
		case *views.Window:
			b.WriteString(fmt.Sprintf("Win#%d", v.Number()))
		default:
			b.WriteString(fmt.Sprintf("%T", c))
		}
	}
	b.WriteByte(']')
	return b.String()
}

// handleMouseDown is the MouseView callback. Returns true to consume
// (right-click only — pops a menu modally). Left-clicks update pane
// focus without consuming, so the terminal's own click handler (cursor
// positioning, link-following, …) still runs.
//
// Wheel ticks no longer reach this function: fv-go projects them as
// EvMouseWheel, which MouseView.HandleEvent filters out before
// invoking OnMouse.
type mouseEvent = drivers.Event

func (m *Mux) handleMouseDown(ev *mouseEvent) bool {
	// Any mouse-down on a window body makes fv-go raise that window to the
	// top of the desktop's z-order (Window.HandleEvent → MakeFirst) and
	// consume the event. That buries our topmost click-listener, so the
	// NEXT click reaches the window first and never reaches us — focus
	// would change exactly once and then freeze. The window's raise
	// happens later in THIS same dispatch (it sits under us in z-order),
	// so re-raising now would be undone immediately; schedule it for the
	// next idle pass instead. drainCallbacks runs before the next event is
	// handled, restoring the listener to the top in time for that click.
	// raiseMouseListener is a no-op when already topmost, so this is cheap
	// on clicks that didn't raise anything (empty desktop, same window).
	defer views.CallSoon(m.raiseMouseListener)

	ws := m.windowAtPoint(ev.Where)
	if ws == nil {
		if fw := m.fileWindowAtPoint(ev.Where); fw != nil {
			m.focusWindowView(fw.Frame.Self())
			return false
		}
		if debug.Mouse() {
			debug.Logf("mouse", "handleMouseDown: no fvmux window under cursor, falling through")
		}
		return false
	}
	leaf := findLeafAtPoint(ws, ev.Where)
	if leaf == nil {
		if debug.Mouse() {
			debug.Logf("mouse",
				"handleMouseDown: no leaf at (%d,%d) — likely a splitter or border, falling through",
				ev.Where.X, ev.Where.Y)
		}
		return false
	}

	switch {
	case ev.Buttons&consts.MbLeftButton != 0:
		m.focusWindowView(ws.Frame.Self())
		if leaf != ws.Focus && leaf.Pane != nil {
			m.setPaneFocus(ws, leaf)
		}
		if debug.Mouse() {
			debug.Logf("mouse", "handleMouseDown: LEFT, falling through")
		}
		return false // let the click reach the terminal too.
	case ev.Buttons&consts.MbRightButton != 0:
		// The consumed right-click never reaches fv-go's window raise,
		// so focus the clicked window explicitly — the context-menu
		// actions dispatch against the *current* window and would
		// otherwise target the occluded one beneath.
		m.focusWindowView(ws.Frame.Self())
		// Bring focus to the right-clicked pane before showing the menu.
		if leaf != ws.Focus && leaf.Pane != nil {
			m.setPaneFocus(ws, leaf)
		}
		m.showPaneContextMenu(ev.Where)
		return true
	}
	if debug.Mouse() {
		debug.Logf("mouse", "handleMouseDown: unhandled buttons=%#02x", ev.Buttons)
	}
	return false
}

func (m *Mux) fileWindowAtPoint(p geom.Point) *fileWindowState {
	desk := &m.App.Desktop.Group
	for i := len(desk.Children) - 1; i >= 0; i-- {
		c := desk.Children[i]
		if c == views.View(m.mouseView) || c == m.App.Desktop.Background {
			continue
		}
		bv := c.BaseView()
		if bv.Size.X <= 0 || bv.Size.Y <= 0 {
			continue
		}
		r := geom.NewRect(bv.Origin.X, bv.Origin.Y, bv.Origin.X+bv.Size.X, bv.Origin.Y+bv.Size.Y)
		if !r.Contains(p) {
			continue
		}
		return m.fileWindows[c]
	}
	return nil
}

// findLeafAtPoint returns the leaf whose cached LastRect (in desktop
// coordinates) contains p, or nil if none. Pane.LastRect is set by
// layout.Materialize on every rerender — covers the common case where
// the user clicked into a split.
func findLeafAtPoint(ws *windowState, p geom.Point) *layout.PaneNode {
	if ws == nil || ws.Root == nil {
		return nil
	}
	frameOrigin := ws.Frame.BaseView().Origin
	// LastRect is window-local (set inside Materialize against the
	// window interior). Translate the global click to window-local.
	local := geom.Point{X: p.X - frameOrigin.X, Y: p.Y - frameOrigin.Y}
	if ws.Zoomed != nil {
		// LastRect intentionally retains each pane's unzoomed geometry for
		// directional navigation. A zoomed pane nevertheless owns the whole
		// visible interior for mouse focus.
		if visible := ws.Root.FindByID(*ws.Zoomed); visible != nil &&
			windowInterior(ws.Frame).Contains(local) {
			return visible
		}
		return nil
	}
	var hit *layout.PaneNode
	ws.Root.Leaves(func(l *layout.PaneNode) {
		if l.Pane == nil {
			return
		}
		if l.Pane.LastRect.Contains(local) {
			hit = l
		}
	})
	return hit
}

// windowAtPoint returns the fvmux window whose frame is topmost under
// p, or nil when p isn't over any fvmux window — including when a
// dialog or Files window covers that spot. Click handling must
// target the window the user sees under the cursor, not the focused
// window: with overlapping windows those differ, and pane actions
// would silently hit an occluded pane in the window beneath.
func (m *Mux) windowAtPoint(p geom.Point) *windowState {
	desk := &m.App.Desktop.Group
	for i := len(desk.Children) - 1; i >= 0; i-- {
		c := desk.Children[i]
		if c == views.View(m.mouseView) || c == m.App.Desktop.Background {
			continue
		}
		bv := c.BaseView()
		if bv.Size.X <= 0 || bv.Size.Y <= 0 {
			continue // invisible OfPreProcess helpers
		}
		r := geom.NewRect(bv.Origin.X, bv.Origin.Y,
			bv.Origin.X+bv.Size.X, bv.Origin.Y+bv.Size.Y)
		if !r.Contains(p) {
			continue
		}
		if ws := m.windows[c]; ws != nil {
			return ws
		}
		return nil // a dialog or other non-window view owns this point
	}
	return nil
}

// paneContextRows is the fixed right-click menu layout, as registry command
// IDs in display order; 0 marks a "---" separator. Labels and chords are
// pulled from the registry at open time (see paneContextMenuItems) so a
// prefix rebind or keybindings.toml override shows the live chord. Dispatch
// (showPaneContextMenu) keeps the pane-targeted m.doSplit / m.doZoom / …
// methods, which act on the clicked pane rather than the focused one the
// command actions would hit.
var paneContextRows = []uint16{
	commands.CmdSplitH,
	commands.CmdSplitV,
	commands.CmdZoomPane,
	commands.CmdRenamePane,
	commands.CmdSFTPHere,
	commands.CmdSFTPNewHere,
	0,
	commands.CmdSendSIGINT,
	commands.CmdSendSIGQUIT,
	commands.CmdSendEOF,
	commands.CmdSendSIGTERM,
	0,
	commands.CmdRespawnPane,
	commands.CmdClosePane,
}

// paneContextMenuItems renders paneContextRows into popupmenu labels. Each
// registry-backed row shows its command Name with " (<chord>)" appended
// when a chord is bound; separators pass through as "---".
func paneContextMenuItems(reg *commands.Registry) []string {
	items := make([]string, len(paneContextRows))
	for i, id := range paneContextRows {
		if id == 0 {
			items[i] = "---"
			continue
		}
		c := reg.ByID(id)
		if c == nil {
			continue
		}
		label := c.Name
		if c.Chord != "" {
			label += " (" + c.Chord + ")"
		}
		if c.Enabled != nil && !c.Enabled(nil) {
			label += " [disabled]"
		}
		items[i] = label
	}
	return items
}

// showPaneContextMenu opens a popupmenu at the click position with the
// per-pane actions the right-click handler exposes. The chosen index maps
// back through paneContextRows to a command ID, so selecting a separator
// (id 0) is a no-op.
func (m *Mux) showPaneContextMenu(origin geom.Point) {
	items := paneContextMenuItems(m.Reg)
	idx := popupmenu.New(origin, items, 46).Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(paneContextRows) {
		return
	}
	id := paneContextRows[idx]
	if c := m.Reg.ByID(id); c != nil && c.Enabled != nil && !c.Enabled(nil) {
		m.showUnavailableCommand(c)
		return
	}
	switch id {
	case commands.CmdSplitH:
		m.doSplit(false)
	case commands.CmdSplitV:
		m.doSplit(true)
	case commands.CmdZoomPane:
		m.doZoom()
	case commands.CmdRenamePane:
		m.renamePane()
	case commands.CmdSFTPHere:
		m.openFilesHere(false)
	case commands.CmdSFTPNewHere:
		m.openFilesHere(true)
	case commands.CmdSendSIGINT:
		m.sendSIGINT()
	case commands.CmdSendSIGQUIT:
		m.sendSIGQUIT()
	case commands.CmdSendEOF:
		m.sendEOF()
	case commands.CmdSendSIGTERM:
		m.sendSIGTERM()
	case commands.CmdRespawnPane:
		m.respawnPane()
	case commands.CmdClosePane:
		m.doClose()
	}
}
