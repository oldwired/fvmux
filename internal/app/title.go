package app

// displayTitle is the window's caption fallback chain for list and label
// surfaces (window list, join-from picker, status bar, number flash): an
// explicit user rename wins, else the shell-set OSC title, else the
// profile-derived fallback. Site-specific decoration (number prefixes,
// the whimsy home glyph) stays at the call site.
func (ws *windowState) displayTitle() string {
	base := ""
	if ws.UserTitle != "" {
		base = ws.UserTitle
	} else if ws.ShellTitle != "" {
		base = ws.ShellTitle
	} else {
		base = ws.Title
	}
	if ws.Focus != nil && ws.Focus.Pane != nil && ws.Focus.Pane.SSHAlias != "" {
		alias := ws.Focus.Pane.SSHAlias
		if base == "" || base == alias {
			return "[" + alias + "] terminal"
		}
		return "[" + alias + "] terminal · " + base
	}
	return base
}
