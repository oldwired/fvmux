package cheatsheet

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/ui"
)

// Show opens a centred modal showing the auto-generated cheatsheet.
// Esc / OK dismiss.
func Show(a *fvapp.Application, reg *commands.Registry) {
	desk := a.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 70, 22, 4)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
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
