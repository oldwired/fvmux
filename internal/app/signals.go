package app

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
)

// OnQuitRequested is the OnQuitRequest hook: it decides whether a quit
// may proceed and, when it may, persists the session first. A normal
// quit asks CanQuit (which may show the confirm-kill prompt); a
// signal-driven quit bypasses that prompt entirely (a modal is
// impossible mid-signal) and goes straight to save-and-proceed.
func (m *Mux) OnQuitRequested() bool {
	if !m.signalQuit.Load() && !m.CanQuit() {
		return false
	}
	_ = m.SaveSessionSilent() // no-op when no session is named.
	return true
}

// InstallSignalHandlers wires OS-signal-driven graceful shutdown. fv-go's
// Run() installs no signal handling of its own, so without this an
// external SIGTERM/SIGHUP/SIGINT (e.g. `kill`, terminal hangup) would
// terminate fvmux via Go's default disposition — skipping OnQuitRequest,
// so the session autosave never runs and ControlMaster children are
// orphaned.
//
// The first signal sets signalQuit and posts CmQuitApp, funnelling the
// shutdown through the same loop-exit path as a normal quit (OnQuitRequest
// → SaveSessionSilent → deferred ShutdownSSHPool / a.Done() PTY teardown).
// A second signal hard-exits, in case the event loop is wedged and never
// drains the posted command.
//
// SIGWINCH is intentionally NOT handled here — fv-go's backend already
// translates it into a resize event.
func (m *Mux) InstallSignalHandlers() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	go func() {
		<-ch // first signal: request a graceful, state-saving quit.
		m.signalQuit.Store(true)
		m.App.PostEvent(drivers.Event{What: consts.EvCommand, Command: consts.CmQuitApp})
		<-ch // second signal: the loop is stuck — bail out hard.
		os.Exit(1)
	}()
}

// sendBytes is the common path for control-byte signals (Ctrl-C, Ctrl-\,
// Ctrl-D, …). The byte hits the PTY just like the user pressing the key
// would, so the kernel's tty driver generates the corresponding signal
// for the foreground process group.
func (m *Mux) sendBytes(b ...byte) {
	if t := m.FocusedTerminal(); t != nil {
		_, _ = t.Write(b)
	}
}

func (m *Mux) sendSIGINT()  { m.sendBytes(0x03) } // Ctrl-C
func (m *Mux) sendSIGQUIT() { m.sendBytes(0x1c) } // Ctrl-\
func (m *Mux) sendEOF()     { m.sendBytes(0x04) } // Ctrl-D

// sendSIGTERM signals the child PROCESS directly (not the foreground
// job — useful when the foreground shell is stuck or unresponsive).
// On Windows, Signal(SIGTERM) returns an error; we ignore it.
func (m *Mux) sendSIGTERM() {
	t := m.FocusedTerminal()
	if t == nil {
		return
	}
	pid := t.PID()
	if pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
}
