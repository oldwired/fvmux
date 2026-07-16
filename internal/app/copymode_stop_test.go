package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/copymode"
	"github.com/oldwired/fvmux/internal/session"
)

// TestStopPane_ClosesCopyMode is the regression for finding #21: stopPane
// must tear down an active copy-mode driver when the terminal it is reading
// is stopped. Otherwise the OfPreProcess driver stays installed desktop-wide,
// eating keys on behalf of a dead pane.
func TestStopPane_ClosesCopyMode(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{App: &fvapp.Application{Program: &fvapp.Program{Desktop: desk}}}

	// A pane whose terminal is the one copy mode is attached to. The
	// terminal is never Started, so no PTY is spawned; EnterCopyMode /
	// ExitCopyMode / Stop all operate on the in-memory buffer.
	term := terminal.New(geom.NewRect(0, 0, 40, 12))
	pane := &session.Pane{ID: session.NewPaneID(), Term: term}

	m.copyMode = copymode.Show(m.App, term)
	if !m.copyMode.Active() {
		t.Fatal("precondition: copy mode should be active after Show")
	}
	if m.copyMode.Term() != term {
		t.Fatal("precondition: copy mode should be attached to pane's terminal")
	}

	m.stopPane(pane)

	if m.copyMode.Active() {
		t.Fatal("stopPane did not close copy mode for the stopped terminal")
	}
}
