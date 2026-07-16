package splash

import (
	"strings"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"

	"github.com/oldwired/fvmux/internal/glue"
	"github.com/oldwired/fvmux/internal/ui"
)

// askGlue offers to install the fvmuxa wrapper plus fvmux.tmux.conf
// when at least one of them is missing. It returns true if the user
// picked Install — the caller is responsible for the actual write.
//
// Returns false when nothing is missing (skips the dialog silently)
// so re-running the wizard via Help → Reset doesn't badger users who
// already opted in once. Users who declined and later changed their
// mind can delete the existing file(s) or re-run Reset, which will
// re-prompt because Missing() will report them again.
func askGlue(a *fvapp.Application, confDir string) bool {
	missing := glue.DefaultLocations(confDir).Missing()
	if len(missing) == 0 {
		return false
	}

	var pathList strings.Builder
	for _, p := range missing {
		pathList.WriteString("    ")
		pathList.WriteString(p)
		pathList.WriteString("\n")
	}

	body := "" +
		"  fvmux runs in the foreground. For detach/reattach,\n" +
		"  fvmux can install a small wrapper plus a private\n" +
		"  tmux config:\n\n" +
		pathList.String() + "\n" +
		"  After install, run 'fvmuxa' instead of 'fvmux'.\n" +
		"  Detach with F12 d; reattach by running fvmuxa.\n" +
		"  Requires tmux on PATH."

	desk := a.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 72, 18, 2)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "Install detach/reattach glue?")
	d.Insert(dialogs.NewStaticText(geom.NewRect(2, 2, w-2, h-4), body))
	// Mirror welcome's reserved-Cm-codes trick: only the four standard
	// values end Dialog modality. CmYes = Install, CmCancel = Skip
	// (also fired by the [✕] close box).
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-13, h-3, w/2-3, h-2),
		"~I~nstall", consts.CmYes, dialogs.BfDefault,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2+3, h-3, w/2+13, h-2),
		"~S~kip", consts.CmCancel, 0,
	))
	return a.Desktop.ExecView(d) == consts.CmYes
}
