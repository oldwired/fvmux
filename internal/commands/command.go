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
	Category  string   // "Pane", "Window", "Edit", "View", "File", "Connections", "Transfer", "Help"
	Name      string   // "Split Left/Right" — palette-friendly.
	Aliases   []string // legacy names accepted by keybindings.toml lookup.
	MenuLabel string   // "Split Left/Ri~g~ht" — Borland-style hotkey markers.
	Chord     string   // "C-g %" — empty if unbound. Mutated by prefix rebind + user overrides.
	Hidden    bool     // Omit from menus and palette but keep ID stable.
	Action    func(ctx *Ctx)
	Enabled   func(ctx *Ctx) bool // nil ⇒ always enabled.
	// DisabledReason is concise status-line feedback for an unavailable
	// keyboard/palette invocation. Runtime wiring owns the predicate and
	// reason together so every command surface describes the same state.
	DisabledReason string

	// FactoryChord is the chord registered by Defaults() before any
	// runtime mutation. Used by Registry.ResetChords() so a reload of
	// keybindings.toml starts from a clean baseline. Set automatically
	// on Register; do not assign by hand.
	FactoryChord string
}
