package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/prefix"
	"github.com/oldwired/fvmux/internal/session"
)

// TestPaneContextMenuLabelsTrackRegistry asserts the right-click menu sources
// its chords from the registry rather than hardcoding them, so a prefix
// rebind or keybindings.toml override is reflected in the labels.
func TestPaneContextMenuLabelsTrackRegistry(t *testing.T) {
	reg := commands.Defaults()

	items := paneContextMenuItems(reg)
	if !hasItem(items, "Split Left/Right (C-g %)") {
		t.Fatalf("baseline menu missing default split chord: %v", items)
	}
	if !hasItem(items, "Kill Pane (C-g x)") {
		t.Fatalf("baseline menu missing default kill chord: %v", items)
	}
	// Separator preserved and signal rows (no chord) render bare.
	if !hasItem(items, "---") {
		t.Errorf("menu dropped its separator: %v", items)
	}
	if !hasItem(items, "Send SIGTERM") {
		t.Errorf("menu dropped the plain Send SIGTERM row: %v", items)
	}

	reg.RebindPrefix("C-g", "C-b")
	items = paneContextMenuItems(reg)
	if !hasItem(items, "Split Left/Right (C-b %)") {
		t.Errorf("after rebind, split chord not updated: %v", items)
	}
	if !hasItem(items, "Kill Pane (C-b x)") {
		t.Errorf("after rebind, kill chord not updated: %v", items)
	}
	if hasItem(items, "Split Left/Right (C-g %)") {
		t.Errorf("after rebind, stale C-g chord still shown: %v", items)
	}
	if len(paneContextRows) < 4 || paneContextRows[3] != commands.CmdRenamePane {
		t.Fatalf("pane context rename row = %v, want CmdRenamePane", paneContextRows)
	}
	for _, id := range paneContextRows {
		if id == commands.CmdRenameWindow {
			t.Fatalf("pane context menu still contains CmdRenameWindow: %v", paneContextRows)
		}
	}
}

func hasItem(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}

// termPane is newTestPane with a real (PTY-less) Terminal attached, which
// layout.Materialize needs in order to wrap each leaf.
func termPane() *session.Pane {
	return &session.Pane{ID: session.NewPaneID(), Term: terminal.New(geom.Rect{})}
}

// at builds a global click event location at (x, y).
func at(x, y int) geom.Point { return geom.Point{X: x, Y: y} }

// TestClickReRaisesMouseListener is the regression for focus freezing
// after exactly one click-change. fv-go raises a clicked window to the
// top of the desktop's z-order (Window.HandleEvent → MakeFirst) and
// consumes the event, which buries our topmost MouseView so the NEXT
// click never reaches it. handleMouseDown must schedule a re-raise (run
// on the next idle pass) that restores the listener to the top.
func TestClickReRaisesMouseListener(t *testing.T) {
	// Capture CallSoon so the deferred re-raise can be run deterministically;
	// the real Program drains these before handling the next event.
	var scheduled []func()
	views.SetCallSoon(func(fn func()) { scheduled = append(scheduled, fn) })
	defer views.SetCallSoon(nil)

	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	mv := prefix.NewMouseView(geom.NewRect(0, 0, 80, 24), nil)
	desk.Insert(mv) // children: [Background, mv] — listener topmost
	m := &Mux{
		App:       &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		mouseView: mv,
	}

	// A click raised a window above the listener: [Background, mv, win].
	win := views.NewWindow(geom.NewRect(0, 0, 20, 10), "w", 1)
	desk.Insert(win)
	if last := desk.Children[len(desk.Children)-1]; last != views.View(win) {
		t.Fatalf("precondition: window should be topmost, got %T", last)
	}

	// The next click. currentWindow() returns nil (win isn't registered),
	// so handleMouseDown early-returns — but its deferred re-raise must
	// still be scheduled regardless of the path taken.
	ev := drivers.Event{
		What:    consts.EvMouseDown,
		Buttons: consts.MbLeftButton,
		Where:   geom.Point{X: 5, Y: 5},
	}
	m.handleMouseDown(&ev)

	if len(scheduled) == 0 {
		t.Fatal("handleMouseDown scheduled no re-raise — the MouseView stays buried and focus freezes")
	}
	for _, fn := range scheduled {
		fn()
	}

	if last := desk.Children[len(desk.Children)-1]; last != views.View(mv) {
		t.Fatalf("after re-raise the MouseView should be topmost again, got %T", last)
	}
}

// TestFindLeafFollowsWindowResize is the regression for click-to-focus
// breaking after a window is resized. A mouse-drag resize stretches the
// panes live but does not rebuild the tree, so Pane.LastRect — the click
// hit-map, written only by Materialize — goes stale and clicks in the
// grown region stop landing on a pane. OnResize now rerenders (which
// re-Materializes), resyncing the hit-map. This test drives that flow
// directly: Materialize, resize, re-Materialize, and check hit-testing.
func TestFindLeafFollowsWindowResize(t *testing.T) {
	// Window placed at a non-zero desktop origin so the test also covers
	// findLeafAtPoint's global→window-local translation.
	w := views.NewWindow(geom.NewRect(5, 3, 45, 23), "t", 1) // 40x20

	a, b := termPane(), termPane()
	ra := layout.Leaf(a)
	root := layout.SplitH(ra, ra, b) // vertical split: a | b, side by side
	ws := &windowState{Frame: w, Root: root, Focus: ra}

	// Initial materialize into the small interior + insert (what window
	// creation does).
	body := layout.Materialize(root, windowInterior(w), nil)
	w.Insert(body)

	fox, foy := w.BaseView().Origin.X, w.BaseView().Origin.Y
	leafOf := func(pane *session.Pane) *layout.PaneNode {
		var n *layout.PaneNode
		root.Leaves(func(l *layout.PaneNode) {
			if l.Pane == pane {
				n = l
			}
		})
		return n
	}

	// Sanity: clicks partition into the two panes while small. Probe just
	// inside each half so we never land on the one-cell splitter gap.
	if got := findLeafAtPoint(ws, at(fox+5, foy+10)); got != leafOf(a) {
		t.Fatalf("left-half click resolved to %v, want pane a", got)
	}
	if got := findLeafAtPoint(ws, at(fox+34, foy+10)); got != leafOf(b) {
		t.Fatalf("right-half click resolved to %v, want pane b", got)
	}

	// Grow the window the way a resize-drag does. The body stretches via
	// GrowMode, but LastRect is now stale.
	w.ChangeBounds(geom.NewRect(5, 3, 85, 43)) // 80x40
	farRight := at(fox+72, foy+36)             // deep in the new bottom-right region
	if got := findLeafAtPoint(ws, farRight); got != nil {
		t.Fatalf("pre-rerender: stale hit-map should miss the grown region, got %v", got)
	}

	// OnResize → rerender re-Materializes at the new interior, refreshing
	// every LastRect. Reproduce that step.
	layout.Materialize(root, windowInterior(w), nil)

	if got := findLeafAtPoint(ws, farRight); got != leafOf(b) {
		t.Fatalf("post-rerender: grown bottom-right should focus pane b, got %v", got)
	}
	// Left half still resolves to a after the grow.
	if got := findLeafAtPoint(ws, at(fox+5, foy+20)); got != leafOf(a) {
		t.Fatalf("post-rerender: left-half click resolved to %v, want pane a", got)
	}
}
