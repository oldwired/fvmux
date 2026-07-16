package copymode

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"
)

// TestDriver_NilSafety pins the nil-receiver guards on the Driver lifecycle
// handle (finding #21): the Mux stores the driver in a field that is nil
// whenever copy mode isn't running, and every query/teardown call site
// relies on those being safe on a nil *Driver.
func TestDriver_NilSafety(t *testing.T) {
	var d *Driver
	if d.Active() {
		t.Error("(*Driver)(nil).Active() = true; want false")
	}
	if d.Term() != nil {
		t.Error("(*Driver)(nil).Term() != nil")
	}
	d.Close() // must not panic
}

// TestDriver_ShowCloseLifecycle exercises the real lifecycle without a PTY:
// terminal.New initializes the copy buffer, so EnterCopyMode / ExitCopyMode
// work on an un-Started terminal. Show → Active; Close → !Active with OnDone
// fired exactly once; a second Close is a no-op that never refires OnDone.
func TestDriver_ShowCloseLifecycle(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	app := &fvapp.Application{Program: &fvapp.Program{Desktop: desk}}
	term := terminal.New(geom.NewRect(0, 0, 40, 12)) // never Started

	drv := Show(app, term)
	if drv == nil {
		t.Fatal("Show returned nil for a non-nil terminal")
	}
	if !drv.Active() {
		t.Fatal("driver not Active after Show")
	}
	if drv.Term() != term {
		t.Fatal("Term() did not return the terminal passed to Show")
	}

	var done int
	drv.OnDone = func() { done++ }

	drv.Close()
	if drv.Active() {
		t.Error("driver still Active after Close")
	}
	if done != 1 {
		t.Errorf("OnDone fired %d times after Close; want 1", done)
	}

	// Idempotent: a second Close must not refire OnDone.
	drv.Close()
	if done != 1 {
		t.Errorf("OnDone refired on second Close; count = %d, want 1", done)
	}
}
