package app

import (
	"github.com/oldwired/fv-go/pkg/fv/views"

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

func (m *Mux) resizeBy(dx, dy int) {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return
	}
	sz := ws.Frame.Size
	if dx != 0 {
		adjustNearestSplit(ws.Focus, views.SplitVertical, sz.X, dx)
	}
	if dy != 0 {
		adjustNearestSplit(ws.Focus, views.SplitHorizontal, sz.Y, dy)
	}
	m.rerender(ws)
}

// adjustNearestSplit walks up from leaf to the nearest enclosing split
// of the requested orientation, then nudges its Ratio by deltaCells /
// total. Clamped to (0.05, 0.95). Arrow semantics: h moves the boundary
// left, l moves it right, k up, j down — independent of which side
// the focused pane occupies.
func adjustNearestSplit(leaf *layout.PaneNode, orient views.SplitOrientation, total int, deltaCells int) {
	if total <= 0 || leaf == nil {
		return
	}
	n := leaf
	for n.Parent != nil {
		if n.Parent.Orientation == orient {
			r := n.Parent.Ratio + float64(deltaCells)/float64(total)
			if r < 0.05 {
				r = 0.05
			}
			if r > 0.95 {
				r = 0.95
			}
			n.Parent.Ratio = r
			return
		}
		n = n.Parent
	}
}
