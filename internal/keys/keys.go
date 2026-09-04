// Package keys owns the canonical, user-visible representation of keyboard
// chords. Runtime events, configuration validation, registry lookup, and help
// generation all pass through this package so they cannot disagree about what
// a key means.
package keys

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/term"
)

// Chord is a parsed chord sequence — one or more Steps.
type Chord struct {
	Steps []Step
}

// Step is one keystroke: an Atom plus modifier flags. Named atoms use their
// canonical lowercase spelling ("tab", "f1", "pgup", ...); single-rune
// atoms retain case because "x" and "X" are distinct terminal input.
type Step struct {
	Atom  string
	Ctrl  bool
	Alt   bool
	Shift bool
}

// Severity classifies a binding validation diagnostic.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Diagnostic explains why a chord is invalid or potentially non-portable.
type Diagnostic struct {
	Severity Severity
	Reason   string
}

// Parse parses s using the exact grammar supported by runtime dispatch.
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
	for len(s) >= 2 && s[1] == '-' {
		switch s[0] {
		case 'C', 'c':
			if st.Ctrl {
				return Step{}, errors.New("duplicate Ctrl modifier")
			}
			st.Ctrl = true
		case 'S', 's':
			if st.Shift {
				return Step{}, errors.New("duplicate Shift modifier")
			}
			st.Shift = true
		case 'A', 'a', 'M', 'm':
			if st.Alt {
				return Step{}, errors.New("duplicate Alt/Meta modifier")
			}
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
	canonical, ok := canonicalAtom(atom)
	if !ok {
		return Step{}, fmt.Errorf("unknown key atom %q", atom)
	}
	st.Atom = canonical

	// Classic terminal protocols collapse these Ctrl spellings onto named
	// keys. Canonicalizing here lets duplicate ownership be detected after
	// aliases collapse instead of accepting a binding runtime can never emit.
	if st.Ctrl && !st.Alt && !st.Shift {
		switch strings.ToLower(st.Atom) {
		case "i":
			return Step{Atom: "tab"}, nil
		case "j", "m":
			return Step{Atom: "enter"}, nil
		case "[":
			return Step{Atom: "esc"}, nil
		case "?":
			return Step{Atom: "backspace"}, nil
		case "@":
			return Step{Atom: "space", Ctrl: true}, nil
		}
	}

	// Ctrl+letter is case-insensitive at the terminal. Unmodified rune case
	// remains significant ("D" and "d" are different bindings).
	if st.Ctrl && utf8.RuneCountInString(st.Atom) == 1 {
		r, _ := utf8.DecodeRuneInString(st.Atom)
		if unicode.IsLetter(r) {
			st.Atom = string(unicode.ToLower(r))
		}
	}
	return st, nil
}

func canonicalAtom(s string) (string, bool) {
	switch strings.ToLower(s) {
	case "space", "spc":
		return "space", true
	case "enter", "return", "ret":
		return "enter", true
	case "tab":
		return "tab", true
	case "esc", "escape":
		return "esc", true
	case "backspace", "bsp", "bs":
		return "backspace", true
	case "delete", "del":
		return "delete", true
	case "insert", "ins":
		return "insert", true
	case "left":
		return "left", true
	case "right":
		return "right", true
	case "up":
		return "up", true
	case "down":
		return "down", true
	case "pgup", "pageup":
		return "pgup", true
	case "pgdn", "pagedown":
		return "pgdn", true
	case "home":
		return "home", true
	case "end":
		return "end", true
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "f") {
		if n, err := strconv.Atoi(lower[1:]); err == nil && n >= 1 && n <= 12 {
			return "f" + strconv.Itoa(n), true
		}
	}
	if utf8.RuneCountInString(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		if unicode.IsPrint(r) {
			return s, true
		}
	}
	return "", false
}

// StepFromEvent converts a keyboard event through fv-go's normalized,
// lossless key identity. It is the only runtime event-to-binding path.
func StepFromEvent(ev *drivers.Event) (Step, bool) {
	if ev == nil {
		return Step{}, false
	}
	return StepFromIdentity(ev.EffectiveKey())
}

// StepFromIdentity converts fv-go's normalized key identity to a binding
// step. The result always formats to a spelling accepted by Parse.
func StepFromIdentity(id drivers.KeyIdentity) (Step, bool) {
	if !id.Valid {
		return Step{}, false
	}
	st := Step{
		Ctrl:  id.Mods.Has(term.ModCtrl),
		Alt:   id.Mods.Has(term.ModAlt),
		Shift: id.Mods.Has(term.ModShift),
	}
	if atom, ok := atomFromKey(id.Key); ok {
		st.Atom = atom
		return normalizeIdentityStep(st)
	}
	if id.Key != term.KeyNone {
		return Step{}, false
	}
	if id.Rune == 0 && st.Ctrl {
		st.Atom = "space"
		return normalizeIdentityStep(st)
	}
	if st.Ctrl {
		switch id.Rune {
		case 0x08:
			st.Atom = "h"
		case 0x09:
			return Step{Atom: "tab"}, true
		case 0x0a, 0x0d:
			return Step{Atom: "enter"}, true
		case 0x1b:
			return Step{Atom: "esc"}, true
		case 0x1c:
			st.Atom = `\`
		case 0x1d:
			st.Atom = "]"
		case 0x1e:
			st.Atom = "^"
		case 0x1f:
			st.Atom = "_"
		case 0x7f:
			return Step{Atom: "backspace"}, true
		default:
			if id.Rune >= 1 && id.Rune <= 26 {
				st.Atom = string(rune('a' + id.Rune - 1))
			} else if id.Rune == '@' {
				st.Atom = "space"
			} else if id.Rune == '?' {
				return Step{Atom: "backspace"}, true
			} else if unicode.IsPrint(id.Rune) {
				st.Atom = string(unicode.ToLower(id.Rune))
			} else {
				return Step{}, false
			}
		}
	} else {
		if !unicode.IsPrint(id.Rune) {
			return Step{}, false
		}
		st.Atom = string(id.Rune)
	}
	return normalizeIdentityStep(st)
}

func normalizeIdentityStep(st Step) (Step, bool) {
	parsed, err := parseStep(FormatStep(st))
	if err != nil {
		return Step{}, false
	}
	return parsed, true
}

func atomFromKey(key term.Key) (string, bool) {
	switch key {
	case term.KeyEnter:
		return "enter", true
	case term.KeyTab:
		return "tab", true
	case term.KeyBackspace:
		return "backspace", true
	case term.KeyEsc:
		return "esc", true
	case term.KeySpace:
		return "space", true
	case term.KeyUp:
		return "up", true
	case term.KeyDown:
		return "down", true
	case term.KeyLeft:
		return "left", true
	case term.KeyRight:
		return "right", true
	case term.KeyHome:
		return "home", true
	case term.KeyEnd:
		return "end", true
	case term.KeyPgUp:
		return "pgup", true
	case term.KeyPgDn:
		return "pgdn", true
	case term.KeyIns:
		return "insert", true
	case term.KeyDel:
		return "delete", true
	}
	if key >= term.KeyF1 && key <= term.KeyF12 {
		return "f" + strconv.Itoa(int(key-term.KeyF1)+1), true
	}
	return "", false
}

// FormatStep returns the canonical registry spelling for one key step.
func FormatStep(s Step) string {
	var b strings.Builder
	if s.Ctrl {
		b.WriteString("C-")
	}
	if s.Alt {
		b.WriteString("A-")
	}
	if s.Shift {
		b.WriteString("S-")
	}
	b.WriteString(displayAtom(s.Atom))
	return b.String()
}

// Format returns the canonical string form of c.
func Format(c Chord) string {
	parts := make([]string, len(c.Steps))
	for i, s := range c.Steps {
		parts[i] = FormatStep(s)
	}
	return strings.Join(parts, " ")
}

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
	case "insert":
		return "Insert"
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
	if strings.HasPrefix(atom, "f") {
		if _, err := strconv.Atoi(atom[1:]); err == nil {
			return strings.ToUpper(atom)
		}
	}
	return atom
}

// Canonical normalizes a chord string. Invalid input is returned unchanged;
// callers that accept user input should use ValidateBindingChord so the error
// is never hidden.
func Canonical(s string) string {
	c, err := Parse(s)
	if err != nil {
		return s
	}
	return Format(c)
}

// ValidateBindingChord validates fvmux's v1 binding model: the configured
// prefix plus exactly one second key. The first step may be written as the
// factory C-g token, the active prefix token, or <prefix>; successful results
// are normalized back into factory C-g space before Registry overrides apply.
func ValidateBindingChord(chord, activePrefix string) (string, []Diagnostic) {
	parts := strings.Fields(chord)
	if len(parts) != 2 {
		return "", []Diagnostic{{
			Severity: SeverityError,
			Reason:   "bindings must contain exactly two steps: <prefix> plus one key",
		}}
	}
	first := parts[0]
	if !strings.EqualFold(first, "<prefix>") &&
		!strings.EqualFold(first, "C-g") &&
		!strings.EqualFold(first, activePrefix) {
		return "", []Diagnostic{{
			Severity: SeverityError,
			Reason: fmt.Sprintf("first step %q is not <prefix>, factory C-g, or active prefix %s",
				first, activePrefix),
		}}
	}

	parsed, err := Parse("C-g " + parts[1])
	if err != nil {
		return "", []Diagnostic{{Severity: SeverityError, Reason: err.Error()}}
	}
	canonical := Format(parsed)
	var diagnostics []Diagnostic
	if strings.EqualFold(parts[1], "C-h") {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Reason:   "C-h may arrive as Backspace in some host terminal configurations",
		})
	}
	second := parsed.Steps[1]
	if second.Alt {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Reason:   "Alt/Meta bindings depend on terminal Option/Meta settings and keyboard layout",
		})
	}
	return canonical, diagnostics
}
