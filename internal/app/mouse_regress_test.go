package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/session"
)

// TestFindLeafAtPoint_ZoomedExcludesHidden is the regression for finding
// #10. In a zoomed window only the zoomed pane is on screen and only its
// LastRect is refreshed by Materialize; the hidden panes keep stale rects
// covering the same interior. findLeafAtPoint must skip every leaf but the
// zoomed one, otherwise a click resolves to an invisible pane (and Ctrl-G x
// would kill a process the user can't see).
func TestFindLeafAtPoint_ZoomedExcludesHidden(t *testing.T) {
	// Window at a non-zero desktop origin so we also exercise the
	// global→window-local translation findLeafAtPoint performs.
	w := views.NewWindow(geom.NewRect(10, 5, 90, 29), "z", 1) // 80x24 @ (10,5)
	frameOrigin := w.BaseView().Origin

	// paneA is the zoomed pane: Materialize would have written it the FULL
	// interior. paneB keeps a stale right-half rect from before the zoom.
	paneA := &session.Pane{ID: session.NewPaneID(), LastRect: geom.NewRect(1, 1, 79, 23)}
	paneB := &session.Pane{ID: session.NewPaneID(), LastRect: geom.NewRect(40, 1, 79, 23)}
	leafA, leafB := layout.Leaf(paneA), layout.Leaf(paneB)
	root := layout.Split(views.SplitVertical, leafA, leafB)

	zid := paneA.ID
	ws := &windowState{Frame: w, Root: root, Focus: leafA, Zoomed: &zid}

	// A click deep in the right half. In window-local coords this lands
	// inside BOTH rects — document the trap that made this a bug: without
	// the zoom exclusion a last-match-wins walk would silently pick paneB.
	local := geom.Point{X: 60, Y: 12}
	if !paneB.LastRect.Contains(local) {
		t.Fatalf("test setup: stale paneB rect should contain %v so the "+
			"exclusion is actually load-bearing", local)
	}
	globalRight := geom.Point{X: frameOrigin.X + local.X, Y: frameOrigin.Y + local.Y}
	if got := findLeafAtPoint(ws, globalRight); got != leafA {
		t.Fatalf("right-half click in zoomed window resolved to %v; want the "+
			"zoomed leaf A (not the hidden B)", got)
	}

	// A point outside the window interior (on the frame border, local 0,0)
	// hits nothing.
	if got := findLeafAtPoint(ws, frameOrigin); got != nil {
		t.Fatalf("click on the window border resolved to %v; want nil", got)
	}
}

// registerWindowAt builds a bare window at r, registers it with m (so it
// lands both on the desktop z-order and in m.windows), and returns the
// state. windowAtPoint reads only the frame geometry and the registry, so
// no pane tree is needed.
func registerWindowAt(m *Mux, r geom.Rect, n int) *windowState {
	w := views.NewWindow(r, "w", n)
	ws := &windowState{ID: session.NewWindowID(), Number: n, Frame: w}
	m.registerWindow(w, ws)
	return ws
}

// TestWindowAtPoint_TopmostWinsOverFocused is the regression for finding
// #11. handleMouseDown previously used currentWindow() (the *focused*
// window) to resolve a click. With overlapping windows the focused window
// and the topmost window under the cursor differ, so a click on the visible
// (topmost) window drove pane actions against the occluded focused one.
// windowAtPoint must return the topmost fvmux window under the point,
// independent of desktop focus.
func TestWindowAtPoint_TopmostWinsOverFocused(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}

	// Bottom first, then top (later insert == higher z-order). They overlap
	// in x∈[20,40).
	wsBottom := registerWindowAt(m, geom.NewRect(0, 0, 40, 20), 1)
	wsTop := registerWindowAt(m, geom.NewRect(20, 0, 60, 20), 2)

	// Focus the OTHER (bottom) window to reproduce the bug's precondition:
	// the focused window is not the one under the cursor.
	desk.Focus(wsBottom.Frame.Self())
	if m.currentWindow() != wsBottom {
		t.Fatalf("precondition: bottom window should be focused, got %v", m.currentWindow())
	}

	// A point in the overlap must resolve to the topmost window, not the
	// focused one.
	if got := m.windowAtPoint(geom.Point{X: 30, Y: 10}); got != wsTop {
		t.Fatalf("overlap click resolved to %v; want the topmost window "+
			"(currentWindow would have wrongly returned the focused bottom one)", got)
	}
	// A point over only the bottom window resolves to it.
	if got := m.windowAtPoint(geom.Point{X: 5, Y: 10}); got != wsBottom {
		t.Fatalf("bottom-only click resolved to %v; want the bottom window", got)
	}
	// A point over neither window is nil.
	if got := m.windowAtPoint(geom.Point{X: 70, Y: 70}); got != nil {
		t.Fatalf("click over empty desktop resolved to %v; want nil", got)
	}
}

// TestWindowAtPoint_DialogReturnsNil confirms that a non-window view
// (dialog/picker) covering the click makes windowAtPoint return nil rather
// than falling through to a window beneath it — click actions must not
// reach into a window occluded by a modal surface.
func TestWindowAtPoint_DialogReturnsNil(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	ws := registerWindowAt(m, geom.NewRect(0, 0, 40, 20), 1)

	// Without a dialog, a point over the window resolves to it.
	if got := m.windowAtPoint(geom.Point{X: 10, Y: 10}); got != ws {
		t.Fatalf("precondition: point over the lone window resolved to %v; want it", got)
	}

	// Drop a sized non-window view on top (topmost) covering that point.
	dialog := views.NewBackground(geom.NewRect(0, 0, 40, 20), '#')
	desk.Insert(dialog)
	if got := m.windowAtPoint(geom.Point{X: 10, Y: 10}); got != nil {
		t.Fatalf("point under a covering dialog resolved to %v; want nil", got)
	}
}
