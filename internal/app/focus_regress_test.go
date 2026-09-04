package app

import (
	"strings"
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/session"
)

func newFocusMux(t *testing.T) (*Mux, *windowState, *layout.PaneNode, *layout.PaneNode) {
	t.Helper()
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	w := views.NewWindow(geom.NewRect(0, 0, 80, 24), "focus", 1)
	a, b := termPane(), termPane()
	leafA := layout.Leaf(a)
	root := layout.SplitH(leafA, leafA, b)
	leafB := root.FindByID(b.ID)
	ws := &windowState{ID: session.NewWindowID(), Number: 1, Frame: w, Root: root}
	w.Insert(layout.Materialize(root, windowInterior(w), nil))
	m.registerWindow(w, ws)
	if !m.setPaneFocus(ws, leafA) {
		t.Fatal("could not establish initial pane focus")
	}
	return m, ws, leafA, leafB
}

func assertVisibleFocus(t *testing.T, ws *windowState, want *layout.PaneNode) {
	t.Helper()
	if ws.Focus != want {
		t.Fatalf("logical focus = %v, want %v", ws.Focus, want)
	}
	if ws.Zoomed == nil || want.Pane == nil || *ws.Zoomed != want.Pane.ID {
		t.Fatalf("zoomed pane = %v, want focused pane %v", ws.Zoomed, want.Pane.ID)
	}
}

func TestZoomedPaneNavigationKeepsVisibleAndLogicalFocusTogether(t *testing.T) {
	m, ws, leafA, leafB := newFocusMux(t)
	m.doZoom()
	assertVisibleFocus(t, ws, leafA)

	m.focusNextPaneInTree(+1)
	assertVisibleFocus(t, ws, leafB)
	m.focusNextPaneInTree(-1)
	assertVisibleFocus(t, ws, leafA)

	m.doFocusDir(layout.Right)
	assertVisibleFocus(t, ws, leafB)
	m.doFocusDir(layout.Left)
	assertVisibleFocus(t, ws, leafA)
}

func TestSplitWhileZoomedUnzoomsAndFocusesVisibleNewPane(t *testing.T) {
	m := newCatMux(t)
	w, err := m.openWindowFromProfile(m.Opts.Profiles[0])
	if err != nil {
		t.Fatalf("openWindowFromProfile: %v", err)
	}
	ws := m.windows[w.Self()]
	original := ws.Focus
	m.doZoom()
	assertVisibleFocus(t, ws, original)

	m.doSplitWith(m.Opts.Profiles[0], false)
	if ws.Zoomed != nil {
		t.Fatalf("split left window zoomed on pane %v; want both panes visible", *ws.Zoomed)
	}
	if ws.Focus == nil || ws.Focus == original || ws.Root.FindByID(ws.Focus.Pane.ID) != ws.Focus {
		t.Fatalf("split focus = %v, want the live new sibling", ws.Focus)
	}
	if got := len(ws.Root.CollectLeaves()); got != 2 {
		t.Fatalf("split produced %d leaves, want 2", got)
	}
}

func TestPaneTargetCommandsStayOnVisibleFocus(t *testing.T) {
	m, ws, visible, hidden := newFocusMux(t)
	m.doZoom()
	m.focusNextPaneInTree(+1)
	visible, hidden = hidden, visible
	assertVisibleFocus(t, ws, visible)

	// Paste and all signal helpers select their target via FocusedTerminal.
	// Capture paste's target without touching the process-global clipboard.
	var pasteTarget *terminal.Terminal
	m.pasteTo = func(target *terminal.Terminal) error {
		pasteTarget = target
		return nil
	}
	m.pasteClipboard()
	if pasteTarget != visible.Pane.Term {
		t.Fatalf("paste target = %p, want visible terminal %p", pasteTarget, visible.Pane.Term)
	}

	// sendBytes is the shared path for SIGINT/SIGQUIT/EOF. PTY-less test
	// terminals render injected bytes, making the selected target observable.
	m.sendBytes('V')
	if got := visible.Pane.Term.ScrollbackText(); !strings.Contains(got, "V") {
		t.Fatalf("visible signal target did not receive byte; scrollback=%q", got)
	}
	if got := hidden.Pane.Term.ScrollbackText(); strings.Contains(got, "V") {
		t.Fatalf("hidden pane received signal byte; scrollback=%q", got)
	}
	assertVisibleFocus(t, ws, visible)

	// Kill follows that same synchronized target.
	m.doClose()
	if ws.Root.FindByID(visible.Pane.ID) != nil {
		t.Fatal("kill pane left the visible target in the tree")
	}
	if ws.Root.FindByID(hidden.Pane.ID) == nil {
		t.Fatal("kill pane removed the hidden logical target")
	}
}

func TestPaneTargetCommandsFailClosedOnHiddenFocus(t *testing.T) {
	m, ws, visible, hidden := newFocusMux(t)
	zid := visible.Pane.ID
	ws.Zoomed = &zid
	ws.Focus = hidden
	if got := m.FocusedTerminal(); got != nil {
		t.Fatalf("FocusedTerminal returned hidden target %p; want nil", got)
	}
	before := len(ws.Root.CollectLeaves())
	m.doClose()
	if got := len(ws.Root.CollectLeaves()); got != before {
		t.Fatalf("doClose mutated inconsistent zoom state: leaves %d -> %d", before, got)
	}
	if ws.Focus != hidden || ws.Zoomed == nil || *ws.Zoomed != visible.Pane.ID {
		t.Fatal("command targeting defensively rewrote inconsistent state")
	}
}

func TestSetPaneFocusRejectsForeignLeaf(t *testing.T) {
	m, ws, visible, _ := newFocusMux(t)
	foreign := layout.Leaf(termPane())
	if m.setPaneFocus(ws, foreign) {
		t.Fatal("setPaneFocus accepted a leaf outside ws.Root")
	}
	if ws.Focus != visible {
		t.Fatalf("foreign focus changed ws.Focus to %v", ws.Focus)
	}
}

func TestResizeByMovesExactCellAndReportsMissingBoundary(t *testing.T) {
	m, ws, left, _ := newFocusMux(t)
	before := left.Pane.LastRect.Width()
	if !m.resizeBy(1, 0) {
		t.Fatal("right resize reported no movement")
	}
	if got := left.Pane.LastRect.Width(); got != before+1 {
		t.Fatalf("left pane width after one-cell resize = %d, want %d", got, before+1)
	}
	if m.resizeBy(-1, 0) {
		t.Fatal("leftmost pane unexpectedly found a divider on its left")
	}
	if m.flashText != "no movable divider to the left" {
		t.Fatalf("missing-boundary feedback = %q", m.flashText)
	}
	if ws.Focus != left {
		t.Fatal("resize changed pane focus")
	}
}
