// Package splash handles fvmux's first-run UX and the on-demand reset
// path: an ASCII splash, a welcome dialog, and a prefix-key picker.
//
// Step 8 ships the ASCII fallback only; sub-step 12 polishes by
// preferring assets/splash.six via SIXEL when the host terminal
// supports it.
package splash

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"

	"github.com/oldwired/fvmux/internal/prefix"
)

// Result describes whatever the wizard wants the caller to apply.
// Empty PrefixKey means "user kept the current prefix" — caller
// should treat as no-op.
type Result struct {
	PrefixKey string
}

// Run drives the three-step wizard: splash → welcome → prefix picker.
// currentPrefixKey is the config.toml-style token ("C-g", "C-b", "C-a")
// shown as the current selection.
//
// The caller is responsible for persisting Result and flipping
// state.FirstRunDone — keeping Run side-effect-free makes the same
// function usable for both first-launch and Help → Reset First-Run.
func Run(a *fvapp.Application, currentPrefixKey string) Result {
	showSplash(a)
	showWelcome(a)
	picked := pickPrefix(a, currentPrefixKey)
	return Result{PrefixKey: picked}
}

func showSplash(a *fvapp.Application) {
	desk := a.Desktop.BaseView()
	w, h := 54, 12
	if w > desk.Size.X-4 {
		w = desk.Size.X - 4
	}
	if h > desk.Size.Y-4 {
		h = desk.Size.Y - 4
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "fvmux")
	d.Insert(dialogs.NewStaticText(
		geom.NewRect(2, 2, w-2, h-3),
		"  fvmux — a floating-window terminal multiplexer\n"+
			"           windows in your windows.\n\n"+
			"  Press your prefix key, then ? for help.\n"+
			"  You can re-open this wizard from Help → Reset\n"+
			"  First-Run Wizard at any time.",
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-6, h-3, w/2+6, h-2),
		"Cont~i~nue", consts.CmOK, dialogs.BfDefault,
	))
	a.Desktop.ExecView(d)
}

func showWelcome(a *fvapp.Application) {
	msgbox.Show(&a.Desktop.Group, msgbox.Info,
		"Useful chords (assuming Ctrl-G prefix):\n\n"+
			"  c   new window\n"+
			"  %   split horizontal\n"+
			"  P   command palette\n"+
			"  ?   full cheatsheet\n"+
			"  ,   rename window\n\n"+
			"Next: choose your prefix key.",
		msgbox.OKOnly)
}

// pickPrefix asks the user to choose one of prefix.Available. Returns
// the picked ConfigKey, or "" if the user cancelled (keep current).
func pickPrefix(a *fvapp.Application, currentPrefixKey string) string {
	items := make([]string, len(prefix.Available))
	for i, s := range prefix.Available {
		mark := "  "
		if s.ConfigKey == currentPrefixKey {
			mark = "* "
		}
		items[i] = mark + s.Label
	}
	desk := a.Desktop.BaseView()
	origin := geom.Point{
		X: (desk.Size.X - 30) / 2,
		Y: (desk.Size.Y - 8) / 2,
	}
	idx := popupmenu.New(origin, items, 40).Run(&a.Desktop.Group)
	if idx < 0 || idx >= len(prefix.Available) {
		return ""
	}
	return prefix.Available[idx].ConfigKey
}
