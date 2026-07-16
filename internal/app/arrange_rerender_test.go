package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/session"
)

// TestTile_RerendersLastRect is the regression for finding #22: tileGrid /
// cascade now rerender each window after ChangeBounds. fv-go's Window
// OnResize fires only for mouse-drag resizes, never for programmatic
// ChangeBounds, so without an explicit rerender every pane's click hit-map
// (Pane.LastRect, written only by Materialize) keeps its pre-tile geometry.
// After m.tile() each pane's LastRect must be re-materialized to the window's
// new interior.
func TestTile_RerendersLastRect(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}

	// Two small single-pane windows, each materialized + registered the way
	// window creation does.
	var panes []*session.Pane
	for i := 0; i < 2; i++ {
		w := views.NewWindow(geom.NewRect(0, 0, 20, 10), "w", i+1)
		pane := termPane()
		root := layout.Leaf(pane)
		ws := &windowState{
			ID:     session.NewWindowID(),
			Number: i + 1,
			Frame:  w,
			Root:   root,
			Focus:  root,
		}
		body := layout.Materialize(root, windowInterior(w), nil)
		w.Insert(body)
		m.registerWindow(w, ws)
		panes = append(panes, pane)
	}

	// Wipe every hit-map so a non-empty result after tile can only come from
	// a fresh Materialize.
	for _, p := range panes {
		p.LastRect = geom.Rect{}
	}

	m.tile()

	// Each pane's LastRect must now match its window's new interior.
	for _, ws := range m.windows {
		var leaf *layout.PaneNode
		ws.Root.Leaves(func(l *layout.PaneNode) { leaf = l })
		if leaf == nil || leaf.Pane == nil {
			t.Fatal("window lost its pane after tile")
		}
		got := leaf.Pane.LastRect
		if got.Empty() {
			t.Errorf("pane LastRect still empty after tile — rerender did not run")
			continue
		}
		want := windowInterior(ws.Frame)
		if !got.Equals(want) {
			t.Errorf("pane LastRect = %+v after tile; want the new window "+
				"interior %+v", got, want)
		}
	}
}
