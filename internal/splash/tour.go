package splash

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"

	"github.com/oldwired/fvmux/internal/ui"
)

// RunTour walks the user through five centred tooltip dialogs, each
// describing one piece of fvmux UI: menu bar, status line, prefix
// indicator, window-number badges, splitter bars. Each step has Next
// (Enter / Continue) and Skip (Esc) buttons; Skip exits the whole
// tour, Next advances.
//
// Called from the first-run wizard when the user clicks "Take the
// tour", and from Help → Reset First-Run.
func RunTour(a *fvapp.Application) {
	steps := []struct {
		title, body string
	}{
		{
			"Tour 1/5 — Menu bar",
			"  The top bar shows the eight top-level menus.\n" +
				"  Click any menu, or Alt-letter (e.g., Alt-F for File).\n\n" +
				"  Every command lives both in a menu and in the\n" +
				"  command palette — Ctrl-G P opens it.",
		},
		{
			"Tour 2/5 — Status line",
			"  The bottom row shows session name, window list,\n" +
				"  focused-pane title, live CPU + RAM, and a clock.\n\n" +
				"  Bell '!' and activity '-' markers appear per\n" +
				"  window; the focused window is starred '*'.",
		},
		{
			"Tour 3/5 — Prefix key",
			"  Most chords start with a prefix (default Ctrl-G).\n" +
				"  After the prefix, *PREFIX* appears on the left\n" +
				"  of the status bar — that means fvmux is waiting\n" +
				"  for the next key.\n\n" +
				"  Press the prefix twice to send a literal Ctrl-G\n" +
				"  to the focused pane.",
		},
		{
			"Tour 4/5 — Window number",
			"  Each window's title bar shows a small number badge.\n" +
				"  Ctrl-G 1..9 jumps directly to that window;\n" +
				"  windows past 9 show a '+' badge and are reachable\n" +
				"  via Ctrl-G w (list) or Ctrl-G f (fuzzy find).\n\n" +
				"  Ctrl-G , renames the focused window.",
		},
		{
			"Tour 5/5 — Splitters",
			"  Ctrl-G %   splits the focused pane horizontally.\n" +
				"  Ctrl-G \"   splits vertically.\n" +
				"  Ctrl-G h/j/k/l moves focus between panes.\n" +
				"  Ctrl-G R enters resize mode (arrows resize).\n\n" +
				"  That's the tour — happy multiplexing!",
		},
	}
	for _, s := range steps {
		if !showStep(a, s.title, s.body) {
			return
		}
	}
}

func showStep(a *fvapp.Application, title, body string) bool {
	desk := a.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 60, 14, 2)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), title)
	d.Insert(dialogs.NewStaticText(geom.NewRect(2, 2, w-2, h-4), body))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-13, h-3, w/2-3, h-2),
		"~N~ext", consts.CmOK, dialogs.BfDefault,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2+3, h-3, w/2+13, h-2),
		"~S~kip", consts.CmCancel, 0,
	))
	return a.Desktop.ExecView(d) == consts.CmOK
}
