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
		prof = profile.Defaults()[0]
	}
	interior := windowInterior(ws.Frame)
	newPane, err := profile.Instantiate(prof, interior, m.Opts.Config.Terminal.ScrollbackLines)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't respawn %s:\n%s",
			[]any{prof.Command, err.Error()},
			msgbox.OKOnly)
		return
	}
	m.wireTerminalCallbacks(newPane, ws.Frame)
	ws.Focus.Pane = newPane
	m.rerender(ws)
}
