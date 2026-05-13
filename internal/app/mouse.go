package app

import (
	"fmt"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"

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
	m.windows[w.Self()] = ws
	m.windowOrder = append(m.windowOrder, w.Self())
	w.OnClose = func() { m.cleanupWindow(ws) }
	m.App.Desktop.InsertWindow(w)
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
	ws := m.currentWindow()
	if ws == nil {
		if debug.Mouse() {
			debug.Logf("mouse", "handleMouseDown: no current fvmux window, falling through")
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
		if leaf != ws.Focus && leaf.Pane != nil {
			ws.Focus = leaf
			focusTerminalPath(ws.Frame, leaf.Pane.Term)
			m.refreshStatusBar()
		}
		if debug.Mouse() {
			debug.Logf("mouse", "handleMouseDown: LEFT, falling through")
		}
		return false // let the click reach the terminal too.
	case ev.Buttons&consts.MbRightButton != 0:
		// Bring focus to the right-clicked pane before showing the menu.
		if leaf != ws.Focus && leaf.Pane != nil {
			ws.Focus = leaf
			focusTerminalPath(ws.Frame, leaf.Pane.Term)
			m.refreshStatusBar()
		}
		m.showPaneContextMenu(ev.Where)
		return true
	}
	if debug.Mouse() {
		debug.Logf("mouse", "handleMouseDown: unhandled buttons=%#02x", ev.Buttons)
	}
	return false
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

// showPaneContextMenu opens a popupmenu at the click position with
// the per-pane actions the right-click handler exposes. Indices are
// resolved against the fixed item list below.
func (m *Mux) showPaneContextMenu(origin geom.Point) {
	items := []string{
		"Split horizontal (C-g %)",
		"Split vertical (C-g \")",
		"Zoom / Unzoom (C-g z)",
		"Rename window… (C-g ,)",
		"---",
		"Send Interrupt (Ctrl-C)",
		"Send Quit (Ctrl-\\)",
		"Send EOF (Ctrl-D)",
		"Send SIGTERM",
		"---",
		"Respawn dead pane",
		"Kill pane (C-g x)",
	}
	idx := popupmenu.New(origin, items, 36).Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(items) {
		return
	}
	switch idx {
	case 0:
		m.doSplit(false)
	case 1:
		m.doSplit(true)
	case 2:
		m.doZoom()
	case 3:
		m.renameWindow()
	case 5:
		m.sendSIGINT()
	case 6:
		m.sendSIGQUIT()
	case 7:
		m.sendEOF()
	case 8:
		m.sendSIGTERM()
	case 10:
		m.respawnPane()
	case 11:
		m.doClose()
	}
}
