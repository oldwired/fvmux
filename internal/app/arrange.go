package app

import (
	"math"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// fvmuxWindows returns the *views.Window children of the desktop in
// z-order (back-to-front). Skips the wallpaper Background and any
// 0-size OfPreProcess helpers (prefix listener, sync, mouse) — these
// live on the desktop too but aren't user windows.
func (m *Mux) fvmuxWindows() []*views.Window {
	out := make([]*views.Window, 0, len(m.windowOrder))
	for _, c := range m.App.Desktop.Children {
		if c == m.App.Desktop.Background {
			continue
		}
		w, ok := c.(*views.Window)
		if !ok {
			continue
		}
		out = append(out, w)
	}
	return out
}

// tile arranges every window in a near-square grid filling the desktop.
// Ported from fvdemo/winmenu.go; the algorithm is straight Turbo Vision.
func (m *Mux) tile() {
	ws := m.fvmuxWindows()
	if len(ws) == 0 {
		return
	}
	cols := int(math.Ceil(math.Sqrt(float64(len(ws)))))
	if cols < 1 {
		cols = 1
	}
	rows := (len(ws) + cols - 1) / cols
	m.tileGrid(ws, cols, rows)
	m.refreshStatusBar()
}

func (m *Mux) tileHorizontal() {
	ws := m.fvmuxWindows()
	if len(ws) == 0 {
		return
	}
	m.tileGrid(ws, 1, len(ws))
	m.refreshStatusBar()
}

func (m *Mux) tileVertical() {
	ws := m.fvmuxWindows()
	if len(ws) == 0 {
		return
	}
	m.tileGrid(ws, len(ws), 1)
	m.refreshStatusBar()
}

func (m *Mux) tileGrid(ws []*views.Window, cols, rows int) {
	desktopW, desktopH := m.App.Desktop.Size.X, m.App.Desktop.Size.Y
	cellW := desktopW / cols
	cellH := desktopH / rows
	for i, w := range ws {
		col := i % cols
		row := i / cols
		x0 := col * cellW
		y0 := row * cellH
		x1 := x0 + cellW
		y1 := y0 + cellH
		if col == cols-1 {
			x1 = desktopW
		}
		if row == rows-1 {
			y1 = desktopH
		}
		w.ChangeBounds(geom.NewRect(x0, y0, x1, y1))
		// fv-go's Window.OnResize only fires from mouse-drag resizes,
		// never from programmatic ChangeBounds — rerender here or every
		// pane's click hit-map (Pane.LastRect) keeps pre-tile geometry
		// and split positions stay GrowMode-stretched instead of
		// ratio-derived.
		if state := m.windows[w.Self()]; state != nil {
			m.rerender(state)
		}
	}
}

// cascade resizes every window to 75% of the desktop and offsets them
// diagonally by 2 cells per step. Mirrors Turbo Vision's Cascade.
func (m *Mux) cascade() {
	ws := m.fvmuxWindows()
	if len(ws) == 0 {
		return
	}
	cols, rows := m.App.Desktop.Size.X, m.App.Desktop.Size.Y
	winW, winH := cols*3/4, rows*3/4
	if winW < 30 {
		winW = 30
	}
	if winH < 8 {
		winH = 8
	}
	for i, w := range ws {
		off := i * 2
		x := safeOffset(off, cols-winW)
		y := safeOffset(off, rows-winH)
		w.ChangeBounds(geom.NewRect(x, y, x+winW, y+winH))
		// Same as tileGrid: programmatic ChangeBounds never fires
		// OnResize, so resync the hit-map + split ratios ourselves.
		if state := m.windows[w.Self()]; state != nil {
			m.rerender(state)
		}
	}
	m.refreshStatusBar()
}

// cascadeNoResize keeps each window's current size, just shifts origins
// diagonally so they aren't perfectly stacked.
func (m *Mux) cascadeNoResize() {
	ws := m.fvmuxWindows()
	if len(ws) == 0 {
		return
	}
	desktopW, desktopH := m.App.Desktop.Size.X, m.App.Desktop.Size.Y
	for i, w := range ws {
		off := i * 2
		x := safeOffset(off, desktopW-w.Size.X)
		y := safeOffset(off, desktopH-w.Size.Y)
		w.ChangeBounds(geom.NewRect(x, y, x+w.Size.X, y+w.Size.Y))
	}
	m.refreshStatusBar()
}

func safeOffset(off, max int) int {
	if max <= 0 {
		return 0
	}
	x := off % max
	if x < 0 {
		x = 0
	}
	return x
}
