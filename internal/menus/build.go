// Package menus builds fvmux's eight-menu top bar from the commands
// registry, keeping the visible menus synchronised with whatever
// commands and chords are currently registered.
//
// Unlike the early flat-by-category approach, this version hand-crafts
// each menu so we can introduce dividers, submenus, and a deliberate
// ordering (e.g., Quit is always last in File). Every entry is still
// sourced from the registry by ID — chord rebinds and label changes
// flow through automatically.
package menus

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	fvmenus "github.com/oldwired/fv-go/pkg/fv/menus"

	"github.com/oldwired/fvmux/internal/commands"
)

// Build assembles the full menu bar with no dynamic extras. Tests use
// it; the runtime calls BuildWithExtras so themes/profiles/sessions
// submenus pick up live state.
func Build(bounds geom.Rect, reg *commands.Registry) *fvmenus.MenuBar {
	return BuildWithExtras(bounds, reg, Extras{})
}

// BuildWithExtras assembles the full menu bar with dynamic submenus
// (currently active transfers) sourced from extras. Each menu is constructed by hand so
// layout (order, separators, submenus) is explicit and inspectable.
func BuildWithExtras(bounds geom.Rect, reg *commands.Registry, extras Extras) *fvmenus.MenuBar {
	bar := fvmenus.NewMenu(
		&fvmenus.Item{Name: "~F~ile", Sub: fileMenu(reg, extras)},
		&fvmenus.Item{Name: "~E~dit", Sub: editMenu(reg)},
		&fvmenus.Item{Name: "~V~iew", Sub: viewMenu(reg, extras)},
		&fvmenus.Item{Name: "~P~ane", Sub: paneMenu(reg)},
		&fvmenus.Item{Name: "~W~indow", Sub: windowMenu(reg)},
		&fvmenus.Item{Name: "~C~onnections", Sub: connectionsMenu(reg)},
		&fvmenus.Item{Name: "~T~ransfer", Sub: transferMenu(reg, extras)},
		&fvmenus.Item{Name: "~H~elp", Sub: helpMenu(reg)},
	)
	return fvmenus.NewMenuBar(bounds, bar)
}

func fileMenu(reg *commands.Registry, extras Extras) *fvmenus.Menu {
	items := []*fvmenus.Item{
		item(reg, commands.CmdNewWindow),
		item(reg, commands.CmdNewWindowFromProfile),
	}
	items = append(items,
		item(reg, commands.CmdRun),
		sep(),
		item(reg, commands.CmdNewSession),
		item(reg, commands.CmdOpenSession),
		item(reg, commands.CmdSaveSession),
		item(reg, commands.CmdSaveSessionAs),
		item(reg, commands.CmdRenameSession),
		item(reg, commands.CmdDeleteSession),
		sep(),
		item(reg, commands.CmdOpenConfig),
		item(reg, commands.CmdOpenProfiles),
		item(reg, commands.CmdOpenKeybindings),
		sep(),
		item(reg, commands.CmdDetach),
		sep(),
		item(reg, commands.CmdQuit),
	)
	return menu(items...)
}

func editMenu(reg *commands.Registry) *fvmenus.Menu {
	return menu(
		item(reg, commands.CmdEnterCopyMode),
		item(reg, commands.CmdPaste),
		item(reg, commands.CmdFindScrollback),
		sep(),
		item(reg, commands.CmdToggleSyncInput),
		sep(),
		sub("Send Si~g~nal",
			item(reg, commands.CmdSendSIGINT),
			item(reg, commands.CmdSendSIGQUIT),
			item(reg, commands.CmdSendEOF),
			item(reg, commands.CmdSendSIGTERM),
		),
	)
}

func viewMenu(reg *commands.Registry, extras Extras) *fvmenus.Menu {
	items := []*fvmenus.Item{
		item(reg, commands.CmdZoomPane),
		item(reg, commands.CmdCycleLayout),
		sub("Layout ~P~reset",
			item(reg, commands.CmdLayoutEvenH),
			item(reg, commands.CmdLayoutEvenV),
			item(reg, commands.CmdLayoutMainH),
			item(reg, commands.CmdLayoutMainV),
			item(reg, commands.CmdLayoutTiled),
		),
		sep(),
		item(reg, commands.CmdFlashNumbers),
		item(reg, commands.CmdRedraw),
		sep(),
		item(reg, commands.CmdToggleClock),
		item(reg, commands.CmdToggleStatusBar),
		item(reg, commands.CmdToggleMenuBar),
		sep(),
		item(reg, commands.CmdThemePicker),
		item(reg, commands.CmdEditThemes),
	}
	return menu(items...)
}

func paneMenu(reg *commands.Registry) *fvmenus.Menu {
	return menu(
		item(reg, commands.CmdSplitH),
		item(reg, commands.CmdSplitV),
		sep(),
		sub("~F~ocus",
			item(reg, commands.CmdFocusLeft),
			item(reg, commands.CmdFocusDown),
			item(reg, commands.CmdFocusUp),
			item(reg, commands.CmdFocusRight),
			sep(),
			item(reg, commands.CmdFocusNext),
			item(reg, commands.CmdFocusPrev),
		),
		sep(),
		item(reg, commands.CmdSwapNext),
		item(reg, commands.CmdSwapPrev),
		sep(),
		item(reg, commands.CmdBreakOut),
		item(reg, commands.CmdJoinFrom),
		sep(),
		item(reg, commands.CmdRenamePane),
		sep(),
		item(reg, commands.CmdEnterResize),
		sep(),
		item(reg, commands.CmdClosePane),
		item(reg, commands.CmdRespawnPane),
	)
}

func windowMenu(reg *commands.Registry) *fvmenus.Menu {
	return menu(
		item(reg, commands.CmdRenameWindow),
		sep(),
		item(reg, commands.CmdNextWindow),
		item(reg, commands.CmdPrevWindow),
		item(reg, commands.CmdLastWindow),
		sep(),
		item(reg, commands.CmdWindowList),
		item(reg, commands.CmdFindWindow),
		sub("Focus Window ~#~",
			item(reg, commands.CmdFocusWindow1),
			item(reg, commands.CmdFocusWindow2),
			item(reg, commands.CmdFocusWindow3),
			item(reg, commands.CmdFocusWindow4),
			item(reg, commands.CmdFocusWindow5),
			item(reg, commands.CmdFocusWindow6),
			item(reg, commands.CmdFocusWindow7),
			item(reg, commands.CmdFocusWindow8),
			item(reg, commands.CmdFocusWindow9),
		),
		sep(),
		sub("~A~rrange",
			item(reg, commands.CmdTile),
			item(reg, commands.CmdTileHorizontal),
			item(reg, commands.CmdTileVertical),
			sep(),
			item(reg, commands.CmdCascade),
			item(reg, commands.CmdCascadeNoResize),
		),
		sep(),
		item(reg, commands.CmdKillWindow),
	)
}

func connectionsMenu(reg *commands.Registry) *fvmenus.Menu {
	items := []*fvmenus.Item{
		item(reg, commands.CmdConnectHost),
		sep(),
		item(reg, commands.CmdActiveConnections),
	}
	items = append(items,
		sep(),
		item(reg, commands.CmdEditHosts),
		item(reg, commands.CmdReloadHosts),
	)
	return menu(items...)
}

func transferMenu(reg *commands.Registry, extras Extras) *fvmenus.Menu {
	items := []*fvmenus.Item{
		item(reg, commands.CmdSFTPBrowser),
		item(reg, commands.CmdSFTPHere),
		item(reg, commands.CmdSFTPNewHere),
		item(reg, commands.CmdToggleFilesFollow),
		sep(),
		item(reg, commands.CmdUploadFile),
		item(reg, commands.CmdDownloadFile),
		sep(),
		item(reg, commands.CmdActiveTransfers),
		item(reg, commands.CmdClearCompleted),
	}
	if len(extras.Transfers) > 0 {
		items = append(items, sep(),
			subItems("Active ~T~ransfers Submenu", itemsFromExtras(extras.Transfers)))
	}
	return menu(items...)
}

func helpMenu(reg *commands.Registry) *fvmenus.Menu {
	return menu(
		item(reg, commands.CmdCommandPalette),
		item(reg, commands.CmdCheatsheet),
		sep(),
		item(reg, commands.CmdLogViewer),
		item(reg, commands.CmdReloadConfig),
		sep(),
		item(reg, commands.CmdResetFirstRun),
		sep(),
		item(reg, commands.CmdAbout),
	)
}

// --- helpers ----------------------------------------------------------

// item returns the fv-menu Item for a registered command, or nil if
// the ID doesn't exist (so menu drops it silently).
func item(reg *commands.Registry, id uint16) *fvmenus.Item {
	c := reg.ByID(id)
	if c == nil {
		return nil
	}
	disabled := c.Enabled != nil && !c.Enabled(nil)
	return &fvmenus.Item{
		Name:     menuLabel(c),
		Command:  c.ID,
		Disabled: disabled,
		Shortcut: c.Chord,
	}
}

// RefreshAvailability re-evaluates registry predicates against an existing
// menu tree. Menu items store Disabled as a value, so the app calls this on
// state refreshes rather than rebuilding (and accidentally re-showing) a
// hidden menu bar. Returns true when any row changed.
func RefreshAvailability(bar *fvmenus.MenuBar, reg *commands.Registry) bool {
	if bar == nil || bar.Menu == nil || reg == nil {
		return false
	}
	changed := false
	var walk func(*fvmenus.Menu)
	walk = func(menu *fvmenus.Menu) {
		if menu == nil {
			return
		}
		for _, it := range menu.Items {
			if it == nil {
				continue
			}
			if it.Sub != nil {
				walk(it.Sub)
			}
			if it.Command == 0 {
				continue
			}
			c := reg.ByID(it.Command)
			if c == nil {
				continue // dynamic menu entries do not live in the registry.
			}
			disabled := c.Enabled != nil && !c.Enabled(nil)
			if it.Disabled != disabled {
				it.Disabled = disabled
				changed = true
			}
		}
	}
	walk(bar.Menu)
	return changed
}

func menuLabel(c *commands.Command) string {
	if c.MenuLabel != "" {
		return c.MenuLabel
	}
	return c.Name
}

// sep returns a horizontal-rule item.
func sep() *fvmenus.Item { return fvmenus.Separator() }

// sub builds a submenu item containing the given children (nil children
// are dropped — same forgiving rule as item()).
func sub(label string, items ...*fvmenus.Item) *fvmenus.Item {
	return &fvmenus.Item{Name: label, Sub: menu(items...)}
}

// subItems is sub() but takes its children as a slice — convenient for
// extras-provider results that come pre-built.
func subItems(label string, items []*fvmenus.Item) *fvmenus.Item {
	return &fvmenus.Item{Name: label, Sub: menu(items...)}
}

// menu builds a *Menu from items, dropping nils so callers can pass
// item(reg, X) freely without worrying about unregistered IDs.
func menu(items ...*fvmenus.Item) *fvmenus.Menu {
	out := make([]*fvmenus.Item, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, it)
	}
	return fvmenus.NewMenu(out...)
}
