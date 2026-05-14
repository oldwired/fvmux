package app

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/sftp"
)

// transferHintUpload / transferHintDownload run when the user invokes
// the menu/palette entries from outside an open SFTP browser. The
// active F5/F6 hotkeys are scoped to the browser modal — these stubs
// nudge the user toward the right surface rather than no-oping silently.
func (m *Mux) transferHintUpload() {
	m.transferHint("Upload", "F6 inside the SFTP browser uploads a local file to the remote cwd.")
}

func (m *Mux) transferHintDownload() {
	m.transferHint("Download", "F5 inside the SFTP browser downloads the focused remote file.")
}

func (m *Mux) transferHint(title, body string) {
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
		title+": "+body+"\nOpen with Ctrl-G F.", msgbox.OKOnly)
}

// showActiveTransfers reports the running / recent transfers from the
// live SFTP browser, if any. When no browser is open the manager is
// nil and we say so.
func (m *Mux) showActiveTransfers() {
	mgr := sftp.LiveManager()
	if mgr == nil {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No SFTP browser is open. Open one with Ctrl-G F.", msgbox.OKOnly)
		return
	}
	snap := mgr.Snapshot()
	if len(snap) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No transfers queued.", msgbox.OKOnly)
		return
	}
	var sb strings.Builder
	sb.WriteString("Transfers:\n")
	for _, t := range snap {
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
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info, sb.String(), msgbox.OKOnly)
}

// clearCompletedTransfers drops Done/Failed/Cancelled rows from the
// live browser's transfer list.
func (m *Mux) clearCompletedTransfers() {
	mgr := sftp.LiveManager()
	if mgr == nil {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No SFTP browser is open.", msgbox.OKOnly)
		return
	}
	_ = mgr.ClearCompleted()
}
