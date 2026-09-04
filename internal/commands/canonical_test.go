package commands_test

import (
	"testing"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/keys"
)

// TestDefaultChordsAreCanonical is the contract that makes user-override
// canonicalisation correct: every built-in Command.Chord must already be
// in keys' canonical form. If this holds, a user binding run through
// keys.Canonical lands on exactly the string the registry indexes by, so
// "c-g tab" and "C-g Tab" both resolve to the same command.
func TestDefaultChordsAreCanonical(t *testing.T) {
	reg := commands.Defaults()
	seen := map[string]string{}
	for _, c := range reg.All() {
		if c.Chord == "" {
			continue
		}
		if got := keys.Canonical(c.Chord); got != c.Chord {
			t.Errorf("command %q chord %q is not canonical: keys.Canonical → %q",
				c.Name, c.Chord, got)
		}
		if previous := seen[c.Chord]; previous != "" {
			t.Errorf("commands %q and %q share default chord %q", previous, c.Name, c.Chord)
		}
		seen[c.Chord] = c.Name
		parsed, err := keys.Parse(c.Chord)
		if err != nil || keys.Format(parsed) != c.Chord {
			t.Errorf("command %q chord %q does not round-trip: parsed=%+v err=%v", c.Name, c.Chord, parsed, err)
		}
	}
}

// TestCanonicalNormalisesUserSpellings confirms the spellings a user
// might type all collapse onto the canonical registry form.
func TestCanonicalNormalisesUserSpellings(t *testing.T) {
	cases := map[string]string{
		"c-g tab":   "C-g Tab",
		"C-g Tab":   "C-g Tab",
		"c-g space": "C-g Space",
		"C-G x":     "C-g x",   // modifier case normalises…
		"C-g D":     "C-g D",   // …but the atom's case is preserved.
		"c-g c-c":   "C-g C-c", // ctrl-modified second key.
		"C-g %":     "C-g %",
	}
	for in, want := range cases {
		if got := keys.Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}
