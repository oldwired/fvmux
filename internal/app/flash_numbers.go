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
		num, title := m.workspaceWindowNumber(key), m.workspaceWindowTitle(key)
		if num == 0 {
			continue
		}
		if i > 0 {
			sb.WriteString("  ")
		}
		fmt.Fprintf(&sb, "[%d] %s", num, title)
	}
	if sb.Len() == 0 {
		return
	}
	m.setFlash(sb.String(), 1500*time.Millisecond, flashPrioNumbers)
	m.refreshStatusBar()
}
