package app

import (
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/notification"
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
// auto-dismiss after 6s. The 180s timer is tracked so a quit before it
// fires can cancel it (avoids touching a torn-down Desktop).
func (m *Mux) eggTea() {
	m.notice("Tea timer", "Steeping… 3 minutes.", 6*time.Second)
	m.scheduleEgg(180*time.Second, func() {
		m.notice("Tea ready", "Your tea is ready.", 8*time.Second)
	})
}

// eggRot13 rotates the focused pane's terminal output for 10 seconds.
// The OnFeed filter is installed once at spawn (wireTerminalCallbacks)
// and merely consults pane.Rot13; flipping that atomic — rather than
// swapping OnFeed on a running terminal — keeps the read loop race-free.
func (m *Mux) eggRot13() {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil || ws.Focus.Pane == nil {
		return
	}
	pane := ws.Focus.Pane
	pane.Rot13.Store(true)
	m.notice("rot13", "Output rotated for 10 s.", 4*time.Second)
	m.scheduleEgg(10*time.Second, func() { pane.Rot13.Store(false) })
}

// scheduleEgg fires fn after d, tracking the timer so StopEggs can
// cancel everything pending at shutdown. fn is marshalled onto the UI
// goroutine (via CallSoon) because egg callbacks touch UI state — the
// Desktop (notifications) and pane flags — which is not safe to mutate
// from the timer goroutine.
func (m *Mux) scheduleEgg(d time.Duration, fn func()) {
	var t *time.Timer
	t = time.AfterFunc(d, func() {
		views.CallSoon(fn)
		m.eggMu.Lock()
		defer m.eggMu.Unlock()
		for i, x := range m.eggTimers {
			if x == t {
				m.eggTimers = append(m.eggTimers[:i], m.eggTimers[i+1:]...)
				return
			}
		}
	})
	m.eggMu.Lock()
	m.eggTimers = append(m.eggTimers, t)
	m.eggMu.Unlock()
}

// StopEggs cancels every pending easter-egg timer. Called from the
// process-exit defer so we don't touch a torn-down Desktop after Run
// returns.
func (m *Mux) StopEggs() {
	m.eggMu.Lock()
	pending := m.eggTimers
	m.eggTimers = nil
	m.eggMu.Unlock()
	for _, t := range pending {
		t.Stop()
	}
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
