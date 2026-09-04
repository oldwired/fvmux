package app

import (
	"time"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/prefix"
)

// enterResizeMode installs a sticky pre-process listener that captures
// every keypress until Esc. While in mode, the status bar shows
// *RESIZE*.
func (m *Mux) enterResizeMode() {
	if m.resizeMode {
		return
	}
	m.resizeMode = true
	// Suspend the prefix listener: it sits ahead of the resize view in
	// z-order, so without this it would arm on (and swallow) the prefix
	// key instead of letting resize mode absorb the keystroke.
	if m.prefix != nil {
		m.prefix.SetSuspended(true)
	}
	m.resizeView = prefix.NewResizeView(m.handleResizeKey)
	m.App.Desktop.Insert(m.resizeView)
	m.refreshStatusBar()
}

// exitResizeMode removes the sticky listener.
func (m *Mux) exitResizeMode() {
	if !m.resizeMode {
		return
	}
	m.resizeMode = false
	if m.resizeView != nil {
		m.App.Desktop.Delete(m.resizeView)
		m.resizeView = nil
	}
	if m.prefix != nil {
		m.prefix.SetSuspended(false)
	}
	m.refreshStatusBar()
}

// handleResizeKey processes one keystroke in resize mode. Returns true
// when handled (consume the event); false leaves the event alone, but
// in practice resize mode swallows everything to avoid surprise
// keystrokes hitting the focused pane.
func (m *Mux) handleResizeKey(r rune, special string) bool {
	if special == "Esc" {
		m.exitResizeMode()
		return true
	}
	delta := 1
	if r >= 'A' && r <= 'Z' {
		delta = 5
	}
	switch r {
	case 'h', 'H':
		m.resizeBy(-delta, 0)
	case 'l', 'L':
		m.resizeBy(+delta, 0)
	case 'k', 'K':
		m.resizeBy(0, -delta)
	case 'j', 'J':
		m.resizeBy(0, +delta)
	}
	// Always consume — accidental keystrokes in resize mode must NOT
	// reach the focused PTY.
	return true
}

func (m *Mux) resizeBy(dx, dy int) bool {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return false
	}
	moved := false
	direction := "that direction"
	if dx != 0 {
		dir := layout.Right
		direction = "right"
		if dx < 0 {
			dir = layout.Left
			direction = "left"
			dx = -dx
		}
		moved = layout.ResizeToward(ws.Focus, dir, dx)
	}
	if dy != 0 {
		dir := layout.Down
		direction = "down"
		if dy < 0 {
			dir = layout.Up
			direction = "up"
			dy = -dy
		}
		moved = layout.ResizeToward(ws.Focus, dir, dy) || moved
	}
	if !moved {
		m.setFlash("no movable divider to the "+direction, 1200*time.Millisecond, flashPrioResize)
		m.refreshStatusBar()
		return false
	}
	m.rerender(ws)
	return true
}
