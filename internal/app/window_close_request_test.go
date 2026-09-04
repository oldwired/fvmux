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

func newCloseRequestMux(t *testing.T, confirmKill bool, paneDead bool) (*Mux, *windowState) {
	t.Helper()
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	cfg := config.Defaults()
	cfg.General.ConfirmKill = confirmKill
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	m.Opts.Config = cfg
	w := views.NewWindow(geom.NewRect(0, 0, 40, 12), "close", 1)
	pane := termPane()
	pane.Dead = paneDead
	root := layout.Leaf(pane)
	ws := &windowState{
		ID: session.NewWindowID(), Number: 1, Frame: w, Root: root, Focus: root,
	}
	w.Insert(layout.Materialize(root, windowInterior(w), nil))
	m.registerWindow(w, ws)
	return m, ws
}

func TestWindowFrameCloseRequestHonorsKillConfirmation(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		m, ws := newCloseRequestMux(t, true, false)
		calls := 0
		m.confirmKillPrompt = func(message string) bool {
			calls++
			return false
		}

		ws.Frame.Close()

		if calls != 1 {
			t.Fatalf("confirmation calls = %d, want 1", calls)
		}
		if _, ok := m.windows[ws.Frame.Self()]; !ok || ws.Frame.BaseView().Owner == nil {
			t.Fatal("cancelled frame close detached or cleaned up the window")
		}
	})

	t.Run("confirmed", func(t *testing.T) {
		m, ws := newCloseRequestMux(t, true, false)
		calls := 0
		m.confirmKillPrompt = func(message string) bool {
			calls++
			return true
		}

		ws.Frame.Close()

		if calls != 1 {
			t.Fatalf("confirmation calls = %d, want 1", calls)
		}
		if _, ok := m.windows[ws.Frame.Self()]; ok || ws.Frame.BaseView().Owner != nil {
			t.Fatal("confirmed frame close left the window attached or registered")
		}
	})
}

func TestWindowFrameCloseRequestSkipsPromptWhenSafeOrDisabled(t *testing.T) {
	for _, tc := range []struct {
		name        string
		confirmKill bool
		paneDead    bool
	}{
		{name: "dead pane", confirmKill: true, paneDead: true},
		{name: "confirmation disabled", confirmKill: false, paneDead: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, ws := newCloseRequestMux(t, tc.confirmKill, tc.paneDead)
			m.confirmKillPrompt = func(string) bool {
				t.Fatal("safe/disabled close unexpectedly prompted")
				return false
			}

			ws.Frame.Close()

			if _, ok := m.windows[ws.Frame.Self()]; ok || ws.Frame.BaseView().Owner != nil {
				t.Fatal("safe/disabled frame close left the window attached or registered")
			}
		})
	}
}

func TestExplicitKillWindowDoesNotDoublePrompt(t *testing.T) {
	m, ws := newCloseRequestMux(t, true, false)
	calls := 0
	m.confirmKillPrompt = func(string) bool {
		calls++
		return true
	}

	m.killWindow()

	if calls != 1 {
		t.Fatalf("explicit kill confirmation calls = %d, want 1", calls)
	}
	if _, ok := m.windows[ws.Frame.Self()]; ok {
		t.Fatal("explicit kill left the window registered")
	}
}
