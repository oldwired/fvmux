package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oldwired/fvmux/internal/commands"
)

func writeKeybindings(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadKeybindings_RejectsUndispatchableBoundChords is the regression for
// finding #20 (rejection direction): a binding whose chord the prefix
// dispatcher can never emit (arrows, F-keys, S-/A- modified steps) must be
// reported in `rejected` — carrying the command name — and must NOT enter
// overrides. Applying the (empty) overrides to a factory registry then leaves
// each target command's factory chord intact, proving the rejection never
// strips a live binding.
func TestLoadKeybindings_RejectsUndispatchableBoundChords(t *testing.T) {
	path := writeKeybindings(t, `
[[binding]]
chord = "C-g Left"
command = "Kill Pane"

[[binding]]
chord = "C-g F5"
command = "Zoom Focused Pane"

[[binding]]
chord = "C-g S-Tab"
command = "New Window"
`)
	overrides, rejected, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("undispatchable bound chords must not enter overrides; got %d: %+v", len(overrides), overrides)
	}
	if len(rejected) != 3 {
		t.Fatalf("expected 3 rejected bindings, got %d: %v", len(rejected), rejected)
	}
	for _, want := range []string{"Kill Pane", "Zoom Focused Pane", "New Window"} {
		found := false
		for _, r := range rejected {
			if strings.Contains(r, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rejected slice missing command name %q: %v", want, rejected)
		}
	}

	// Apply the (empty) overrides to a factory registry: because the rejected
	// bindings were never applied, each target command's factory chord still
	// resolves through LookupChord.
	reg := commands.Defaults()
	reg.ApplyOverrides(overrides)
	for chord, name := range map[string]string{
		"C-g x": "Kill Pane",
		"C-g z": "Zoom Focused Pane",
		"C-g c": "New Window",
	} {
		c := reg.LookupChord(chord)
		if c == nil {
			t.Errorf("factory chord %q was stripped; LookupChord returned nil", chord)
			continue
		}
		if c.Name != name {
			t.Errorf("chord %q resolves to %q, want %q", chord, c.Name, name)
		}
	}
}

// TestLoadKeybindings_RemovalWithUndispatchableChordAccepted verifies the
// removal exemption: a `command = ""` entry only ever deletes an existing
// binding, so an undispatchable chord is a harmless no-op and must NOT be
// rejected — it flows into overrides as a removal.
func TestLoadKeybindings_RemovalWithUndispatchableChordAccepted(t *testing.T) {
	path := writeKeybindings(t, `
[[binding]]
chord = "C-g Left"
command = ""
`)
	overrides, rejected, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if len(rejected) != 0 {
		t.Fatalf("a removal entry must never be rejected; got %v", rejected)
	}
	if len(overrides) != 1 {
		t.Fatalf("removal entry should produce exactly one override, got %d: %+v", len(overrides), overrides)
	}
	if overrides[0].Command != "" {
		t.Errorf("removal override Command = %q, want empty", overrides[0].Command)
	}
	if overrides[0].Chord != "C-g Left" {
		t.Errorf("removal override Chord = %q, want %q", overrides[0].Chord, "C-g Left")
	}
}

// TestLoadKeybindings_AcceptsDispatchableChords guards the accept side: chords
// the dispatcher CAN emit (a named special atom, a bare digit, a Ctrl-letter
// step, Tab) must pass through into overrides with nothing rejected.
func TestLoadKeybindings_AcceptsDispatchableChords(t *testing.T) {
	path := writeKeybindings(t, `
[[binding]]
chord = "C-g Space"
command = "Kill Pane"

[[binding]]
chord = "C-g 5"
command = "Zoom Focused Pane"

[[binding]]
chord = "C-g C-c"
command = "New Window"

[[binding]]
chord = "C-g Tab"
command = "Next Window"
`)
	overrides, rejected, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if len(rejected) != 0 {
		t.Fatalf("dispatchable chords must not be rejected; got %v", rejected)
	}
	if len(overrides) != 4 {
		t.Fatalf("expected 4 accepted overrides, got %d: %+v", len(overrides), overrides)
	}
}
