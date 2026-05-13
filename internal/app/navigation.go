package app

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"
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
	ws.Focus = next
	if next.Pane != nil {
		focusTerminalPath(ws.Frame, next.Pane.Term)
	}
	m.refreshStatusBar()
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
		title := ws.UserTitle
		if title == "" {
			title = ws.ShellTitle
		}
		if title == "" {
			title = ws.Title
		}
		items = append(items, fmt.Sprintf("%d: %s", ws.Number, title))
		keys = append(keys, key)
	}
	desk := m.App.Desktop.BaseView()
	w, h := 60, 14
	if w > desk.Size.X-4 {
		w = desk.Size.X - 4
	}
	if h > desk.Size.Y-4 {
		h = desk.Size.Y - 4
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	idx := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), items).Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(keys) {
		return
	}
	m.focusWindowView(keys[idx])
}
