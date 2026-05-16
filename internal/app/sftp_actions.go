package app

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/sftp"
)

// transferHintUpload / transferHintDownload run when the user invokes
// the menu/palette entries from outside an open SFTP browser. The
// transfer actions are scoped to the browser's Copy button / F5 key;
// these stubs nudge the user toward the right surface rather than
// no-oping silently.
func (m *Mux) transferHintUpload() {
	m.transferHint("Upload", "Use the Copy button (or F5) inside the SFTP browser, with focus on a local listing entry.")
}

func (m *Mux) transferHintDownload() {
	m.transferHint("Download", "Use the Copy button (or F5) inside the SFTP browser, with focus on a remote listing entry.")
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
