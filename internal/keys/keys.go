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
	for {
		switch {
		case strings.HasPrefix(s, "C-"):
			st.Ctrl = true
			s = s[2:]
		case strings.HasPrefix(s, "S-"):
			st.Shift = true
			s = s[2:]
		case strings.HasPrefix(s, "A-"), strings.HasPrefix(s, "M-"):
			st.Alt = true
			s = s[2:]
		default:
			goto done
		}
	}
done:
	if s == "" {
		return Step{}, errors.New("modifier without atom")
	}
	st.Atom = canonicalize(s)
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

// Format returns a canonical string form of c. Round-trips through
// Parse for well-formed input.
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
		sb.WriteString(s.Atom)
		parts[i] = sb.String()
	}
	return strings.Join(parts, " ")
}
