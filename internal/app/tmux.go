package app

import (
	"os"
	"os/exec"

	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/commands"
)

// inTmux reports whether fvmux is running inside a tmux client. tmux
// always exports TMUX into the child environment; its absence means we
// were launched directly.
func inTmux() bool { return os.Getenv("TMUX") != "" }

// detachFromTmux asks the outer tmux to detach the current client. The
// tmux server keeps the session (and therefore fvmux) running in the
// background until `fvmuxa` is invoked again to reattach.
func (m *Mux) detachFromTmux() {
	if !inTmux() {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"fvmux isn't running inside tmux — nothing to detach from.\n\n"+
				"To get detach/reattach, run fvmux via 'fvmuxa'\n"+
				"(see scripts/fvmuxa and 'make install-glue').",
			msgbox.OKOnly)
		return
	}
	// tmux detach-client uses the socket inferred from the TMUX env
	// var, so this hits the fvmuxa-private socket cleanly.
	if err := exec.Command("tmux", "detach-client").Run(); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"tmux detach-client failed:\n%s",
			[]any{err.Error()}, msgbox.OKOnly)
	}
}

// wireTmuxAction registers CmdDetach. Enabled only when inside tmux —
// menus and the palette grey it out otherwise, matching the user's
// mental model that it's a no-op without an outer tmux.
func (m *Mux) wireTmuxAction() {
	c := m.Reg.ByID(commands.CmdDetach)
	if c == nil {
		return
	}
	c.Action = func(*commands.Ctx) { m.detachFromTmux() }
	c.Enabled = func(*commands.Ctx) bool { return inTmux() }
}
