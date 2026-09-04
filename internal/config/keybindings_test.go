package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/keys"
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

func TestLoadKeybindingsAcceptsEffectiveKeySurface(t *testing.T) {
	path := writeKeybindings(t, `
[[binding]]
chord = "<prefix> Left"
command = "Kill Pane"

[[binding]]
chord = "C-g A-F5"
command = "Zoom Focused Pane"

[[binding]]
chord = "C-b C-Space"
command = "New Window"

[[binding]]
chord = "<prefix> 界"
command = "Next Window"
`)
	overrides, diagnostics, err := LoadKeybindings(path, "C-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 4 {
		t.Fatalf("got %d overrides, want 4: %+v", len(overrides), overrides)
	}
	for _, override := range overrides {
		if !strings.HasPrefix(override.Chord, "C-g ") {
			t.Errorf("override was not normalized to factory prefix: %+v", override)
		}
	}
	if len(diagnostics) != 1 || diagnostics[0].Severity != "warning" {
		t.Fatalf("Alt binding should produce one portability warning: %+v", diagnostics)
	}
}

func TestLoadKeybindingsRejectsInvalidGrammar(t *testing.T) {
	path := writeKeybindings(t, `
[[binding]]
chord = "C-x a"
command = "Kill Pane"

[[binding]]
chord = "<prefix> HyperDrive"
command = "Zoom Focused Pane"

[[binding]]
chord = "<prefix> a b"
command = "New Window"
`)
	overrides, diagnostics, err := LoadKeybindings(path, "C-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Fatalf("invalid entries entered overrides: %+v", overrides)
	}
	if len(diagnostics) != 3 {
		t.Fatalf("got %d diagnostics, want 3: %+v", len(diagnostics), diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != "error" {
			t.Errorf("invalid entry was not an error: %+v", diagnostic)
		}
	}
}

func TestLoadKeybindingsPrefixSpellingsNormalizeIdentically(t *testing.T) {
	path := writeKeybindings(t, `
[[binding]]
chord = "C-g a"
command = "Kill Pane"

[[binding]]
chord = "C-b b"
command = "Zoom Focused Pane"

[[binding]]
chord = "<prefix> c"
command = "New Window"
`)
	overrides, diagnostics, err := LoadKeybindings(path, "C-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	want := []string{"C-g a", "C-g b", "C-g c"}
	for i := range want {
		if overrides[i].Chord != want[i] {
			t.Errorf("override %d = %q, want %q", i, overrides[i].Chord, want[i])
		}
	}
}

func TestLoadKeybindingsAliasesCanonicalizeBeforeCollision(t *testing.T) {
	path := writeKeybindings(t, `
[[binding]]
chord = "<prefix> C-i"
command = "Kill Pane"

[[binding]]
chord = "<prefix> Tab"
command = "Zoom Focused Pane"
`)
	overrides, diagnostics, err := LoadKeybindings(path, "C-g")
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected syntax diagnostics: %+v", diagnostics)
	}
	if overrides[0].Chord != "C-g Tab" || overrides[1].Chord != "C-g Tab" {
		t.Fatalf("aliases did not collapse: %+v", overrides)
	}
	reg := commands.Defaults()
	diagnostics = reg.ApplyOverrides(overrides)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[len(diagnostics)-1].Reason, "collision") {
		t.Fatalf("collapsed alias collision was not diagnosed: %+v", diagnostics)
	}
}

func TestLoadKeybindingsMissingFile(t *testing.T) {
	overrides, diagnostics, err := LoadKeybindings(filepath.Join(t.TempDir(), "missing.toml"), "C-g")
	if err != nil || len(overrides) != 0 || len(diagnostics) != 0 {
		t.Fatalf("missing file = overrides:%v diagnostics:%v err:%v", overrides, diagnostics, err)
	}
}

func TestPortablePresetIsValidAndLetterOnly(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "keybindings-portable.toml")
	overrides, diagnostics, err := LoadKeybindings(path, "C-g")
	if err != nil {
		t.Fatal(err)
	}
	reg := commands.Defaults()
	diagnostics = append(diagnostics, reg.ApplyOverrides(overrides)...)
	if len(diagnostics) != 0 {
		t.Fatalf("portable preset diagnostics: %+v", diagnostics)
	}
	seen := map[string]bool{}
	for _, override := range overrides {
		parsed, err := keys.Parse(override.Chord)
		if err != nil {
			t.Fatal(err)
		}
		step := parsed.Steps[1]
		letter := len(step.Atom) == 1 &&
			((step.Atom[0] >= 'a' && step.Atom[0] <= 'z') ||
				(step.Atom[0] >= 'A' && step.Atom[0] <= 'Z'))
		if !letter || step.Ctrl || step.Alt || step.Shift {
			t.Errorf("portable preset contains non-letter second step: %q", override.Chord)
		}
		if seen[override.Chord] {
			t.Errorf("portable preset duplicates %q", override.Chord)
		}
		seen[override.Chord] = true
	}
}
