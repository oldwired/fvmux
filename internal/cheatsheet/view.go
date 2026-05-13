package cheatsheet

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"

	"github.com/oldwired/fvmux/internal/commands"
)

// Show opens a centred modal showing the auto-generated cheatsheet.
// Esc / OK dismiss.
func Show(a *fvapp.Application, reg *commands.Registry) {
	desk := a.Desktop.BaseView()
	w, h := 70, 22
	if w > desk.Size.X-4 {
		w = desk.Size.X - 4
	}
	if h > desk.Size.Y-4 {
		h = desk.Size.Y - 4
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "Cheatsheet (Esc to close)")
	mv := markdown.New(geom.NewRect(2, 2, w-3, h-3), nil)
	mv.SetMarkdown(Generate(reg))
	d.Insert(mv)
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-5, h-3, w/2+5, h-2),
		"O~K~", consts.CmOK, dialogs.BfDefault,
	))
	a.Desktop.ExecView(d)
}
