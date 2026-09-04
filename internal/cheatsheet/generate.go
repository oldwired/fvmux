// Package cheatsheet auto-generates fvmux's keybinding reference from
// the live commands.Registry. The same generator feeds the in-app
// Ctrl-G ? viewer and the build-time-baked assets/cheatsheet.md.
//
//go:generate go run ./cmd/genmd
package cheatsheet

import (
	"sort"
	"strings"
	"time"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/shortcuts"
	"github.com/oldwired/fvmux/internal/whimsy"
)

// Categories defines the order categories appear in the cheatsheet.
// Any registered category not listed here gets appended after these
// (sorted alphabetically) so the cheatsheet stays exhaustive.
var Categories = []string{
	"File", "Edit", "View", "Pane", "Window",
	"Connections", "Transfer", "Help",
}

// Generate produces the cheatsheet for the live Ctrl-G ? viewer, with a
// whimsy tagline footer that rotates across days.
func Generate(reg *commands.Registry) string {
	return generate(reg, dailyTagline())
}

// GenerateBaked produces the byte-for-byte reproducible cheatsheet baked
// into assets/cheatsheet.md by cmd/genmd. It omits the date-dependent
// tagline so the committed asset stays stable and the drift test (which
// diffs it against a fresh Generate) never flaps.
func GenerateBaked(reg *commands.Registry) string {
	return generate(reg, "")
}

// generate renders every visible command's chord and name grouped by
// category in Categories order, appending tagline as a footer when non-empty.
func generate(reg *commands.Registry, tagline string) string {
	var b strings.Builder
	b.WriteString("# fvmux Keyboard Reference\n\n")
	b.WriteString("Global commands use the configured prefix. Contextual keys apply only in the named mode or focused view.\n\n")
	b.WriteString("## Global prefix commands\n\n")

	seen := map[string]bool{}
	for _, cat := range Categories {
		writeCategory(&b, reg, cat)
		seen[cat] = true
	}

	// Append any categories the user added beyond the standard eight.
	extras := map[string]bool{}
	for _, c := range reg.All() {
		if !seen[c.Category] && c.Category != "" {
			extras[c.Category] = true
		}
	}
	var sortedExtras []string
	for k := range extras {
		sortedExtras = append(sortedExtras, k)
	}
	sort.Strings(sortedExtras)
	for _, cat := range sortedExtras {
		writeCategory(&b, reg, cat)
	}
	b.WriteString(shortcuts.Markdown(activePrefix(reg)))
	if tagline != "" {
		b.WriteString("---\n\n")
		b.WriteString("*")
		b.WriteString(tagline)
		b.WriteString("*\n")
	}
	// Markdown files conventionally end in one newline. Keeping this invariant
	// here prevents generated outputs from accumulating a blank line at EOF.
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// dailyTagline picks a whimsy footer that is stable within a session but
// rotates across days.
func dailyTagline() string {
	if len(whimsy.Taglines) == 0 {
		return ""
	}
	return whimsy.Taglines[int(time.Now().YearDay())%len(whimsy.Taglines)]
}

func writeCategory(b *strings.Builder, reg *commands.Registry, category string) {
	cmds := reg.ByCategory(category)
	if len(cmds) == 0 {
		return
	}
	var visible []*commands.Command
	for _, c := range cmds {
		if c.Hidden {
			continue
		}
		visible = append(visible, c)
	}
	if len(visible) == 0 {
		return
	}
	b.WriteString("### ")
	b.WriteString(category)
	b.WriteString("\n\n")
	for _, c := range visible {
		b.WriteString("- ")
		if c.Chord != "" {
			b.WriteString("`")
			b.WriteString(c.Chord)
			b.WriteString("` — ")
		}
		b.WriteString(highlightMenuAccelerator(c.Name, c.MenuLabel))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// highlightMenuAccelerator transfers the Borland-style ~X~ marker from a
// menu label onto the same character in the command's more descriptive name.
// MarkdownView renders that span with its brighter emphasis color. Keeping the
// command name preserves useful suffixes that menus omit, such as "(Ctrl-C)".
func highlightMenuAccelerator(name, menuLabel string) string {
	start := strings.IndexByte(menuLabel, '~')
	if start < 0 {
		return name
	}
	relEnd := strings.IndexByte(menuLabel[start+1:], '~')
	if relEnd < 0 {
		return name
	}
	end := start + 1 + relEnd
	plainLabel := menuLabel[:start] + menuLabel[start+1:end] + menuLabel[end+1:]
	nameBase := strings.Index(name, plainLabel)
	if nameBase < 0 {
		nameBase = strings.Index(strings.ToLower(name), strings.ToLower(plainLabel))
	}
	if nameBase < 0 {
		return name
	}
	hotStart := nameBase + start
	hotEnd := hotStart + len(menuLabel[start+1:end])
	return name[:hotStart] + "**" + name[hotStart:hotEnd] + "**" + name[hotEnd:]
}

func activePrefix(reg *commands.Registry) string {
	if command := reg.ByID(commands.CmdLiteralPrefix); command != nil {
		if parts := strings.Fields(command.Chord); len(parts) > 0 {
			return parts[0]
		}
	}
	return "C-g"
}
