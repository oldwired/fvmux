package app

import (
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/statusbar"
	"github.com/oldwired/fvmux/internal/sysmon"
	"github.com/oldwired/fvmux/internal/whimsy"
)

// snapshotStatus assembles the live state from the Mux into a
// statusbar.Snapshot. Cheap; called from the 1 s ticker and after any
// state change.
func (m *Mux) snapshotStatus() statusbar.Snapshot {
	curView := m.App.Desktop.Current()
	curWS := m.currentWindow()

	cpu := sysmon.CPUHistory()
	if m.konamiOn {
		cpu = flipHistory(cpu)
	}
	out := statusbar.Snapshot{
		SessionName: m.Opts.SessionName,
		ResizeMode:  m.resizeMode,
		PrefixArmed: m.prefix != nil && m.prefix.Armed(),
		HideClock:   m.hideClock,
		CPUHistory:  cpu,
		RAMUsage:    sysmon.RAMUsage(),
	}
	if curWS != nil {
		out.SyncInput = curWS.SyncInput
		if curWS.Focus != nil && curWS.Focus.Pane != nil {
			out.FocusedTitle = curWS.Focus.Pane.DisplayTitle()
			out.FocusedCWD = curWS.Focus.Pane.CWD
		}
	}
	// Ctrl-G q flash takes over the focused-title slot temporarily.
	if !m.flashUntil.IsZero() && time.Now().Before(m.flashUntil) {
		out.FocusedTitle = m.flashText
		out.FocusedCWD = ""
	}

	for _, key := range m.windowOrder {
		ws := m.windows[key]
		if ws == nil {
			continue
		}
		title := ws.displayTitle()
		title = whimsy.HomeGlyphFor(title) + title
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

// flipHistory mirrors each sample around 0.5 — used by :konami to
// turn the CPU sparkline upside-down without breaking the renderer's
// [0,1] expectations.
func flipHistory(h []float64) []float64 {
	if len(h) == 0 {
		return h
	}
	out := make([]float64, len(h))
	for i, v := range h {
		out[i] = 1 - v
	}
	return out
}

// refreshStatusBar pushes a fresh snapshot to the bar.
func (m *Mux) refreshStatusBar() {
	m.refreshMenuAvailability()
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
