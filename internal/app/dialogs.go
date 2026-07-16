package app

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"

	"github.com/oldwired/fvmux/internal/ui"
)

// promptString opens a centred modal with an InputLine seeded with
// `initial`. Returns the entered text and true if the user confirmed,
// or "", false on Cancel/Esc. The OK button accepts empty input and the
// text is returned verbatim (no trimming) — the caller decides what that
// means (Rename Pane keeps whatever was typed; session names trim + validate
// at their own call sites).
func promptString(a *fvapp.Application, title, label, initial string) (string, bool) {
	return ui.PromptString(a, title, label, initial)
}
