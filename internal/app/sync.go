package app

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/prefix"
)

// installSyncListener inserts a fallthrough OfPreProcess view that
// mirrors keyboard input across every pane in a window with SyncInput.
func (m *Mux) installSyncListener() {
	m.syncView = prefix.NewSyncView(m.broadcastIfSync)
	m.App.Desktop.Insert(m.syncView)
}

// broadcastIfSync is the SyncView callback. When the focused window has
// SyncInput enabled, the original keydown event is replayed (via value
// copies) into every other pane's Terminal.HandleEvent — fv-go does the
// PTY-byte encoding for us.
func (m *Mux) broadcastIfSync(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown {
		return
	}
	// Resize mode and copy mode consume their keys via listeners that
	// sit LATER in the OfPreProcess chain — without this gate their
	// navigation keys (l/h/j/k, arrows, Esc) would already have been
	// broadcast into every synced pane by the time they're consumed,
	// typing stray characters into remote shells.
	if m.resizeMode || m.copyMode.Active() {
		return
	}
	ws := m.currentWindow()
	if ws == nil || !ws.SyncInput {
		return
	}
	if ws.Focus == nil || ws.Focus.Pane == nil {
		return
	}
	focused := ws.Focus.Pane.Term
	saved := *ev
	ws.Root.Leaves(func(l *layout.PaneNode) {
		if l.Pane == nil || l.Pane.Term == nil || l.Pane.Term == focused {
			return
		}
		evCopy := saved
		m.sendSync(&evCopy, l.Pane.Term)
	})
}

// sendSync delivers one broadcast event to a single synced pane's terminal.
// Production feeds it to terminal.HandleEvent (fv-go does the PTY-byte
// encoding); the syncSend override, when set, intercepts delivery so a test
// can observe exactly which panes the sync gate reached.
func (m *Mux) sendSync(ev *drivers.Event, t *terminal.Terminal) {
	if m.syncSend != nil {
		m.syncSend(ev, t)
		return
	}
	t.HandleEvent(ev)
}

func (m *Mux) toggleSyncInput() {
	ws := m.currentWindow()
	if ws == nil {
		return
	}
	ws.SyncInput = !ws.SyncInput
	m.refreshStatusBar()
}
