package app

import (
	"fmt"
	"strings"
	"time"
)

// flashNumbers posts a 1.5 s status-bar override listing each window's
// number → title mapping. Cheaper than overlay-on-window, and gives the
// user enough information to use Ctrl-G 1..9. The 1s ticker carries
// the override away once flashUntil expires.
func (m *Mux) flashNumbers() {
	var sb strings.Builder
	for i, key := range m.windowOrder {
		ws := m.windows[key]
		if ws == nil {
			continue
		}
		if i > 0 {
			sb.WriteString("  ")
		}
		title := ws.UserTitle
		if title == "" {
			title = ws.ShellTitle
		}
		if title == "" {
			title = ws.Title
		}
		fmt.Fprintf(&sb, "[%d] %s", ws.Number, title)
	}
	if sb.Len() == 0 {
		return
	}
	m.flashUntil = time.Now().Add(1500 * time.Millisecond)
	m.flashText = sb.String()
	m.refreshStatusBar()
}
