package app

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/menus"
	"github.com/oldwired/fvmux/internal/statusbar"
)

// renamePane prompts for a new title and pins it to the focused
// pane's Pane.Title. Until the shell next emits an OSC title, this
// shows in the status-bar window list as a per-pane label.
func (m *Mux) renamePane() {
	ws := m.currentWindow()
	if ws == nil || ws.Focus == nil || ws.Focus.Pane == nil {
		return
	}
	text, ok := promptString(m.App, "Rename Pane",
		"Pane title (empty to clear):", ws.Focus.Pane.Title)
	if !ok {
		return
	}
	ws.Focus.Pane.Title = text
	m.refreshWindowTitle(ws)
	m.refreshStatusBar()
}

// toggleClock flips m.hideClock; the status-bar's right slot suppresses
// the clock when true.
func (m *Mux) toggleClock() {
	m.hideClock = !m.hideClock
	m.refreshStatusBar()
}

// toggleStatusBar hides / re-shows the bottom status bar.
func (m *Mux) toggleStatusBar() {
	if m.Opts.StatusBar != nil && m.Opts.StatusBar.Line != nil &&
		m.Opts.StatusBar.Line.BaseView().Owner != nil {
		m.App.SetStatusLine(nil)
		return
	}
	desk := m.App.BaseView().Size
	bar := statusbar.Build(
		geom.NewRect(0, desk.Y-1, desk.X, desk.Y),
		m.Opts.SessionName,
		m.Opts.Config.Appearance.StatusClock,
	)
	m.Opts.StatusBar = bar
	m.App.SetStatusLine(bar.Line)
	m.refreshStatusBar()
}

// toggleMenuBar hides / re-shows the top menu bar.
func (m *Mux) toggleMenuBar() {
	if m.App.MenuBar != nil && m.App.MenuBar.BaseView().Owner != nil {
		m.App.SetMenuBar(nil)
		return
	}
	// Re-show through the canonical RefreshUI rebuild — a direct
	// menus.Build here would drop the dynamic Extras submenus (Active
	// Masters, Active Transfers) and leave the Mux dispatch table
	// stale, silently diverging from every other menu refresh.
	if m.Opts.RefreshUI != nil {
		m.Opts.RefreshUI()
		return
	}
	desk := m.App.BaseView().Size
	m.App.SetMenuBar(menus.Build(geom.NewRect(0, 0, desk.X, 1), m.Reg))
}

// redraw forces a full repaint via fv-go's dirty-mark machinery.
// Cheap; useful when something underneath fvmux (an outer-tmux resize,
// a SIGWINCH) leaves the screen confused.
func (m *Mux) redraw() {
	views.MarkDirty()
}
