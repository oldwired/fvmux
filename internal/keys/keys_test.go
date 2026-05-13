package keys

import "testing"

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
