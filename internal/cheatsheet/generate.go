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
	"github.com/oldwired/fvmux/internal/whimsy"
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
	if len(whimsy.Taglines) > 0 {
		// Deterministic-per-day pick so the footer is stable within a
		// session but rotates across days.
		idx := int(time.Now().YearDay()) % len(whimsy.Taglines)
		b.WriteString("---\n\n")
		b.WriteString("*")
		b.WriteString(whimsy.Taglines[idx])
		b.WriteString("*\n")
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
