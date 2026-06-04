package headless

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
)

// TestKeybindingsOverrideAppliesUserChord verifies that
// config.LoadKeybindings + registry.ApplyOverrides reroutes a chord
// to a user-chosen command. This is the keybinding-reload path that
// Phase A ships and Help → Reload Config exercises.
func TestKeybindingsOverrideAppliesUserChord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.toml")
	body := `
[[binding]]
chord = "C-g X"
command = "Kill Pane"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write keybindings.toml: %v", err)
	}

	overrides, err := config.LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("len(overrides) = %d, want 1", len(overrides))
	}

	reg := commands.Defaults()
	reg.ApplyOverrides(overrides)

	got := reg.LookupChord("C-g X")
	if got == nil {
		t.Fatal("LookupChord C-g X = nil after ApplyOverrides")
		return
	}
	if got.Name != "Kill Pane" {
		t.Errorf("C-g X bound to %q, want %q", got.Name, "Kill Pane")
	}
}

// TestKeybindingsRemoveClearsChord verifies an override with empty
// command unbinds the chord — used so users can take back keys they
// don't want fvmux holding.
func TestKeybindingsRemoveClearsChord(t *testing.T) {
	reg := commands.Defaults()
	reg.ApplyOverrides([]commands.Override{
		{Chord: "C-g x", Command: ""}, // unbind the default Close Pane.
	})
	if c := reg.LookupChord("C-g x"); c != nil {
		t.Errorf("LookupChord C-g x = %v, want nil after unbind", c)
	}
}
