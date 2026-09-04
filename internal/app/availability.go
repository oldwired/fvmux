package app

import (
	"time"

	fvmenus "github.com/oldwired/fv-go/pkg/fv/menus"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/menus"
)

// wireCommandAvailability installs the live predicates shared by prefix
// dispatch, the palette, the menu bar, and the pane context menu. The
// closures intentionally capture Mux rather than Ctx: non-dispatch surfaces
// build rows without an invocation context but still need the same answer.
func (m *Mux) wireCommandAvailability() {
	set := func(reason string, enabled func() bool, ids ...uint16) {
		for _, id := range ids {
			if c := m.Reg.ByID(id); c != nil {
				c.Enabled = func(*commands.Ctx) bool { return enabled() }
				c.DisabledReason = reason
			}
		}
	}

	hasPane := func() bool { return m.availabilityPane() != nil }
	hasLivePane := func() bool {
		leaf := m.availabilityPane()
		return leaf != nil && paneIsAlive(leaf.Pane)
	}
	hasDeadPane := func() bool {
		leaf := m.availabilityPane()
		return leaf != nil && leaf.Pane.Dead
	}
	hasMultiplePanes := func() bool {
		ws := m.currentWindow()
		return focusedPane(ws) != nil && len(ws.Root.CollectLeaves()) > 1
	}
	hasWindow := func() bool { return m.currentWindow() != nil }
	hasWindows := func() bool { return len(m.windowOrder) > 0 }
	hasMultipleWindows := func() bool { return len(m.windowOrder) > 1 }

	set("no focused pane", hasPane,
		commands.CmdSplitH, commands.CmdSplitV, commands.CmdClosePane,
		commands.CmdRenamePane)
	set("focused pane is not an SSH session", func() bool {
		leaf := m.availabilityPane()
		return leaf != nil && leaf.Pane != nil && leaf.Pane.SSHAlias != ""
	}, commands.CmdSFTPHere, commands.CmdSFTPNewHere)
	set("focused pane is not running", hasLivePane,
		commands.CmdLiteralPrefix, commands.CmdEnterCopyMode, commands.CmdPaste,
		commands.CmdFindScrollback, commands.CmdSendSIGINT, commands.CmdSendSIGQUIT,
		commands.CmdSendEOF, commands.CmdSendSIGTERM)
	set("focused pane is still running", hasDeadPane, commands.CmdRespawnPane)
	set("requires at least two panes", hasMultiplePanes,
		commands.CmdZoomPane, commands.CmdSwapNext, commands.CmdSwapPrev,
		commands.CmdBreakOut, commands.CmdFocusNext, commands.CmdFocusPrev,
		commands.CmdEnterResize, commands.CmdToggleSyncInput, commands.CmdCycleLayout,
		commands.CmdLayoutEvenH, commands.CmdLayoutEvenV, commands.CmdLayoutMainH,
		commands.CmdLayoutMainV, commands.CmdLayoutTiled)

	for id, dir := range map[uint16]layout.Direction{
		commands.CmdFocusLeft:  layout.Left,
		commands.CmdFocusDown:  layout.Down,
		commands.CmdFocusUp:    layout.Up,
		commands.CmdFocusRight: layout.Right,
	} {
		direction := dir
		set("no pane in that direction", func() bool {
			ws := m.currentWindow()
			leaf := focusedPane(ws)
			return leaf != nil && layout.FocusDir(ws.Root, leaf, direction) != nil
		}, id)
	}

	set("no focused window", hasWindow, commands.CmdRenameWindow)
	set("no focused window", func() bool {
		return m.currentWindow() != nil || m.currentFileWindow() != nil
	}, commands.CmdKillWindow)
	set("no windows", hasWindows, commands.CmdWindowList, commands.CmdFindWindow,
		commands.CmdFlashNumbers)
	set("requires at least two windows", hasMultipleWindows,
		commands.CmdNextWindow, commands.CmdPrevWindow)
	set("no previous window", func() bool {
		return m.lastFocused != nil && m.workspaceWindowExists(m.lastFocused)
	}, commands.CmdLastWindow)
	set("no eligible single-pane window", m.hasJoinableWindow, commands.CmdJoinFrom)

	for n, id := range []uint16{
		commands.CmdFocusWindow1, commands.CmdFocusWindow2, commands.CmdFocusWindow3,
		commands.CmdFocusWindow4, commands.CmdFocusWindow5, commands.CmdFocusWindow6,
		commands.CmdFocusWindow7, commands.CmdFocusWindow8, commands.CmdFocusWindow9,
	} {
		number := n + 1
		set("window is not open", func() bool { return m.windowNumberInUse(number) }, id)
	}
}

func (m *Mux) availabilityPane() *layout.PaneNode {
	if m == nil || m.App == nil || m.App.Program == nil {
		return nil
	}
	return focusedPane(m.currentWindow())
}

func (m *Mux) hasJoinableWindow() bool {
	dst := m.currentWindow()
	if dst == nil || focusedPane(dst) == nil {
		return false
	}
	for _, key := range m.windowOrder {
		ws := m.windows[key]
		if ws != nil && ws != dst && ws.Root != nil && len(ws.Root.CollectLeaves()) == 1 {
			return true
		}
	}
	return false
}

func (m *Mux) showUnavailableCommand(c *commands.Command) {
	if c == nil {
		return
	}
	reason := c.DisabledReason
	if reason == "" {
		reason = "not available here"
	}
	m.setFlash(c.Name+": "+reason, 1800*time.Millisecond, flashPrioCommand)
	m.refreshStatusBar()
}

// refreshMenuAvailability updates a visible menu in place. This preserves a
// deliberately hidden menu bar; rebuilding through Options.RefreshUI would
// otherwise make it reappear as a side effect of an ordinary focus change.
func (m *Mux) refreshMenuAvailability() {
	if m == nil || m.App == nil || m.App.MenuBar == nil {
		return
	}
	bar, ok := m.App.MenuBar.(*fvmenus.MenuBar)
	if !ok {
		return
	}
	if menus.RefreshAvailability(bar, m.Reg) {
		views.MarkDirty()
	}
}
