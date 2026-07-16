package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/copymode"
	"github.com/oldwired/fvmux/internal/layout"
)

// TestBroadcastIfSync_Gates is the regression for finding #12: resize mode and
// copy mode short-circuit broadcastIfSync so their navigation keys (l/h/j/k,
// arrows, Esc) aren't mirrored into every synced pane before the later
// listener in the OfPreProcess chain consumes them.
//
// The suppressed write is not observable on an un-Started terminal
// (terminal.HandleEvent no-ops when its PTY is nil), so this test injects the
// m.syncSend seam: a recorder that captures exactly which pane terminals the
// gate delivers to, intercepting before terminal.HandleEvent. Deleting either
// gate now flips a recorded count from 0 to non-zero — so the test genuinely
// pins the gate instead of merely exercising it.
func TestBroadcastIfSync_Gates(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}

	// A synced current window: focus on leafA, with leafB the non-focused
	// sibling. broadcastIfSync should reach leafB's terminal only — never the
	// focused pane, never a pane twice.
	a, b := termPane(), termPane()
	leafA, leafB := layout.Leaf(a), layout.Leaf(b)
	root := layout.Split(views.SplitVertical, leafA, leafB)
	ws := &windowState{
		Frame:     views.NewWindow(geom.NewRect(0, 0, 40, 20), "sync", 1),
		Root:      root,
		Focus:     leafA,
		SyncInput: true,
	}
	m.registerWindow(ws.Frame, ws)
	desk.Focus(ws.Frame.Self())
	if m.currentWindow() != ws {
		t.Fatalf("precondition: synced window should be current, got %v", m.currentWindow())
	}

	// Recorder seam: capture every terminal the gate would deliver to.
	var got []*terminal.Terminal
	m.syncSend = func(_ *drivers.Event, term *terminal.Terminal) {
		got = append(got, term)
	}

	newKey := func() drivers.Event {
		return drivers.Event{What: consts.EvKeyDown, UnicodeChar: 'x'}
	}
	assertUntouched := func(t *testing.T, ev drivers.Event) {
		t.Helper()
		if ev.What != consts.EvKeyDown || ev.UnicodeChar != 'x' {
			t.Fatalf("broadcastIfSync mutated the original event: %+v", ev)
		}
	}

	// (a) Ungated: exactly the one non-focused pane's terminal is delivered.
	t.Run("ungated_delivers_to_nonfocused_only", func(t *testing.T) {
		got = nil
		m.resizeMode = false
		m.copyMode = nil
		ev := newKey()
		m.broadcastIfSync(&ev)
		assertUntouched(t, ev)
		if len(got) != 1 {
			t.Fatalf("ungated broadcast reached %d terminals, want exactly 1", len(got))
		}
		if got[0] != b.Term {
			t.Error("ungated broadcast reached the wrong terminal; want the non-focused pane's")
		}
		if got[0] == a.Term {
			t.Error("broadcast must never echo into the focused pane")
		}
	})

	// (b) Resize-mode gate: zero deliveries.
	t.Run("resize_mode_gate_suppresses", func(t *testing.T) {
		got = nil
		m.resizeMode = true
		m.copyMode = nil
		ev := newKey()
		m.broadcastIfSync(&ev)
		assertUntouched(t, ev)
		if len(got) != 0 {
			t.Fatalf("resize mode must suppress the broadcast; reached %d terminals", len(got))
		}
		m.resizeMode = false
	})

	// (c) Copy-mode gate: zero deliveries while a driver is active.
	t.Run("copy_mode_gate_suppresses", func(t *testing.T) {
		got = nil
		m.resizeMode = false
		drv := copymode.Show(m.App, terminal.New(geom.NewRect(0, 0, 40, 12)))
		if !drv.Active() {
			t.Fatal("precondition: copy-mode driver should be active")
		}
		m.copyMode = drv
		ev := newKey()
		m.broadcastIfSync(&ev)
		assertUntouched(t, ev)
		if len(got) != 0 {
			t.Fatalf("active copy mode must suppress the broadcast; reached %d terminals", len(got))
		}
		drv.Close()
		m.copyMode = nil
	})

	// Nil-safety: m.copyMode == nil must be treated as inactive (Active() is
	// nil-safe) and the ungated walk must proceed without panicking.
	t.Run("nil_copymode_is_inactive", func(t *testing.T) {
		got = nil
		m.resizeMode = false
		m.copyMode = nil
		ev := newKey()
		m.broadcastIfSync(&ev) // must not panic
		if len(got) != 1 {
			t.Fatalf("nil copyMode should be treated as inactive; reached %d terminals, want 1", len(got))
		}
	})
}
