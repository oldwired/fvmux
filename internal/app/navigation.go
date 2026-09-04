package app

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"

	"github.com/oldwired/fvmux/internal/ui"
)

// focusWindowByNumber jumps to the window with the given Number badge.
// No-op if none matches.
func (m *Mux) focusWindowByNumber(n int) {
	for _, key := range m.windowOrder {
		ws := m.windows[key]
		if ws != nil && ws.Number == n {
			m.focusWindowView(key)
			return
		}
	}
}

// lastWindow flips between the current window and the previous one
// (set by focusWindowView). No-op when only one window has ever been
// focused.
func (m *Mux) lastWindow() {
	if m.lastFocused == nil {
		return
	}
	if _, ok := m.windows[m.lastFocused]; !ok {
		m.lastFocused = nil
		return
	}
	m.focusWindowView(m.lastFocused)
}

// focusNextPaneInTree moves focus to the next leaf in the current
// window's tree (in-order traversal). direction = +1 forward,
// -1 backward. Wraps around.
func (m *Mux) focusNextPaneInTree(direction int) {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil {
		return
	}
	leaves := ws.Root.CollectLeaves()
	if len(leaves) < 2 {
		return
	}
	idx := -1
	for i, l := range leaves {
		if l == ws.Focus {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	next := leaves[(idx+direction+len(leaves))%len(leaves)]
	m.setPaneFocus(ws, next)
}

// showWindowList opens a fuzzy-search picker over every window in
// creation order. Driven by both Ctrl-G w (Window List) and Ctrl-G f
// (Find Window) — the rendered rows mix number + title so either
// natural query works.
func (m *Mux) showWindowList() {
	if len(m.windowOrder) == 0 {
		return
	}
	items := make([]string, 0, len(m.windowOrder))
	keys := make([]views.View, 0, len(m.windowOrder))
	for _, key := range m.windowOrder {
		ws := m.windows[key]
		if ws == nil {
			continue
		}
		items = append(items, fmt.Sprintf("%d: %s", ws.Number, ws.displayTitle()))
		keys = append(keys, key)
	}
	desk := m.App.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 60, 14, 4)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	idx := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), items).Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(keys) {
		return
	}
	m.focusWindowView(keys[idx])
}
