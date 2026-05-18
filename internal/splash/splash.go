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
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"

	"github.com/oldwired/fvmux/internal/prefix"
)

// Result describes whatever the wizard wants the caller to apply.
// Empty PrefixKey / Shell / Theme means "user kept the current value"
// — caller should treat as no-op for that field. QuitRequested ==
// true means the user clicked "Quit fvmux" in the welcome dialog;
// the caller should exit the app. InstallGlue == true means the user
// opted into the fvmuxa wrapper install; the caller invokes
// glue.Install() to actually write the files (splash itself never
// touches disk).
type Result struct {
	PrefixKey     string
	Shell         string
	Theme         string
	InstallGlue   bool
	QuitRequested bool
}

// ThemeChoice is the wizard's hook for surfacing fvmux's themes. The
// callback opens whatever picker fvmux uses (live preview) and
// returns the chosen theme name, or "" if the user cancelled / kept
// the current. The splash package can't import internal/theme
// directly — that's the host's job.
type ThemeChoice func() string

// Run drives the wizard: splash → welcome → (optional tour) → prefix
// picker → shell picker → theme picker → optional glue installer.
// currentPrefixKey / currentShell are the values currently in
// config.toml, shown as the active selection in each picker (an empty
// currentShell means "honour $SHELL"). pickTheme, when non-nil, opens
// the host's live theme picker after the shell step. confDir is
// fvmux's config root — the glue step uses it to decide where
// fvmux.tmux.conf belongs and whether it's already installed.
//
// The caller is responsible for persisting Result and flipping
// state.FirstRunDone — keeping Run side-effect-free makes the same
// function usable for both first-launch and Help → Reset First-Run.
func Run(a *fvapp.Application, currentPrefixKey, currentShell, confDir string, pickTheme ThemeChoice) Result {
	showSplash(a)
	choice := showWelcome(a)
	switch choice {
	case welcomeQuit:
		return Result{QuitRequested: true}
	case welcomeTour:
		RunTour(a)
	}
	pickedPrefix := pickPrefix(a, currentPrefixKey)
	pickedShell := pickShell(a, currentShell)
	var pickedTheme string
	if pickTheme != nil {
		pickedTheme = pickTheme()
	}
	installGlue := askGlue(a, confDir)
	return Result{
		PrefixKey:   pickedPrefix,
		Shell:       pickedShell,
		Theme:       pickedTheme,
		InstallGlue: installGlue,
	}
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

// welcomeChoice is the user's answer to the 3-button welcome dialog.
type welcomeChoice int

const (
	welcomeSkip welcomeChoice = iota // continue to prefix picker without tour.
	welcomeTour                      // run the 5-step tour.
	welcomeQuit                      // exit fvmux now.
)

// Reuse fv-go's reserved Cm codes for the welcome buttons so the
// Dialog ends modal on click. Mapping (3 buttons + close-box):
//
//	Take the tour → CmYes
//	Skip          → CmOK   (and CmCancel from the [✕] close box)
//	Quit fvmux    → CmNo
//
// Custom Cm codes would leave the dialog open until the close box
// is hit — Dialog.HandleEvent only EndModals on the four standard
// values.
func showWelcome(a *fvapp.Application) welcomeChoice {
	desk := a.Desktop.BaseView()
	w, h := 64, 13
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "Welcome to fvmux")
	d.Insert(dialogs.NewStaticText(
		geom.NewRect(2, 2, w-2, h-4),
		"  Useful chords (assuming Ctrl-G prefix):\n\n"+
			"    c   new window\n"+
			"    %   split horizontal\n"+
			"    P   command palette\n"+
			"    ?   full cheatsheet\n"+
			"    ,   rename window\n\n"+
			"  Take the 5-step tour, skip to the prefix picker,\n"+
			"  or quit and come back later.",
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(2, h-3, 18, h-2),
		"Take the ~t~our", consts.CmYes, dialogs.BfDefault,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(20, h-3, 30, h-2),
		"~S~kip", consts.CmOK, 0,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(32, h-3, 50, h-2),
		"~Q~uit fvmux", consts.CmNo, 0,
	))
	switch a.Desktop.ExecView(d) {
	case consts.CmYes:
		return welcomeTour
	case consts.CmNo:
		return welcomeQuit
	default:
		// CmOK (Skip) and CmCancel (close box) both mean skip.
		return welcomeSkip
	}
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
