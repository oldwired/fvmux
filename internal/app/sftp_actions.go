package app

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/sftp"
)

// transferHintUpload / transferHintDownload run when the user invokes the
// Move (F6) / Copy (F5) menu-palette entries from outside an open SFTP
// browser. Both keys work in either direction on the highlighted entry, so
// these stubs point at the browser rather than no-oping silently.
func (m *Mux) transferHintUpload() {
	m.transferHint("Move / Rename", "Highlight a file or folder in either listing, then F6 to move or rename it — a bare name renames in place, a path moves it to the other side.")
}

func (m *Mux) transferHintDownload() {
	m.transferHint("Copy", "Highlight a file or folder in either listing, then F5 (or the Copy button) to copy it to the other side.")
}

func (m *Mux) transferHint(title, body string) {
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
		title+": "+body+"\nOpen with Ctrl-G F.", msgbox.OKOnly)
}

// showActiveTransfers reports running / recent transfers across every
// open SFTP browser. Non-modal browsers may run concurrently, so this
// walks the full set of live managers.
func (m *Mux) showActiveTransfers() {
	mgrs := sftp.LiveManagers()
	if len(mgrs) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No SFTP browser is open. Open one with Ctrl-G F.", msgbox.OKOnly)
		return
	}
	var any bool
	var sb strings.Builder
	sb.WriteString("Transfers:\n")
	for _, mgr := range mgrs {
		for _, t := range mgr.Snapshot() {
			any = true
			dir := "↑"
			if t.Direction == sftp.Download {
				dir = "↓"
			}
			short := t.LocalPath
			if t.Direction == sftp.Download {
				short = t.RemotePath
			}
			state := "active"
			switch t.Status() {
			case sftp.StatusDone:
				state = "done"
			case sftp.StatusFailed:
				state = "failed: " + t.Error()
			case sftp.StatusCancelled:
				state = "cancelled"
			}
			sb.WriteString(dir + " " + short + "  — " + state + "\n")
		}
	}
	if !any {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No transfers queued.", msgbox.OKOnly)
		return
	}
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info, sb.String(), msgbox.OKOnly)
}

// clearCompletedTransfers drops Done/Failed/Cancelled rows from every
// open browser's transfer list.
func (m *Mux) clearCompletedTransfers() {
	mgrs := sftp.LiveManagers()
	if len(mgrs) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No SFTP browser is open.", msgbox.OKOnly)
		return
	}
	for _, mgr := range mgrs {
		_ = mgr.ClearCompleted()
	}
}
