package app

import (
	"os"
	"path/filepath"
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
)

// TestNewSession_PersistsBeforeClosing is the regression for finding #15:
// File → New Session now calls SaveSessionSilent() on the outgoing named
// session *before* it tears the windows down. Previously the layout work
// since the last manual save was silently discarded.
//
// We build a Mux with a named session ("work") holding one live window,
// then call newSession() and assert the outgoing session was written to
// disk (with its window captured) before the name was cleared and a fresh
// starter window opened.
func TestNewSession_PersistsBeforeClosing(t *testing.T) {
	dir := t.TempDir()
	// Sandbox both roots inside the temp dir so nothing touches $HOME.
	paths := config.Paths{Root: dir, StateRoot: filepath.Join(dir, "state")}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	app := &fvapp.Application{Program: &fvapp.Program{Desktop: desk}}

	cfg := config.Defaults()
	cfg.General.ConfirmKill = false // no modal on close-all in a headless test
	cfg.General.DefaultProfile = "starter"

	m := &Mux{
		App:     app,
		windows: map[views.View]*windowState{},
	}
	m.Opts.Paths = paths
	m.Opts.Config = cfg
	// The fresh starter window that newSession opens spawns a real PTY via
	// NewWindow. Use "cat": with no input it blocks (never exits, emits no
	// output), so the terminal's wait/read goroutines never schedule callback
	// work through the process-global CallSoon hook used by other tests. The
	// child dies when the test binary exits.
	m.Opts.Profiles = []*profile.Profile{{Name: "starter", Command: "cat"}}
	m.Opts.SessionName = "work"

	// Register one live outgoing window *without* spawning a PTY (the
	// terminal is constructed but never Started), so buildSnapshot has a
	// real window to capture and there's no async process teardown racing
	// the test.
	w := views.NewWindow(geom.NewRect(0, 0, 40, 12), "outgoing", 1)
	pane := &session.Pane{
		ID:      session.NewPaneID(),
		Term:    terminal.New(geom.Rect{}),
		Title:   "outgoing",
		Profile: "starter",
	}
	root := layout.Leaf(pane)
	ws := &windowState{
		ID:     session.NewWindowID(),
		Number: 1,
		Title:  "outgoing",
		Frame:  w,
		Root:   root,
		Focus:  root,
	}
	body := layout.Materialize(root, windowInterior(w), nil)
	w.Insert(body)
	m.registerWindow(w, ws)

	sessionFile := paths.SessionFile("work")
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Fatalf("precondition: session file should not exist yet; stat err = %v", err)
	}

	m.newSession()

	// The outgoing session must have been persisted before teardown.
	if _, err := os.Stat(sessionFile); err != nil {
		t.Fatalf("newSession did not save the outgoing session: %v", err)
	}
	snap, err := session.Load(sessionFile)
	if err != nil {
		t.Fatalf("saved session unreadable: %v", err)
	}
	if snap.Name != "work" {
		t.Errorf("saved session Name = %q; want work", snap.Name)
	}
	if len(snap.Windows) != 1 {
		t.Errorf("saved session captured %d windows; want 1 (the outgoing window)", len(snap.Windows))
	}

	// After a New Session the name is cleared (so the save could only have
	// captured "work" if it ran before this reset — i.e. before closing).
	if m.Opts.SessionName != "" {
		t.Errorf("SessionName = %q after newSession; want empty", m.Opts.SessionName)
	}
	// A fresh starter window replaced the outgoing one.
	if len(m.windowOrder) != 1 {
		t.Errorf("windowOrder = %d after newSession; want 1 fresh starter window", len(m.windowOrder))
	}
}
