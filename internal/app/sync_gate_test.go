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

// TestBroadcastIfSync_Gates is the regression for finding #12: resize mode
// and copy mode now short-circuit broadcastIfSync so their navigation keys
// (l/h/j/k, arrows, Esc) aren't mirrored into every synced pane before the
// later listener in the OfPreProcess chain consumes them.
//
// Honesty note: the *suppressed write* is not directly observable headless.
// The broadcast reaches other panes only via terminal.HandleEvent, which is
// a no-op on an un-Started terminal (t.pty == nil → early return), and a
// real PTY's echo would be racy to observe. So this test pins the two
// things that ARE cleanly checkable and that a regression would break:
//
//   - the gate is consulted with a fully-populated synced window whose focus
//     and pane tree are non-nil, and never panics on any of the three paths
//     (resize-gated, copy-gated, ungated walk);
//   - m.copyMode.Active() is nil-safe inside the gate (m.copyMode == nil must
//     not deref);
//   - the original event is never mutated on any path.
func TestBroadcastIfSync_Gates(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}

	// A synced current window with focus and two real (un-Started) panes —
	// the shape broadcastIfSync would actually iterate over.
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

	newKey := func() drivers.Event {
		return drivers.Event{What: consts.EvKeyDown, UnicodeChar: 'x'}
	}
	assertUntouched := func(t *testing.T, ev drivers.Event) {
		t.Helper()
		if ev.What != consts.EvKeyDown || ev.UnicodeChar != 'x' {
			t.Fatalf("broadcastIfSync mutated the original event: %+v", ev)
		}
	}

	// Ungated: not in resize mode, m.copyMode is nil. The gate must treat a
	// nil copyMode as inactive (Active() nil-safe) and fall through to the
	// leaf walk without panicking. The un-Started terminals no-op.
	t.Run("ungated_nil_copymode", func(t *testing.T) {
		m.resizeMode = false
		m.copyMode = nil
		ev := newKey()
		m.broadcastIfSync(&ev)
		assertUntouched(t, ev)
	})

	// Resize-mode gate: must early-return before the leaf walk.
	t.Run("resize_mode_gate", func(t *testing.T) {
		m.resizeMode = true
		m.copyMode = nil
		ev := newKey()
		m.broadcastIfSync(&ev)
		assertUntouched(t, ev)
		m.resizeMode = false
	})

	// Copy-mode gate: an active driver must early-return the broadcast.
	t.Run("copy_mode_gate", func(t *testing.T) {
		m.resizeMode = false
		drv := copymode.Show(m.App, terminal.New(geom.NewRect(0, 0, 40, 12)))
		if !drv.Active() {
			t.Fatal("precondition: copy-mode driver should be active")
		}
		m.copyMode = drv
		ev := newKey()
		m.broadcastIfSync(&ev)
		assertUntouched(t, ev)
		drv.Close()
		m.copyMode = nil
	})
}
