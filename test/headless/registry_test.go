// Package headless drives fvmux through fv-go's in-memory backend so
// CI can exercise the command surface and golden-frame snapshots
// without a real TTY.
//
// Step 11 ships the registry/cheatsheet integration tests; full
// app.Application-level golden-frame tests (split panes, palette
// modal, session restore) land in sub-step 12 alongside the smoke
// scripts.
package headless

import (
	"strings"
	"testing"

	"github.com/oldwired/fvmux/internal/cheatsheet"
	"github.com/oldwired/fvmux/internal/commands"
)

func TestDefaultRegistryHasEveryMenuCategory(t *testing.T) {
	reg := commands.Defaults()
	for _, cat := range cheatsheet.Categories {
		// Help/Connections/Transfer may legitimately have 0 visible
		// commands until later sub-steps populate them, so only check
		// the categories step-11 expects populated.
		switch cat {
		case "File", "Edit", "View", "Pane", "Window", "Help":
			if len(reg.ByCategory(cat)) == 0 {
				t.Errorf("category %q is empty after Defaults()", cat)
			}
		}
	}
}

func TestRegistryChordsUnique(t *testing.T) {
	reg := commands.Defaults()
	seen := map[string]uint16{}
	for _, c := range reg.All() {
		if c.Chord == "" {
			continue
		}
		if prev, dup := seen[c.Chord]; dup {
			t.Errorf("chord %q bound to both id=%d and id=%d", c.Chord, prev, c.ID)
		}
		seen[c.Chord] = c.ID
	}
}

func TestCheatsheetIncludesKnownBindings(t *testing.T) {
	reg := commands.Defaults()
	md := cheatsheet.Generate(reg)
	for _, want := range []string{
		"C-g c", "C-g %", "C-g \"", "C-g x", "C-g z",
		"C-g P", "C-g ?", "C-g X", "C-g h", "C-g j", "C-g k", "C-g l",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("cheatsheet missing chord %q", want)
		}
	}
	if !strings.Contains(md, "## Pane") {
		t.Error("cheatsheet missing Pane category header")
	}
}

func TestKillWindowUsesDeliberateShiftedBinding(t *testing.T) {
	reg := commands.Defaults()
	if got := reg.LookupChord("C-g X"); got == nil || got.ID != commands.CmdKillWindow {
		t.Fatalf("C-g X binding = %#v, want Kill Window", got)
	}
	if got := reg.LookupChord("C-g &"); got != nil {
		t.Fatalf("legacy C-g & binding still active: %#v", got)
	}
}

func TestCommandsHaveActionableMetadata(t *testing.T) {
	reg := commands.Defaults()
	for _, c := range reg.All() {
		if c.Name == "" {
			t.Errorf("command id=%d has empty Name", c.ID)
		}
		if c.Category == "" {
			t.Errorf("command id=%d (%q) has empty Category", c.ID, c.Name)
		}
	}
}
