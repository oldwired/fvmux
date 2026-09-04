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
	all := m.sshPool.Snapshot()
	snap := all[:0]
	for _, c := range all {
		if c.SockLive {
			snap = append(snap, c)
		}
	}
	if len(snap) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No live shared SSH connections.", msgbox.OKOnly)
		return
	}
	var sb strings.Builder
	sb.WriteString("Live shared SSH connections:\n\n")
	for _, c := range snap {
		age := time.Since(c.Started).Truncate(time.Second)
		fmt.Fprintf(&sb, "• [%s] — %d terminal/file user%s — connected %s ago\n",
			c.Alias, c.Refs, pluralS(c.Refs), age)
	}
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info, sb.String(), msgbox.OKOnly)
}

// reloadHosts backs the "Validate hosts.toml" command. Nothing caches
// host data — every picker re-reads the file — so the command's whole
// job is a parse check that surfaces errors at a user-visible moment
// rather than on the next Ctrl-G H.
func (m *Mux) reloadHosts() {
	hosts, err := sshmgr.Load(m.Opts.Paths.HostsFile())
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"hosts.toml: %s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
		"hosts.toml OK — %d host%s known.",
		[]any{len(hosts), pluralS(len(hosts))}, msgbox.OKOnly)
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
