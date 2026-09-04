package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/session"
)

func newNestedRemovalMux(t *testing.T) (*Mux, *windowState, map[string]*layout.PaneNode) {
	t.Helper()
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 160, 60))
	cfg := config.Defaults()
	cfg.General.ConfirmKill = false
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	m.Opts.Config = cfg
	w := views.NewWindow(geom.NewRect(0, 0, 120, 40), "nested", 1)
	a, b, c := termPane(), termPane(), termPane()
	a.Title, b.Title, c.Title = "A", "B", "C"
	leafA, leafB, leafC := layout.Leaf(a), layout.Leaf(b), layout.Leaf(c)
	inner := layout.Split(views.SplitHorizontal, leafB, leafC)
	root := layout.Split(views.SplitVertical, leafA, inner)
	ws := &windowState{
		ID: session.NewWindowID(), Number: 1, Title: "nested", Frame: w, Root: root,
	}
	w.Insert(layout.Materialize(root, windowInterior(w), nil))
	m.registerWindow(w, ws)
	if !m.setPaneFocus(ws, leafB) {
		t.Fatal("could not focus nested pane B")
	}
	return m, ws, map[string]*layout.PaneNode{"A": leafA, "B": leafB, "C": leafC}
}

func assertLiveFocus(t *testing.T, ws *windowState, want *layout.PaneNode) {
	t.Helper()
	if ws.Focus != want {
		t.Fatalf("focus = %v, want closest sibling %v", ws.Focus, want)
	}
	if ws.Root == nil || want.Pane == nil || ws.Root.FindByID(want.Pane.ID) != want {
		t.Fatal("focused pane is not a live leaf in ws.Root")
	}
}

func TestManualCloseFocusesImmediateSibling(t *testing.T) {
	m, ws, leaves := newNestedRemovalMux(t)
	m.doClose()
	assertLiveFocus(t, ws, leaves["C"])
}

func TestAutoCloseFocusesImmediateSibling(t *testing.T) {
	m, ws, leaves := newNestedRemovalMux(t)
	m.AutoClosePane(leaves["B"].Pane)
	assertLiveFocus(t, ws, leaves["C"])
}

func TestBreakOutFocusesImmediateSiblingInSource(t *testing.T) {
	m, ws, leaves := newNestedRemovalMux(t)
	m.doBreakOut()
	assertLiveFocus(t, ws, leaves["C"])
	if len(m.windowOrder) != 2 {
		t.Fatalf("break-out windows = %d, want source plus detached window", len(m.windowOrder))
	}
	if got := ws.Root.FindByID(leaves["B"].Pane.ID); got != nil {
		t.Fatal("broken-out pane still exists in source tree")
	}
	detached := m.windows[m.windowOrder[len(m.windowOrder)-1]]
	if detached == nil {
		t.Fatal("detached window state missing")
	}
	if got, want := leaves["B"].Pane.Term.BaseView().GetBounds(), windowInterior(detached.Frame); got != want {
		t.Fatalf("detached pane bounds = %v, want new window interior %v", got, want)
	}
}

func TestBreakOutTwoPaneLayoutFillsBothWindows(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 160, 60))
	cfg := config.Defaults()
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	m.Opts.Config = cfg
	w := views.NewWindow(geom.NewRect(0, 0, 120, 40), "pair", 1)
	a, b := termPane(), termPane()
	left, right := layout.Leaf(a), layout.Leaf(b)
	root := layout.Split(views.SplitVertical, left, right)
	ws := &windowState{ID: session.NewWindowID(), Number: 1, Title: "pair", Frame: w, Root: root}
	w.Insert(layout.Materialize(root, windowInterior(w), nil))
	m.registerWindow(w, ws)
	m.setPaneFocus(ws, right)

	m.doBreakOut()

	if got, want := a.Term.BaseView().GetBounds(), windowInterior(ws.Frame); got != want {
		t.Fatalf("source survivor bounds = %v, want full window interior %v", got, want)
	}
	detached := m.windows[m.windowOrder[len(m.windowOrder)-1]]
	if detached == nil {
		t.Fatal("detached window state missing")
	}
	if got, want := b.Term.BaseView().GetBounds(), windowInterior(detached.Frame); got != want {
		t.Fatalf("detached pane bounds = %v, want new window interior %v", got, want)
	}
}
