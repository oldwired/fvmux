package keys

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/term"
)

func TestParseSimple(t *testing.T) {
	c, err := Parse("C-g c")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Steps) != 2 {
		t.Fatalf("len(steps) = %d", len(c.Steps))
	}
	if !c.Steps[0].Ctrl || c.Steps[0].Atom != "g" {
		t.Errorf("step 0: %+v", c.Steps[0])
	}
	if c.Steps[1].Ctrl || c.Steps[1].Atom != "c" {
		t.Errorf("step 1: %+v", c.Steps[1])
	}
}

func TestParseMultiMod(t *testing.T) {
	c, err := Parse("C-A-S-Tab")
	if err != nil {
		t.Fatal(err)
	}
	s := c.Steps[0]
	if !s.Ctrl || !s.Alt || !s.Shift || s.Atom != "tab" {
		t.Errorf("got %+v", s)
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseModWithoutAtom(t *testing.T) {
	if _, err := Parse("C-"); err == nil {
		t.Error("expected error for bare modifier")
	}
}

func TestFormatRoundTrip(t *testing.T) {
	cases := []string{
		"C-g c",
		"C-x C-c",
		"S-F1",
		"C-A-tab",
	}
	for _, in := range cases {
		c, err := Parse(in)
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		// Normalize input for comparison.
		out := Format(c)
		c2, err := Parse(out)
		if err != nil {
			t.Fatalf("re-parse %q: %v", out, err)
		}
		if Format(c2) != out {
			t.Errorf("not stable: %q → %q → %q", in, out, Format(c2))
		}
	}
}

func TestParseRejectsUnknownMultiCharacterAtoms(t *testing.T) {
	for _, chord := range []string{"C-g HyperDrive", "C-g F13", "C-g PageSideways"} {
		if _, err := Parse(chord); err == nil {
			t.Errorf("Parse(%q) accepted an unknown atom", chord)
		}
	}
}

func TestControlAliasesCanonicalize(t *testing.T) {
	tests := map[string]string{
		"C-g C-i": "C-g Tab",
		"C-g C-j": "C-g Enter",
		"C-g C-m": "C-g Enter",
		"C-g C-[": "C-g Esc",
		"C-g C-?": "C-g Backspace",
		"C-g C-@": "C-g C-Space",
	}
	for input, want := range tests {
		parsed, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q): %v", input, err)
		}
		if got := Format(parsed); got != want {
			t.Errorf("Format(Parse(%q)) = %q, want %q", input, got, want)
		}
	}
}

func TestStepFromEventUsesEffectiveKey(t *testing.T) {
	tests := []struct {
		name  string
		event term.Event
		want  string
	}{
		{"unicode", term.Event{Kind: term.EventKey, Rune: '界'}, "界"},
		{"ctrl space", term.Event{Kind: term.EventKey, Rune: 0, Mods: term.ModCtrl}, "C-Space"},
		{"ctrl punctuation", term.Event{Kind: term.EventKey, Rune: 0x1c, Mods: term.ModCtrl}, `C-\`},
		{"modified arrow", term.Event{Kind: term.EventKey, Key: term.KeyLeft, Mods: term.ModCtrl | term.ModShift}, "C-S-Left"},
		{"modified navigation", term.Event{Kind: term.EventKey, Key: term.KeyPgDn, Mods: term.ModAlt}, "A-PgDn"},
		{"insert", term.Event{Kind: term.EventKey, Key: term.KeyIns}, "Insert"},
		{"F1", term.Event{Kind: term.EventKey, Key: term.KeyF1}, "F1"},
		{"F12 modifiers", term.Event{Kind: term.EventKey, Key: term.KeyF12, Mods: term.ModAlt | term.ModShift}, "A-S-F12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := drivers.FromTermEvent(tt.event)
			step, ok := StepFromEvent(&ev)
			if !ok {
				t.Fatalf("StepFromEvent rejected effective key %+v", ev.EffectiveKey())
			}
			got := FormatStep(step)
			if got != tt.want {
				t.Fatalf("FormatStep = %q, want %q (identity %+v)", got, tt.want, ev.EffectiveKey())
			}
			canonical, diagnostics := ValidateBindingChord("<prefix> "+got, "C-b")
			if canonical == "" {
				t.Fatalf("event output was rejected by validation: %+v", diagnostics)
			}
			for _, diagnostic := range diagnostics {
				if diagnostic.Severity == SeverityError {
					t.Fatalf("event output validation error: %+v", diagnostic)
				}
			}
		})
	}
}

func TestValidateBindingChordPrefixAndWarnings(t *testing.T) {
	for _, input := range []string{"C-g Left", "C-b Left", "<prefix> Left"} {
		got, diagnostics := ValidateBindingChord(input, "C-b")
		if got != "C-g Left" || hasError(diagnostics) {
			t.Errorf("ValidateBindingChord(%q) = %q, %+v", input, got, diagnostics)
		}
	}
	if _, diagnostics := ValidateBindingChord("C-x Left", "C-b"); !hasError(diagnostics) {
		t.Fatalf("foreign prefix was accepted: %+v", diagnostics)
	}
	_, diagnostics := ValidateBindingChord("<prefix> C-h", "C-g")
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Reason, "Backspace") {
		t.Fatalf("C-h portability warning missing: %+v", diagnostics)
	}
}

func hasError(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError {
			return true
		}
	}
	return false
}
