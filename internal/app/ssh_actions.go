package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/sshmgr"
)

// showActiveConnections lists masters in the SSH ControlMaster pool —
// alias, refcount, uptime. Empty pool gets a friendly "none" message.
func (m *Mux) showActiveConnections() {
	if m.sshPool == nil {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"SSH pool is not initialised.", msgbox.OKOnly)
		return
	}
	snap := m.sshPool.Snapshot()
	if len(snap) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No active SSH connections. Open one with Ctrl-G H.", msgbox.OKOnly)
		return
	}
	var sb strings.Builder
	sb.WriteString("Tracked aliases:\n\n")
	for _, c := range snap {
		age := time.Since(c.Started).Truncate(time.Second)
		state := "dormant"
		if c.SockLive {
			state = "master up"
		}
		fmt.Fprintf(&sb, "• %s — %d ref%s — first used %s ago — %s\n",
			c.Alias, c.Refs, pluralS(c.Refs), age, state)
	}
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info, sb.String(), msgbox.OKOnly)
}

// reloadHosts re-reads hosts.toml. The picker reads it on each open,
// so this is mostly a clarity / UX entry; the side effect is the
// status-bar info popup confirming the file parsed.
func (m *Mux) reloadHosts() {
	// Picker re-reads from disk on each invocation, so all we need to
	// do is validate the file. Doing this here surfaces parse errors
	// at a user-visible moment rather than the next Ctrl-G H.
	hosts, err := sshmgr.Load(m.Opts.Paths.HostsFile())
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"hosts.toml: %s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
		"hosts.toml reloaded — %d host%s known.",
		[]any{len(hosts), pluralS(len(hosts))}, msgbox.OKOnly)
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
