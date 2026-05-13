// Package commands is the single source of truth for every fvmux action.
// Menus, the Ctrl-G P palette, and the auto-generated cheatsheet all draw
// from one Registry — they cannot drift apart by construction.
//
// Command IDs are uint16 values (≥ 1000 to stay clear of fv-go's built-in
// command range) organised by category:
//
//	1000s — window / session
//	1010s — pane / layout
//	1020s — view
//	1030s — edit / clipboard
//	1040s — connections
//	1050s — transfer
//	1060s — help / meta
//	1090s — window list / find
//	1100s — window 1..9 focus
package commands

const (
	// 1000s — window / session
	CmdNewWindow            uint16 = 1001
	CmdNewWindowFromProfile uint16 = 1002 // Ctrl-G C
	CmdQuit                 uint16 = 1003
	CmdNextWindow           uint16 = 1004 // Ctrl-G n
	CmdPrevWindow           uint16 = 1005 // Ctrl-G p
	CmdKillWindow           uint16 = 1006 // Ctrl-G &
	CmdSaveSession          uint16 = 1007 // Ctrl-G S
	CmdRenameWindow         uint16 = 1008 // Ctrl-G ,
	CmdLastWindow           uint16 = 1009 // Ctrl-G Tab
	CmdDetach               uint16 = 1092 // Ctrl-G D — detach from outer tmux (no-op outside).
	CmdRun                  uint16 = 1093 // Ctrl-G : — ad-hoc command into a new window.

	// Embedded config editor entries — File menu.
	CmdOpenConfig      uint16 = 1080
	CmdOpenProfiles    uint16 = 1081
	CmdOpenKeybindings uint16 = 1082

	CmdTile            uint16 = 1094 // Tile windows in a near-square grid.
	CmdTileHorizontal  uint16 = 1095 // Tile as N horizontal strips.
	CmdTileVertical    uint16 = 1096 // Tile as N vertical strips.
	CmdCascade         uint16 = 1097 // Cascade — uniform 75% size, diagonal offsets.
	CmdCascadeNoResize uint16 = 1098 // Cascade without resizing.

	// 1010s — pane / layout
	CmdSplitH    uint16 = 1010 // Ctrl-G %, vertical splitter, panes side-by-side.
	CmdSplitV    uint16 = 1011 // Ctrl-G ", horizontal splitter, panes stacked.
	CmdClosePane uint16 = 1012 // Ctrl-G x.
	CmdZoomPane  uint16 = 1013 // Ctrl-G z.

	CmdFocusLeft  uint16 = 1014 // Ctrl-G h
	CmdFocusDown  uint16 = 1015 // Ctrl-G j
	CmdFocusUp    uint16 = 1016 // Ctrl-G k
	CmdFocusRight uint16 = 1017 // Ctrl-G l

	CmdSwapNext uint16 = 1018 // Ctrl-G }
	CmdSwapPrev uint16 = 1019 // Ctrl-G {

	CmdBreakOut uint16 = 1020 // Ctrl-G !

	CmdFocusNext       uint16 = 1023 // Ctrl-G o
	CmdFocusPrev       uint16 = 1024 // Ctrl-G ;
	CmdRespawnPane     uint16 = 1025
	CmdEnterResize     uint16 = 1026 // Ctrl-G R
	CmdCycleLayout     uint16 = 1027 // Ctrl-G Space
	CmdPaneContextMenu uint16 = 1028 // Hidden: right-click handler.

	// 1020s — view
	CmdThemePicker uint16 = 1022 // Ctrl-G T

	// 1030s — edit / clipboard
	CmdEnterCopyMode   uint16 = 1030 // Ctrl-G [
	CmdPaste           uint16 = 1031 // Ctrl-G ]
	CmdFindScrollback  uint16 = 1032 // Ctrl-G /
	CmdToggleSyncInput uint16 = 1033 // Ctrl-G ~
	CmdSendSIGINT      uint16 = 1034
	CmdSendSIGQUIT     uint16 = 1035
	CmdSendEOF         uint16 = 1036
	CmdSendSIGTERM     uint16 = 1037

	// 1040s — connections
	CmdConnectHost uint16 = 1040 // Ctrl-G H
	CmdEditHosts   uint16 = 1041 // Ctrl-G B

	// 1050s — transfer
	CmdSFTPBrowser uint16 = 1050 // Ctrl-G F

	// 1060s — help / meta
	CmdCommandPalette uint16 = 1060 // Ctrl-G P
	CmdResetFirstRun  uint16 = 1061
	CmdCheatsheet     uint16 = 1062
	CmdLiteralPrefix  uint16 = 1063 // Hidden: forwards Ctrl-G to focused pane.
	CmdTickerRedraw   uint16 = 1064 // Hidden: 1s status bar refresh trigger.
	CmdAutoClosePane  uint16 = 1065 // Hidden: triggered by close-on-exit; InfoPtr = *session.Pane.

	// 1090s — window list / find
	CmdWindowList uint16 = 1090 // Ctrl-G w
	CmdFindWindow uint16 = 1091 // Ctrl-G f

	// 1100s — focus window by number
	CmdFocusWindow1 uint16 = 1101 // Ctrl-G 1
	CmdFocusWindow2 uint16 = 1102 // Ctrl-G 2
	CmdFocusWindow3 uint16 = 1103
	CmdFocusWindow4 uint16 = 1104
	CmdFocusWindow5 uint16 = 1105
	CmdFocusWindow6 uint16 = 1106
	CmdFocusWindow7 uint16 = 1107
	CmdFocusWindow8 uint16 = 1108
	CmdFocusWindow9 uint16 = 1109 // Ctrl-G 9
)
