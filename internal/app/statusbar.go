package app

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/statusbar"
)

// snapshotStatus assembles the live state from the Mux into a
// statusbar.Snapshot. Cheap; called from the 1 s ticker and after any
// state change.
func (m *Mux) snapshotStatus() statusbar.Snapshot {
	curView := m.App.Desktop.Current()
	curWS := m.currentWindow()

	out := statusbar.Snapshot{
		SessionName: m.Opts.SessionName,
		ResizeMode:  m.resizeMode,
		PrefixArmed: m.prefix != nil && m.prefix.Armed(),
	}
	if curWS != nil {
		out.SyncInput = curWS.SyncInput
		if curWS.Focus != nil && curWS.Focus.Pane != nil {
			out.FocusedTitle = curWS.Focus.Pane.Title
			out.FocusedCWD = curWS.Focus.Pane.CWD
		}
	}

	for _, key := range m.windowOrder {
		ws := m.windows[key]
		if ws == nil {
			continue
		}
		title := ws.UserTitle
		if title == "" {
			title = ws.ShellTitle
		}
		if title == "" {
			title = ws.Title
		}
		entry := statusbar.WindowEntry{
			Number:  ws.Number,
			Title:   title,
			Focused: key == curView,
		}
		ws.Root.Leaves(func(l *layout.PaneNode) {
			if l.Pane == nil {
				return
			}
			if l.Pane.IsBellActive() {
				entry.Bell = true
			}
			if l.Pane.IsActivityRecent() {
				entry.Activity = true
			}
		})
		out.Windows = append(out.Windows, entry)
	}
	return out
}

// refreshStatusBar pushes a fresh snapshot to the bar.
func (m *Mux) refreshStatusBar() {
	if m.Opts.StatusBar == nil {
		return
	}
	m.Opts.StatusBar.Refresh(m.snapshotStatus())
}

// wireTickerAction registers CmdTickerRedraw's action so the 1 s
// status refresh runs through the standard dispatch path.
func (m *Mux) wireTickerAction() {
	if c := m.Reg.ByID(commands.CmdTickerRedraw); c != nil {
		c.Action = func(*commands.Ctx) {
			m.refreshStatusBar()
			views.MarkDirty()
		}
	}
}

// StartTicker spins up a goroutine that posts CmdTickerRedraw every
// second; the main event loop picks it up and refreshes the status
// bar (and re-evaluates bell/activity windows).
func (m *Mux) StartTicker() {
	if m.tickerStop != nil {
		return
	}
	m.tickerStop = make(chan struct{})
	go func() {
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				m.App.PostEvent(drivers.Event{
					What:    consts.EvCommand,
					Command: commands.CmdTickerRedraw,
				})
			case <-m.tickerStop:
				return
			}
		}
	}()
}

// StopTicker stops the periodic refresh.
func (m *Mux) StopTicker() {
	if m.tickerStop == nil {
		return
	}
	close(m.tickerStop)
	m.tickerStop = nil
}
