package commands

import "github.com/oldwired/fv-go/pkg/fv/app"

// Ctx is the per-invocation context handed to every Command.Action. Fields
// other than App are populated as later sub-steps land (session lifecycle,
// focused-pane tracking); step 1+2 actions only need App.
type Ctx struct {
	App *app.Application
}

// Command is one fvmux action. The same struct drives menu rendering,
// palette display, and chord dispatch.
type Command struct {
	ID        uint16
	Category  string // "Pane", "Window", "Edit", "View", "File", "Connections", "Transfer", "Help"
	Name      string // "Split Horizontal" — palette-friendly.
	MenuLabel string // "Split ~H~orizontal" — Borland-style hotkey markers.
	Chord     string // "C-g %" — empty if unbound.
	Hidden    bool   // Omit from menus and palette but keep ID stable.
	Action    func(ctx *Ctx)
	Enabled   func(ctx *Ctx) bool // nil ⇒ always enabled.
}
