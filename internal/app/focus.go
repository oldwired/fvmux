package app

import (
	"github.com/oldwired/fvmux/internal/copymode"
	"github.com/oldwired/fvmux/internal/layout"
)

// setPaneFocus is the single state transition for pane focus. It rejects
// stale/foreign nodes so command targets can never drift outside ws.Root.
//
// A zoomed window keeps zoom mode active while focus moves: the newly focused
// pane becomes the rendered pane and rerender replaces the visible body. This
// keeps the logical command target, fv-go focus path, and visible pane in
// lock-step.
func (m *Mux) setPaneFocus(ws *windowState, next *layout.PaneNode) bool {
	if ws == nil || ws.Root == nil || next == nil || !next.IsLeaf() || next.Pane == nil {
		return false
	}
	// FindByID also verifies the PaneID is live, while the identity check
	// rejects an old leaf retained across a layout rebuild.
	if ws.Root.FindByID(next.Pane.ID) != next {
		return false
	}

	ws.Focus = next
	ws.ShellTitle = next.Pane.ShellTitle
	zoomMoved := ws.Zoomed != nil && *ws.Zoomed != next.Pane.ID
	if ws.Zoomed != nil {
		id := next.Pane.ID
		ws.Zoomed = &id
	}
	if zoomMoved {
		m.rerender(ws)
	} else if ws.Frame != nil {
		focusTerminalPath(ws.Frame, next.Pane.Term)
	}
	m.refreshWindowTitle(ws)
	m.refreshStatusBar()
	return true
}

// focusedPane returns the command target only when it is a live leaf and,
// while zoomed, is also the pane the user can see. Inconsistent state is an
// invariant violation: commands fail closed instead of silently retargeting.
func focusedPane(ws *windowState) *layout.PaneNode {
	if ws == nil || ws.Root == nil || ws.Focus == nil || ws.Focus.Pane == nil ||
		ws.Root.FindByID(ws.Focus.Pane.ID) != ws.Focus {
		return nil
	}
	if ws.Zoomed != nil && *ws.Zoomed != ws.Focus.Pane.ID {
		return nil
	}
	return ws.Focus
}

func (m *Mux) pasteClipboard() {
	paste := copymode.Paste
	if m.pasteTo != nil {
		paste = m.pasteTo
	}
	_ = paste(m.FocusedTerminal())
}
