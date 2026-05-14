package app

import (
	"time"

	"github.com/oldwired/fvmux/internal/session"
)

// flashOnBell is the OnBell callback installed by wireTerminalCallbacks
// when bell flash is enabled. Records the bell timestamp on the pane
// (the status-bar '!' marker reads it) AND flashes the frame's title
// bar for 320 ms — two 80 ms ticks, debounced upstream by fv-go's
// Window.FlashTitleBar (≥ 500 ms between calls).
//
// Honours config: when [appearance] bell == "off", neither marker nor
// flash fires. "flash" → flash only. "notify" → marker only (future
// hook for desktop notifications). "both" → flash + marker.
func (m *Mux) flashOnBell(pane *session.Pane) {
	mode := m.Opts.Config.Appearance.Bell
	if mode == "" {
		mode = "flash"
	}
	if mode == "off" {
		return
	}

	if mode == "flash" || mode == "both" {
		// Find the window containing this pane to flash its frame.
		for _, ws := range m.windows {
			if ws == nil || ws.Root == nil || ws.Frame == nil {
				continue
			}
			if ws.Root.FindByID(pane.ID) != nil {
				ws.Frame.FlashTitleBar(320 * time.Millisecond)
				break
			}
		}
	}
	if mode == "flash" || mode == "notify" || mode == "both" {
		pane.BellAt = time.Now()
	}
}
