package app

import (
	"os"
	"os/exec"

	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/glue"
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
				"To get detach/reattach, run fvmux via 'fvmuxa'.\n"+
				"Use Help → Reset First-Run Wizard to install it.",
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

// installGlue runs the file-write side of the first-run wizard's
// glue offer. Splash returns InstallGlue==true; this method does the
// actual disk writes and shows a follow-up dialog when something
// either went wrong or needs the user's attention (PATH not updated).
func (m *Mux) installGlue() {
	locs := glue.DefaultLocations(m.Opts.Paths.Root)
	wrote, err := locs.Install()
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't install fvmuxa glue:\n%s",
			[]any{err.Error()}, msgbox.OKOnly)
		return
	}
	if len(wrote) == 0 {
		return
	}
	if locs.BinDirOnPath() {
		return
	}
	// fvmuxa landed in ~/.local/bin, which isn't on PATH for this
	// user. Without a hint they'd type `fvmuxa` and get "command not
	// found"; the wizard would look broken even though it worked.
	msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
		"Installed fvmuxa, but %s is not on $PATH.\n\n"+
			"Add this to your shell rc, then 'fvmuxa' to launch:\n"+
			"    export PATH=\"$HOME/.local/bin:$PATH\"",
		[]any{locs.BinDir}, msgbox.OKOnly)
}
