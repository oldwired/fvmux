package app

// displayTitle is the window's caption fallback chain for list and label
// surfaces (window list, join-from picker, status bar, number flash): an
// explicit user rename wins, else the shell-set OSC title, else the
// profile-derived fallback. Site-specific decoration (number prefixes,
// the whimsy home glyph) stays at the call site.
func (ws *windowState) displayTitle() string {
	if ws.UserTitle != "" {
		return ws.UserTitle
	}
	if ws.ShellTitle != "" {
		return ws.ShellTitle
	}
	return ws.Title
}
