package app

import (
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/widgets/notification"

	"github.com/oldwired/fvmux/internal/whimsy"
)

// handleEgg checks one of fvmux's built-in `:command` easter eggs. Returns
// true if cmd was recognised (caller should not fall through to shell).
func (m *Mux) handleEgg(cmd string) bool {
	switch strings.ToLower(cmd) {
	case ":tea":
		m.eggTea()
		return true
	case ":konami":
		m.konamiOn = !m.konamiOn
		m.notice("Konami", flashKonamiText(m.konamiOn), 4*time.Second)
		return true
	case ":rot13":
		m.eggRot13()
		return true
	}
	return false
}

// eggTea fires a notification, waits 180s, then fires another. Both
// auto-dismiss after 6s. Tracking 180s with time.AfterFunc keeps the
// goroutine cost negligible.
func (m *Mux) eggTea() {
	m.notice("Tea timer", "Steeping… 3 minutes.", 6*time.Second)
	time.AfterFunc(180*time.Second, func() {
		m.notice("Tea ready", "Your tea is ready.", 8*time.Second)
	})
}

// eggRot13 swaps the focused pane's terminal output through a rot13
// filter for 10 seconds. Uses fv-go's Terminal.OnFeed hook.
func (m *Mux) eggRot13() {
	t := m.FocusedTerminal()
	if t == nil {
		return
	}
	prev := t.OnFeed
	t.OnFeed = func(in []byte) []byte { return whimsy.Rot13(in) }
	m.notice("rot13", "Output rotated for 10 s.", 4*time.Second)
	time.AfterFunc(10*time.Second, func() {
		// Restore the previous hook — typically nil.
		if t != nil {
			t.OnFeed = prev
		}
	})
}

// notice is a thin wrapper around widgets/notification.New that lives
// in the top-right slot.
func (m *Mux) notice(title, body string, ttl time.Duration) {
	n := notification.New(&m.App.Desktop.Group, title, body,
		notification.PosTopRight, 30, ttl)
	m.App.Desktop.Insert(n)
}

func flashKonamiText(on bool) string {
	if on {
		return "Sparkline flipped. (Run `:konami` again to undo.)"
	}
	return "Sparkline restored."
}
