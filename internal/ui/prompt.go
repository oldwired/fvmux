package ui

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// PromptString opens a centred modal with an InputLine seeded with
// initial. Returns the entered text and true if the user confirmed, or
// "", false on Cancel/Esc. The OK button accepts empty input and the
// text is returned verbatim (no trimming) — the caller decides what
// empty or whitespace-only input means.
func PromptString(a *fvapp.Application, title, label, initial string) (string, bool) {
	desk := a.Desktop.BaseView()
	w, h := 54, 8
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2

	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), title)

	il := dialogs.NewInputLine(geom.NewRect(2, 4, w-3, 5), 256)
	il.SetText(initial)
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 2, w-3, 3), label, il))
	d.Insert(il)

	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-12, h-3, w/2-2, h-2),
		"O~K~", consts.CmOK, dialogs.BfDefault,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2+2, h-3, w/2+12, h-2),
		"~C~ancel", consts.CmCancel, 0,
	))

	if a.Desktop.ExecView(d) != consts.CmOK {
		return "", false
	}
	return il.Text(), true
}
