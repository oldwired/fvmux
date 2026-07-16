package app

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/ui"
)

// joinFrom is the inverse of doBreakOut: take a single-leaf window's
// only pane and merge it into the focused window's tree at the focused
// pane's slot. The picker only lists candidate single-leaf windows
// (layout.JoinFrom errors on multi-leaf sources).
func (m *Mux) joinFrom() {
	dst := m.currentWindow()
	if dst == nil || dst.Focus == nil || !dst.Focus.IsLeaf() {
		return
	}

	type cand struct {
		key views.View
		ws  *windowState
	}
	var cands []cand
	for _, key := range m.windowOrder {
		ws := m.windows[key]
		if ws == nil || ws == dst || ws.Root == nil {
			continue
		}
		if len(ws.Root.CollectLeaves()) != 1 {
			continue
		}
		cands = append(cands, cand{key, ws})
	}
	if len(cands) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No single-pane source windows to join.",
			msgbox.OKOnly)
		return
	}

	items := make([]string, len(cands))
	for i, c := range cands {
		items[i] = fmt.Sprintf("%d: %s", c.ws.Number, c.ws.displayTitle())
	}
	desk := m.App.Desktop.BaseView()
	// Previously this picker clamped nothing and could overflow a tiny
	// terminal; give it the standard centre-and-clamp.
	r := ui.CenterRect(desk.Size, 50, 12, 2)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	idx := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), items).Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(cands) {
		return
	}
	src := cands[idx]

	// Pick orientation via popupmenu.
	orient := popupmenu.New(geom.Point{X: x, Y: y},
		[]string{"Split horizontal (side-by-side)", "Split vertical (stacked)"}, 36).
		Run(&m.App.Desktop.Group)
	if orient < 0 {
		return
	}
	splitOrient := views.SplitVertical // SplitH naming = vertical splitter
	if orient == 1 {
		splitOrient = views.SplitHorizontal
	}

	newRoot, err := layout.JoinFrom(dst.Root, dst.Focus, src.ws.Root, splitOrient)
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"JoinFrom failed:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	dst.Root = newRoot
	// Re-wire terminal callbacks for every pane now living under the
	// destination window: the moved panes' OnTitle/OnExit closures
	// captured the SOURCE window, which is about to be deleted — their
	// title updates would resolve a nil windowState forever after.
	// (Re-wiring the destination's own panes is an idempotent refresh.)
	dst.Root.Leaves(func(l *layout.PaneNode) {
		if l.Pane != nil {
			m.wireTerminalCallbacks(l.Pane, dst.Frame)
		}
	})
	// Source window is now empty; close it (cleanup stops the moved
	// pane's terminal? — NO. The pane moved by JoinFrom is the same
	// *session.Pane reference; we must NOT call Stop on it. Drop the
	// source from our maps without cleanupWindow's PTY walk.
	src.ws.Root = nil // prevent cleanupWindow from stopping the moved PTY
	m.removeWindow(src.ws)
	m.rerender(dst)
}
