// Package cheatsheet auto-generates fvmux's keybinding reference from
// the live commands.Registry. The same generator feeds the in-app
// Ctrl-G ? viewer and the build-time-baked assets/cheatsheet.md.
package cheatsheet

import (
	"sort"
	"strings"

	"github.com/oldwired/fvmux/internal/commands"
)

// Categories defines the order categories appear in the cheatsheet.
// Any registered category not listed here gets appended after these
// (sorted alphabetically) so the cheatsheet stays exhaustive.
var Categories = []string{
	"File", "Edit", "View", "Pane", "Window",
	"Connections", "Transfer", "Help",
}

// Generate produces a markdown document with every visible command's
// chord and name, grouped by category in Categories order.
func Generate(reg *commands.Registry) string {
	var b strings.Builder
	b.WriteString("# fvmux Cheatsheet\n\n")

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
	return b.String()
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
	b.WriteString("## ")
	b.WriteString(category)
	b.WriteString("\n\n")
	for _, c := range visible {
		b.WriteString("- ")
		if c.Chord != "" {
			b.WriteString("`")
			b.WriteString(c.Chord)
			b.WriteString("` — ")
		}
		b.WriteString(c.Name)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}
