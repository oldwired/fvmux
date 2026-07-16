package app

import (
	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/profile"
)

// respawnPane replaces a dead pane's terminal with a freshly spawned
// one running the same profile. No-op if the focused pane is alive —
// killing a live pane first is the user's responsibility (Ctrl-G x or
// Send SIGINT).
func (m *Mux) respawnPane() {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil || ws.Focus.Pane == nil {
		return
	}
	pane := ws.Focus.Pane
	if !pane.Dead {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"Pane is still alive — kill it first if you want to respawn.",
			msgbox.OKOnly)
		return
	}
	prof := profile.Find(m.Opts.Profiles, pane.Profile)
	if prof == nil {
		// Ad-hoc ssh panes carry the host alias as their Profile name —
		// resolve through the same fallback session restore uses, so a
		// respawn reconnects to the host instead of silently spawning a
		// local shell the user mistakes for the remote machine.
		prof = m.resolveProfileFallback(pane.Profile)
	}
	interior := windowInterior(ws.Frame)
	newPane, err := m.instantiateProfile(prof, interior)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't respawn %s:\n%s",
			[]any{prof.Command, err.Error()},
			msgbox.OKOnly)
		return
	}
	m.wireTerminalCallbacks(newPane, ws.Frame)
	// Release the dead pane's PTY before dropping the reference, so its
	// file descriptors don't linger until GC.
	m.stopPane(pane)
	ws.Focus.Pane = newPane
	m.rerender(ws)
}
