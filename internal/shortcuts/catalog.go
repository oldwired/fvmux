// Package shortcuts describes keyboard surfaces that are intentionally local
// to a mode or widget rather than global commands.Registry actions.
package shortcuts

import (
	"strings"
)

const (
	ScopeMenu       = "Menu activation and navigation"
	ScopeTerminal   = "Embedded terminal and scrollback"
	ScopeCopy       = "Copy mode"
	ScopeResize     = "Resize mode"
	ScopeFiles      = "Files / SFTP window"
	ScopeCheatsheet = "Cheatsheet window"
	ScopeDialogs    = "Dialogs and pickers"
	ScopeWrapper    = "fvmuxa / tmux wrapper"
	ScopePlatform   = "Platform, layout, and protocol notes"
)

// ContextBinding is one non-registry shortcut or keyboard compatibility note.
// Compact, when present, is used in space-constrained UI hints.
type ContextBinding struct {
	Scope    string
	Chord    string
	Action   string
	Notes    string
	Platform string
	Compact  string
}

// Scopes fixes the deterministic output order.
var Scopes = []string{
	ScopeMenu, ScopeTerminal, ScopeCopy, ScopeResize, ScopeFiles,
	ScopeCheatsheet, ScopeDialogs, ScopeWrapper, ScopePlatform,
}

// Catalogue is the single source of truth for contextual keyboard behavior.
// Global prefix commands remain exclusively in commands.Registry.
var Catalogue = []ContextBinding{
	{Scope: ScopeMenu, Chord: "<prefix> m", Action: "Activate the fvmux menu bar", Notes: "works from terminal and non-terminal focus"},
	{Scope: ScopeMenu, Chord: "F10 / Alt mnemonic", Action: "Activate a top-level menu", Notes: "only when focus is not inside an embedded terminal"},
	{Scope: ScopeMenu, Chord: "Left / Right", Action: "Move between top-level menus"},
	{Scope: ScopeMenu, Chord: "Up / Down", Action: "Move within a menu"},
	{Scope: ScopeMenu, Chord: "Enter", Action: "Open or select the highlighted item"},
	{Scope: ScopeMenu, Chord: "Esc", Action: "Close the active menu"},

	{Scope: ScopeTerminal, Chord: "F10 / Alt-F, Alt-E, Alt-V, Alt-P, Alt-W, Alt-C, Alt-T, Alt-H", Action: "Forward to the child PTY", Notes: "the menu bar deliberately passes these through while terminal focus is raw"},
	{Scope: ScopeTerminal, Chord: "Shift-PgUp / Shift-PgDn", Action: "Scroll back or forward by half a page"},
	{Scope: ScopeTerminal, Chord: "Shift-Home / Shift-End", Action: "Jump to the oldest scrollback or live bottom"},
	{Scope: ScopeTerminal, Chord: "/", Action: "Search scrollback", Notes: "when already viewing scrollback"},
	{Scope: ScopeTerminal, Chord: "n / N", Action: "Move to next / previous search result"},
	{Scope: ScopeTerminal, Chord: "Other keys", Action: "Forward to the child PTY", Notes: "includes fv-go v0.5.4 classic modified arrows, navigation keys, F1-F12, Alt-Unicode, Ctrl-Space, and DECCKM cursor keys"},

	{Scope: ScopeCopy, Chord: "Arrows / PgUp / PgDn / Home / End", Action: "Move the copy cursor"},
	{Scope: ScopeCopy, Chord: "Space", Action: "Toggle the selection anchor"},
	{Scope: ScopeCopy, Chord: "Enter", Action: "Copy the selection and exit"},
	{Scope: ScopeCopy, Chord: "/", Action: "Exit copy mode and start scrollback search"},
	{Scope: ScopeCopy, Chord: "Esc", Action: "Exit without copying"},

	{Scope: ScopeResize, Chord: "h / j / k / l", Action: "Move the nearest divider by one cell"},
	{Scope: ScopeResize, Chord: "H / J / K / L", Action: "Move the nearest divider by five cells"},
	{Scope: ScopeResize, Chord: "Esc", Action: "Exit resize mode"},

	{Scope: ScopeFiles, Chord: "Tab", Action: "Rotate focus through remote tree, remote listing, local tree, and local listing"},
	{Scope: ScopeFiles, Chord: "Enter", Action: "Enter a folder or preview a file"},
	{Scope: ScopeFiles, Chord: "F5", Action: "Copy the selected file or folder to the other side", Compact: "F5 cp"},
	{Scope: ScopeFiles, Chord: "F6", Action: "Move or rename the selected entry", Compact: "F6 mv"},
	{Scope: ScopeFiles, Chord: "F7", Action: "Create a directory on the focused side", Compact: "F7 mkdir"},
	{Scope: ScopeFiles, Chord: "F8", Action: "Delete the selected entry recursively", Compact: "F8 del"},
	{Scope: ScopeFiles, Chord: "Ctrl-R", Action: "Refresh both panels", Compact: "Ctrl-R refresh"},
	{Scope: ScopeFiles, Chord: "Delete", Action: "Cancel the newest active transfer"},
	{Scope: ScopeFiles, Chord: "Esc", Action: "Close the Files window"},

	{Scope: ScopeCheatsheet, Chord: "/", Action: "Fuzzy-search every reference entry"},
	{Scope: ScopeCheatsheet, Chord: "Arrows / PgUp / PgDn / Home / End", Action: "Scroll the reference"},
	{Scope: ScopeCheatsheet, Chord: "Esc", Action: "Clear a text selection or close the Cheatsheet window"},

	{Scope: ScopeDialogs, Chord: "Tab / Shift-Tab", Action: "Move focus between controls"},
	{Scope: ScopeDialogs, Chord: "Arrows", Action: "Move within lists and popup choices"},
	{Scope: ScopeDialogs, Chord: "Type", Action: "Filter fuzzy pickers or edit the active input"},
	{Scope: ScopeDialogs, Chord: "Enter", Action: "Accept the current choice"},
	{Scope: ScopeDialogs, Chord: "Esc", Action: "Cancel or close"},

	{Scope: ScopeWrapper, Chord: "F12 d", Action: "Detach the private tmux session"},
	{Scope: ScopeWrapper, Chord: "F12 F12", Action: "Send one literal F12 to fvmux / its focused child"},

	{Scope: ScopePlatform, Chord: "macOS F-keys", Action: "Hold Fn/Globe when system keyboard settings reserve physical F-keys", Platform: "macOS"},
	{Scope: ScopePlatform, Chord: "Logical characters", Action: "Bindings follow the character produced by the keyboard layout, not the physical key position", Notes: "the portable preset avoids Alt/Meta and punctuation-heavy US-layout positions"},
	{Scope: ScopePlatform, Chord: "Legacy xterm input", Action: "Supported keyboard protocol boundary", Notes: "do not force kitty keyboard protocol, CSI-u, or modifyOtherKeys for the fvmux process; enhanced protocols are not negotiated"},
	{Scope: ScopePlatform, Chord: "Ctrl-I / Ctrl-M / Ctrl-[", Action: "Classic aliases remain Tab / Enter / Esc", Notes: "fvmux canonicalizes these aliases so they cannot create unreachable duplicate bindings"},
}

// ForScope returns a copy of the descriptors belonging to scope.
func ForScope(scope string) []ContextBinding {
	var out []ContextBinding
	for _, binding := range Catalogue {
		if binding.Scope == scope {
			out = append(out, binding)
		}
	}
	return out
}

// MarkdownForScope renders a bullet list. Prefix placeholders are replaced
// for live in-app help while committed docs pass the default C-g.
func MarkdownForScope(scope, prefix string) string {
	var b strings.Builder
	for _, binding := range ForScope(scope) {
		chord := renderPrefix(binding.Chord, prefix)
		b.WriteString("- **")
		b.WriteString(chord)
		b.WriteString("** — ")
		b.WriteString(binding.Action)
		if binding.Notes != "" {
			b.WriteString("; ")
			b.WriteString(binding.Notes)
		}
		if binding.Platform != "" {
			b.WriteString(" [")
			b.WriteString(binding.Platform)
			b.WriteString("]")
		}
		b.WriteString(".\n")
	}
	return b.String()
}

// Markdown renders all contextual scopes in deterministic order.
func Markdown(prefix string) string {
	var b strings.Builder
	for _, scope := range Scopes {
		b.WriteString("## ")
		b.WriteString(scope)
		b.WriteString("\n\n")
		b.WriteString(MarkdownForScope(scope, prefix))
		b.WriteString("\n")
	}
	return b.String()
}

// Compact returns the maintained short hint for a space-constrained scope.
func Compact(scope string) string {
	var parts []string
	for _, binding := range ForScope(scope) {
		if binding.Compact != "" {
			parts = append(parts, binding.Compact)
		}
	}
	return strings.Join(parts, " · ")
}

func renderPrefix(chord, prefix string) string {
	if prefix == "" {
		prefix = "C-g"
	}
	return strings.ReplaceAll(chord, "<prefix>", prefix)
}
