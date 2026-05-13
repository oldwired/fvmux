// Package copymode implements fvmux's "copy mode" — a read-only
// scrollback viewer that lets the user navigate, optionally select,
// and copy terminal history into the clipboard.
//
// Step 7 ships a simplified version: the focused pane's scrollback is
// pulled via terminal.ScrollbackText() and shown in a modal markdown
// view. Selection-based copy uses the host terminal's own
// click-and-drag (kitty / iTerm2 / etc.), which is what most users do
// anyway. Sub-step 12 polishes this into a true cell-grid selector.
package copymode

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/clipboard"
)

// Show opens the scrollback viewer modally over a.Desktop, populated
// with t.ScrollbackText(). Esc / Enter / OK dismiss it; before
// dismissing, the entire scrollback is also pushed to the system
// clipboard (so users can paste anywhere immediately).
func Show(a *fvapp.Application, t *terminal.Terminal) {
	if t == nil {
		return
	}
	text := t.ScrollbackText()
	if text == "" {
		text = "(scrollback empty)"
	}
	_ = clipboard.Set(text)

	desk := a.Desktop.BaseView()
	w, h := 80, 24
	if w > desk.Size.X-4 {
		w = desk.Size.X - 4
	}
	if h > desk.Size.Y-4 {
		h = desk.Size.Y - 4
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "Copy Mode (Esc to close)")
	mv := markdown.New(geom.NewRect(2, 2, w-3, h-3), nil)
	mv.SetMarkdown("```\n" + text + "\n```")
	d.Insert(mv)
	d.Insert(dialogs.NewButton(geom.NewRect(w/2-5, h-3, w/2+5, h-2),
		"O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

// Paste reads the OS clipboard and writes it to t. When the pane has
// bracketed-paste mode enabled, the text is wrapped in the bracketed
// markers; otherwise it's sent raw.
func Paste(t *terminal.Terminal) error {
	if t == nil {
		return nil
	}
	text, err := clipboard.Get()
	if err != nil || text == "" {
		return err
	}
	return t.Paste(text)
}
