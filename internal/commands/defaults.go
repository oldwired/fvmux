package commands

// Defaults returns a fresh Registry pre-populated with every built-in
// fvmux command. Subsequent sub-steps add entries here as features land;
// the registry is the single source of truth for menus, palette, and
// cheatsheet.
//
// Actions are set by callers (the cmd/fvmux wiring layer) after the
// registry is built — keeping Defaults free of cross-package imports.
func Defaults() *Registry {
	r := New()

	r.Register(&Command{
		ID:        CmdNewWindow,
		Category:  "File",
		Name:      "New Window",
		MenuLabel: "~N~ew Window",
		Chord:     "C-g c",
	})
	r.Register(&Command{
		ID:        CmdCheatsheet,
		Category:  "Help",
		Name:      "Cheatsheet",
		MenuLabel: "~C~heatsheet",
		Chord:     "C-g ?",
	})
	r.Register(&Command{
		ID:        CmdQuit,
		Category:  "File",
		Name:      "Quit fvmux",
		MenuLabel: "~Q~uit fvmux",
	})
	r.Register(&Command{
		ID:       CmdLiteralPrefix,
		Category: "Edit",
		Name:     "Send Literal Prefix",
		Chord:    "C-g C-g",
		Hidden:   true,
	})

	r.Register(&Command{
		ID:        CmdSplitH,
		Category:  "Pane",
		Name:      "Split Left/Right",
		Aliases:   []string{"Split Horizontal"},
		MenuLabel: "Split Left/Ri~g~ht",
		Chord:     "C-g %",
	})
	r.Register(&Command{
		ID:        CmdSplitV,
		Category:  "Pane",
		Name:      "Split Top/Bottom",
		Aliases:   []string{"Split Vertical"},
		MenuLabel: "Split ~T~op/Bottom",
		Chord:     "C-g \"",
	})
	r.Register(&Command{
		ID:        CmdClosePane,
		Category:  "Pane",
		Name:      "Kill Pane",
		MenuLabel: "~K~ill Pane",
		Chord:     "C-g x",
	})
	r.Register(&Command{
		ID:        CmdZoomPane,
		Category:  "View",
		Name:      "Zoom Focused Pane",
		MenuLabel: "~Z~oom Focused Pane",
		Chord:     "C-g z",
	})

	r.Register(&Command{ID: CmdFocusLeft, Category: "Pane", Name: "Focus Left", MenuLabel: "Focus ~L~eft", Chord: "C-g h"})
	r.Register(&Command{ID: CmdFocusDown, Category: "Pane", Name: "Focus Down", MenuLabel: "Focus ~D~own", Chord: "C-g j"})
	r.Register(&Command{ID: CmdFocusUp, Category: "Pane", Name: "Focus Up", MenuLabel: "Focus ~U~p", Chord: "C-g k"})
	r.Register(&Command{ID: CmdFocusRight, Category: "Pane", Name: "Focus Right", MenuLabel: "Focus ~R~ight", Chord: "C-g l"})

	r.Register(&Command{ID: CmdSwapNext, Category: "Pane", Name: "Swap with Next", MenuLabel: "Swap with ~N~ext", Chord: "C-g }"})
	r.Register(&Command{ID: CmdSwapPrev, Category: "Pane", Name: "Swap with Prev", MenuLabel: "S~w~ap with Prev", Chord: "C-g {"})

	r.Register(&Command{ID: CmdBreakOut, Category: "Pane", Name: "Break Out to Window", MenuLabel: "~B~reak Out to Window", Chord: "C-g !"})

	r.Register(&Command{ID: CmdNewWindowFromProfile, Category: "File", Name: "New Window from Profile", MenuLabel: "New Window from ~P~rofile…", Chord: "C-g C"})
	r.Register(&Command{ID: CmdNextWindow, Category: "Window", Name: "Next Window", MenuLabel: "N~e~xt Window", Chord: "C-g n"})
	r.Register(&Command{ID: CmdPrevWindow, Category: "Window", Name: "Previous Window", MenuLabel: "~P~revious Window", Chord: "C-g p"})
	r.Register(&Command{ID: CmdKillWindow, Category: "Window", Name: "Kill Window", MenuLabel: "~K~ill Window", Chord: "C-g &"})
	r.Register(&Command{ID: CmdSaveSession, Category: "File", Name: "Save Session", MenuLabel: "~S~ave Session", Chord: "C-g S"})
	r.Register(&Command{ID: CmdRenameWindow, Category: "Window", Name: "Rename Window…", MenuLabel: "~R~ename Window…", Chord: "C-g ,"})

	r.Register(&Command{
		ID:        CmdCommandPalette,
		Category:  "Help",
		Name:      "Command Palette",
		MenuLabel: "Command ~P~alette…",
		Chord:     "C-g P",
	})
	r.Register(&Command{
		ID:        CmdResetFirstRun,
		Category:  "Help",
		Name:      "Reset First-Run Wizard…",
		MenuLabel: "~R~eset First-Run Wizard…",
		Chord:     "C-g W",
	})
	r.Register(&Command{
		ID:        CmdReloadConfig,
		Category:  "Help",
		Name:      "Reload Config",
		MenuLabel: "Re~l~oad Config",
	})
	r.Register(&Command{
		ID:        CmdLogViewer,
		Category:  "Help",
		Name:      "Log Viewer",
		MenuLabel: "Lo~g~ Viewer",
		Chord:     "C-g L",
	})
	r.Register(&Command{
		ID:        CmdAbout,
		Category:  "Help",
		Name:      "About fvmux…",
		MenuLabel: "~A~bout fvmux…",
	})

	// Session lifecycle extras.
	r.Register(&Command{ID: CmdNewSession, Category: "File", Name: "New Session", MenuLabel: "Ne~w~ Session"})
	r.Register(&Command{ID: CmdOpenSession, Category: "File", Name: "Open Session…", MenuLabel: "~O~pen Session…", Chord: "C-g s"})
	r.Register(&Command{ID: CmdSaveSessionAs, Category: "File", Name: "Save Session As…", MenuLabel: "Save Session ~A~s…"})
	r.Register(&Command{ID: CmdRenameSession, Category: "File", Name: "Rename Session…", MenuLabel: "Rena~m~e Session…", Chord: "C-g $"})
	r.Register(&Command{ID: CmdDeleteSession, Category: "File", Name: "Delete Session…", MenuLabel: "De~l~ete Session…"})

	// Pane rename.
	r.Register(&Command{ID: CmdRenamePane, Category: "Pane", Name: "Rename Pane…", MenuLabel: "Rename ~P~ane…"})

	// View toggles + redraw + flash numbers.
	r.Register(&Command{ID: CmdToggleClock, Category: "View", Name: "Toggle Clock", MenuLabel: "Toggle ~C~lock", Chord: "C-g t"})
	r.Register(&Command{ID: CmdToggleStatusBar, Category: "View", Name: "Toggle Status Bar", MenuLabel: "Toggle ~S~tatus Bar"})
	r.Register(&Command{ID: CmdToggleMenuBar, Category: "View", Name: "Toggle Menu Bar", MenuLabel: "Toggle ~M~enu Bar"})
	r.Register(&Command{ID: CmdOpenMenu, Category: "View", Name: "Activate Menu Bar", Chord: "C-g m"})
	r.Register(&Command{ID: CmdRedraw, Category: "View", Name: "Redraw", MenuLabel: "~R~edraw", Chord: "C-g r"})
	r.Register(&Command{ID: CmdFlashNumbers, Category: "View", Name: "Flash Window Numbers", MenuLabel: "Flash Window ~N~umbers", Chord: "C-g q"})

	// JoinFrom — opposite of BreakOut.
	r.Register(&Command{ID: CmdJoinFrom, Category: "Pane", Name: "Join from Window…", MenuLabel: "~J~oin from Window…", Chord: "C-g @"})

	// Layout-preset direct picks.
	r.Register(&Command{ID: CmdLayoutEvenH, Category: "View", Name: "Layout: Even Horizontal", MenuLabel: "Even ~H~orizontal"})
	r.Register(&Command{ID: CmdLayoutEvenV, Category: "View", Name: "Layout: Even Vertical", MenuLabel: "Even ~V~ertical"})
	r.Register(&Command{ID: CmdLayoutMainH, Category: "View", Name: "Layout: Main Horizontal", MenuLabel: "Main H~o~rizontal"})
	r.Register(&Command{ID: CmdLayoutMainV, Category: "View", Name: "Layout: Main Vertical", MenuLabel: "Main Ve~r~tical"})
	r.Register(&Command{ID: CmdLayoutTiled, Category: "View", Name: "Layout: Tiled", MenuLabel: "~T~iled"})
	r.Register(&Command{
		ID:        CmdThemePicker,
		Category:  "View",
		Name:      "Theme…",
		MenuLabel: "~T~heme…",
		Chord:     "C-g T",
	})
	r.Register(&Command{
		ID:        CmdEditThemes,
		Category:  "View",
		Name:      "Edit Themes…",
		MenuLabel: "~E~dit Themes…",
	})

	r.Register(&Command{ID: CmdEnterCopyMode, Category: "Edit", Name: "Enter Copy Mode", MenuLabel: "Enter ~C~opy Mode", Chord: "C-g ["})
	r.Register(&Command{ID: CmdPaste, Category: "Edit", Name: "Paste", MenuLabel: "~P~aste", Chord: "C-g ]"})
	r.Register(&Command{ID: CmdFindScrollback, Category: "Edit", Name: "Find in Scrollback", MenuLabel: "~F~ind in Scrollback", Chord: "C-g /"})

	r.Register(&Command{ID: CmdConnectHost, Category: "Connections", Name: "Connect to Host…", MenuLabel: "~C~onnect to Host…", Chord: "C-g H"})
	r.Register(&Command{ID: CmdEditHosts, Category: "Connections", Name: "Edit hosts.toml", MenuLabel: "~E~dit hosts.toml", Chord: "C-g B"})
	r.Register(&Command{ID: CmdActiveConnections, Category: "Connections", Name: "SSH Connection Diagnostics…", MenuLabel: "SSH Connection ~D~iagnostics…"})
	// "Validate", not "Reload": nothing caches host data (every picker
	// re-reads from disk), so the command's real value is a parse check
	// with a visible result — naming it "Reload" implied staleness that
	// doesn't exist.
	r.Register(&Command{ID: CmdReloadHosts, Category: "Connections", Name: "Validate hosts.toml", MenuLabel: "~V~alidate hosts.toml"})

	r.Register(&Command{ID: CmdSFTPBrowser, Category: "Transfer", Name: "Files Window…", MenuLabel: "~F~iles Window…", Chord: "C-g F"})
	r.Register(&Command{ID: CmdSFTPHere, Category: "Transfer", Name: "Open Files Here", MenuLabel: "Open Files ~H~ere"})
	r.Register(&Command{ID: CmdSFTPNewHere, Category: "Transfer", Name: "Open Another Files Window Here", MenuLabel: "Open Another Files ~W~indow Here"})
	r.Register(&Command{ID: CmdToggleFilesFollow, Category: "Transfer", Name: "Follow Terminal Directory", MenuLabel: "Follow Terminal ~D~irectory"})
	// F5/F6 are bidirectional inside the browser: F5 copies the highlighted
	// entry to the other side, F6 moves/renames it. The names describe the
	// keys, not an up/down direction. Hotkey markers stay unique within the
	// Transfer category (F, o, M, A, C).
	r.Register(&Command{ID: CmdDownloadFile, Category: "Transfer", Name: "Copy to Other Side (F5)", MenuLabel: "C~o~py to Other Side (F5)"})
	r.Register(&Command{ID: CmdUploadFile, Category: "Transfer", Name: "Move / Rename (F6)", MenuLabel: "~M~ove / Rename (F6)"})
	r.Register(&Command{ID: CmdActiveTransfers, Category: "Transfer", Name: "Active Transfers…", MenuLabel: "~A~ctive Transfers…"})
	r.Register(&Command{ID: CmdClearCompleted, Category: "Transfer", Name: "Clear Completed Transfers", MenuLabel: "~C~lear Completed Transfers"})

	r.Register(&Command{
		ID:        CmdDetach,
		Category:  "File",
		Name:      "Detach from tmux",
		MenuLabel: "~D~etach from tmux",
		Chord:     "C-g D",
	})
	r.Register(&Command{
		ID:        CmdRun,
		Category:  "File",
		Name:      "Run Command…",
		MenuLabel: "~R~un Command…",
		Chord:     "C-g :",
	})

	r.Register(&Command{ID: CmdOpenConfig, Category: "File", Name: "Edit config.toml", MenuLabel: "Edit ~c~onfig.toml"})
	r.Register(&Command{ID: CmdOpenProfiles, Category: "File", Name: "Edit profiles.toml", MenuLabel: "Edit pro~f~iles.toml"})
	r.Register(&Command{ID: CmdOpenKeybindings, Category: "File", Name: "Edit keybindings.toml", MenuLabel: "Edit ~k~eybindings.toml"})

	// Window navigation extras.
	r.Register(&Command{ID: CmdLastWindow, Category: "Window", Name: "Last Window (MRU)", MenuLabel: "~L~ast Window", Chord: "C-g Tab"})
	r.Register(&Command{ID: CmdWindowList, Category: "Window", Name: "Window List…", MenuLabel: "Window L~i~st…", Chord: "C-g w"})
	r.Register(&Command{ID: CmdFindWindow, Category: "Window", Name: "Find Window…", MenuLabel: "~F~ind Window…", Chord: "C-g f"})
	r.Register(&Command{ID: CmdFocusWindow1, Category: "Window", Name: "Focus Window 1", Chord: "C-g 1"})
	r.Register(&Command{ID: CmdFocusWindow2, Category: "Window", Name: "Focus Window 2", Chord: "C-g 2"})
	r.Register(&Command{ID: CmdFocusWindow3, Category: "Window", Name: "Focus Window 3", Chord: "C-g 3"})
	r.Register(&Command{ID: CmdFocusWindow4, Category: "Window", Name: "Focus Window 4", Chord: "C-g 4"})
	r.Register(&Command{ID: CmdFocusWindow5, Category: "Window", Name: "Focus Window 5", Chord: "C-g 5"})
	r.Register(&Command{ID: CmdFocusWindow6, Category: "Window", Name: "Focus Window 6", Chord: "C-g 6"})
	r.Register(&Command{ID: CmdFocusWindow7, Category: "Window", Name: "Focus Window 7", Chord: "C-g 7"})
	r.Register(&Command{ID: CmdFocusWindow8, Category: "Window", Name: "Focus Window 8", Chord: "C-g 8"})
	r.Register(&Command{ID: CmdFocusWindow9, Category: "Window", Name: "Focus Window 9", Chord: "C-g 9"})

	r.Register(&Command{ID: CmdTile, Category: "Window", Name: "Tile (grid)", MenuLabel: "~T~ile (grid)", Chord: "C-g g"})
	r.Register(&Command{ID: CmdTileHorizontal, Category: "Window", Name: "Tile Horizontal", MenuLabel: "Tile ~H~orizontal", Chord: "C-g G"})
	r.Register(&Command{ID: CmdTileVertical, Category: "Window", Name: "Tile Vertical", MenuLabel: "Tile ~V~ertical", Chord: "C-g v"})
	r.Register(&Command{ID: CmdCascade, Category: "Window", Name: "Cascade", MenuLabel: "~C~ascade", Chord: "C-g K"})
	r.Register(&Command{ID: CmdCascadeNoResize, Category: "Window", Name: "Cascade (keep sizes)", MenuLabel: "Cascade (keep si~z~es)"})

	// Pane navigation / lifecycle extras.
	r.Register(&Command{ID: CmdFocusNext, Category: "Pane", Name: "Focus Next Pane", MenuLabel: "Focus N~e~xt Pane", Chord: "C-g o"})
	r.Register(&Command{ID: CmdFocusPrev, Category: "Pane", Name: "Focus Previous Pane", MenuLabel: "Focus Previous Pane", Chord: "C-g ;"})
	r.Register(&Command{ID: CmdRespawnPane, Category: "Pane", Name: "Respawn Dead Pane", MenuLabel: "Re~s~pawn Dead Pane"})
	r.Register(&Command{ID: CmdEnterResize, Category: "Pane", Name: "Enter Resize Mode", MenuLabel: "Enter ~R~esize Mode", Chord: "C-g R"})
	r.Register(&Command{ID: CmdCycleLayout, Category: "View", Name: "Cycle Layout", MenuLabel: "Cycle ~L~ayout", Chord: "C-g Space"})

	// Edit / signals.
	r.Register(&Command{ID: CmdToggleSyncInput, Category: "Edit", Name: "Toggle Sync-Input (broadcast)", MenuLabel: "Toggle ~S~ync-Input", Chord: "C-g ~"})
	r.Register(&Command{ID: CmdSendSIGINT, Category: "Edit", Name: "Send Interrupt (Ctrl-C)", MenuLabel: "Send ~I~nterrupt"})
	r.Register(&Command{ID: CmdSendSIGQUIT, Category: "Edit", Name: "Send Quit (Ctrl-\\)", MenuLabel: "Send ~Q~uit"})
	r.Register(&Command{ID: CmdSendEOF, Category: "Edit", Name: "Send EOF (Ctrl-D)", MenuLabel: "Send ~E~OF"})
	r.Register(&Command{ID: CmdSendSIGTERM, Category: "Edit", Name: "Send SIGTERM", MenuLabel: "Send SIG~T~ERM"})

	// Hidden — internal triggers.
	r.Register(&Command{ID: CmdTickerRedraw, Category: "Help", Name: "Ticker Redraw", Hidden: true})
	r.Register(&Command{ID: CmdPaneContextMenu, Category: "Pane", Name: "Pane Context Menu", Hidden: true})

	return r
}
