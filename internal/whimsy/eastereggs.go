// Package whimsy holds fvmux's tasteful easter eggs and once-per-session
// surprises. Each entry is one of:
//
//   - A function the runtime calls at a specific moment (e.g.,
//     FridayAfterFive returns true on Friday after 17:00 local).
//   - A `:command-prompt` handler bound under sub-step 5's command
//     palette (`:tea`, `:konami`, `:rot13`).
//
// Step 12 ships the predicates and helpers; wiring them to live UI is
// the runtime's job (see internal/app/mux.go for the prompt dispatch
// table once it lands).
package whimsy

import (
	"strings"
	"time"
)

// FridayAfterFive reports whether now is Friday at or after 17:00 local.
// The status-bar middle slot flashes "ship it" for 4 seconds whenever a
// new window opens during this window.
func FridayAfterFive(now time.Time) bool {
	if now.Weekday() != time.Friday {
		return false
	}
	return now.Hour() >= 17
}

// HomeGlyphFor returns a small home glyph when title is the literal
// "home". Surfaced in the status-line window list as a discoverable
// quirk for users who name a window "home".
func HomeGlyphFor(title string) string {
	if strings.EqualFold(strings.TrimSpace(title), "home") {
		return "🏠 "
	}
	return ""
}

// Rot13 rotates ASCII letters by 13 places, leaving every other byte
// alone. Used by the `:rot13` command-prompt handler when it briefly
// rotates the focused pane's incoming bytes.
func Rot13(in []byte) []byte {
	out := make([]byte, len(in))
	for i, b := range in {
		switch {
		case b >= 'a' && b <= 'z':
			out[i] = 'a' + (b-'a'+13)%26
		case b >= 'A' && b <= 'Z':
			out[i] = 'A' + (b-'A'+13)%26
		default:
			out[i] = b
		}
	}
	return out
}

// Taglines for the cheatsheet footer. The generator picks one at random
// per build.
var Taglines = []string{
	"a multiplexer for the rest of us.",
	"windows in your windows.",
	"now with hex.",
	"like tmux, but with weather.",
}
