// Package keys parses chord strings — the human-readable
// representation used by Command.Chord and by keybindings.toml.
//
// Grammar (informally):
//
//	chord  := step (SPACE step)*
//	step   := mod* atom
//	mod    := "C-" | "S-" | "A-" | "M-"
//	atom   := alpha | digit | symbol | special
//	special := "F1" | "F2" | … | "Tab" | "Esc" | "Enter" | "Space" | …
//
// The parser is intentionally tolerant: unknown atoms are reported as
// errors with their position. Sub-step 6's status-line PREFIX overlay
// renders the human-readable form back via Format(Chord).
package keys

import (
	"errors"
	"fmt"
	"strings"
)

// Chord is a parsed chord sequence — one or more Steps.
type Chord struct {
	Steps []Step
}

// Step is one keystroke: an Atom plus modifier flags.
type Step struct {
	Atom  string // canonical lowercase atom: "a", "tab", "f1", etc.
	Ctrl  bool
	Alt   bool
	Shift bool
}

// Parse parses s. Returns the parsed Chord or a descriptive error.
func Parse(s string) (Chord, error) {
	if s == "" {
		return Chord{}, errors.New("empty chord")
	}
	var out Chord
	for _, part := range strings.Fields(s) {
		st, err := parseStep(part)
		if err != nil {
			return Chord{}, fmt.Errorf("step %q: %w", part, err)
		}
		out.Steps = append(out.Steps, st)
	}
	if len(out.Steps) == 0 {
		return Chord{}, errors.New("no steps parsed")
	}
	return out, nil
}

func parseStep(s string) (Step, error) {
	var st Step
	// Modifiers are a two-char "X-" prefix, accepted case-insensitively so
	// "c-g" and "C-g" parse identically.
	for len(s) >= 2 && s[1] == '-' {
		switch s[0] {
		case 'C', 'c':
			st.Ctrl = true
		case 'S', 's':
			st.Shift = true
		case 'A', 'a', 'M', 'm':
			st.Alt = true
		default:
			return finishStep(st, s)
		}
		s = s[2:]
	}
	return finishStep(st, s)
}

func finishStep(st Step, atom string) (Step, error) {
	if atom == "" {
		return Step{}, errors.New("modifier without atom")
	}
	st.Atom = canonicalize(atom)
	// Ctrl+letter is case-insensitive at the terminal (Ctrl-G == Ctrl-g),
	// so normalise to a single lowercase form. Letter case still matters
	// for un-modified atoms ("C-g D" ≠ "C-g d").
	if st.Ctrl && len(st.Atom) == 1 && st.Atom[0] >= 'A' && st.Atom[0] <= 'Z' {
		st.Atom = strings.ToLower(st.Atom)
	}
	return st, nil
}

func canonicalize(s string) string {
	switch strings.ToLower(s) {
	case "space", "spc":
		return "space"
	case "enter", "return", "ret":
		return "enter"
	case "tab":
		return "tab"
	case "esc", "escape":
		return "esc"
	case "backspace", "bsp", "bs":
		return "backspace"
	case "delete", "del":
		return "delete"
	case "left":
		return "left"
	case "right":
		return "right"
	case "up":
		return "up"
	case "down":
		return "down"
	case "pgup", "pageup":
		return "pgup"
	case "pgdn", "pagedown":
		return "pgdn"
	case "home":
		return "home"
	case "end":
		return "end"
	}
	// Single character (most chord atoms).
	return s
}

// Format returns the canonical string form of c — the exact form the
// prefix dispatcher emits and that Command.Chord strings use, so a chord
// formatted here compares equal to a registry binding. Special atoms are
// rendered Title-case ("Tab", "Space"); single-character atoms keep their
// case (chords are case-sensitive: "C-g D" ≠ "C-g d"). Round-trips
// through Parse for well-formed input.
func Format(c Chord) string {
	parts := make([]string, len(c.Steps))
	for i, s := range c.Steps {
		var sb strings.Builder
		if s.Ctrl {
			sb.WriteString("C-")
		}
		if s.Alt {
			sb.WriteString("A-")
		}
		if s.Shift {
			sb.WriteString("S-")
		}
		sb.WriteString(displayAtom(s.Atom))
		parts[i] = sb.String()
	}
	return strings.Join(parts, " ")
}

// displayAtom maps a canonical (lowercase) special atom back to the
// Title-case form used in chord strings. Non-special atoms (single
// characters, digits, symbols) pass through untouched, preserving case.
func displayAtom(atom string) string {
	switch atom {
	case "space":
		return "Space"
	case "enter":
		return "Enter"
	case "tab":
		return "Tab"
	case "esc":
		return "Esc"
	case "backspace":
		return "Backspace"
	case "delete":
		return "Delete"
	case "left":
		return "Left"
	case "right":
		return "Right"
	case "up":
		return "Up"
	case "down":
		return "Down"
	case "pgup":
		return "PgUp"
	case "pgdn":
		return "PgDn"
	case "home":
		return "Home"
	case "end":
		return "End"
	}
	return atom
}

// Canonical normalises a chord string to the form Command.Chord and the
// prefix dispatcher use, so user-authored bindings (e.g. "c-g tab") match
// regardless of casing/spelling. Returns s unchanged if it can't be
// parsed — a malformed binding simply won't match anything, which is
// preferable to dropping it silently here.
func Canonical(s string) string {
	c, err := Parse(s)
	if err != nil {
		return s
	}
	return Format(c)
}
