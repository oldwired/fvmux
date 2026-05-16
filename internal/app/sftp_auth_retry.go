package app

import (
	"os"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/sshmgr"
)

// offerAuthThenRetry is the auth-required recovery flow. SFTP refused
// because BatchMode=yes wouldn't let ssh prompt for credentials.
// Offers the user a one-click "open an SSH terminal, authenticate
// there, fvmux will retry the SFTP browser once the ControlMaster
// is up" path.
//
// The SSH pane is a normal interactive window — the user keeps it
// open after auth and can work in it; the SFTP browser pops up
// alongside as soon as the control socket appears.
func (m *Mux) offerAuthThenRetry(alias string) {
	// msgbox is a hardcoded 50×8 dialog with a 3-row text area, so
	// keep this to ≤ 3 lines of ≤ 46 chars each. The "SFTP opens
	// after" hint goes on the third line.
	prompt := "Authentication required for " + alias + ".\n" +
		"Open an SSH window to log in?\n" +
		"(SFTP browser opens after.)"
	if got := msgbox.Show(&m.App.Desktop.Group, msgbox.Question,
		prompt, msgbox.YesNo); got != consts.CmYes {
		return
	}

	// Spawn the interactive ssh pane through the pool so it uses the
	// same control socket we'll watch.
	sock := m.sshPool.Acquire(alias)
	args := append([]string{}, sshmgr.ControlOpts(sock)...)
	args = append(args, alias)
	prof := &profile.Profile{
		Name:    alias,
		Command: "ssh",
		Args:    args,
		Title:   alias,
	}
	if _, err := m.openWindowFromProfile(prof); err != nil {
		m.sshPool.Release(alias)
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"ssh %s failed:\n%s", []any{alias, err.Error()}, msgbox.OKOnly)
		return
	}

	// Poll for the master socket. Fires the SFTP retry on the UI
	// goroutine via views.CallSoon. Bounded: gives up after 30 s so
	// a failed auth doesn't leak a forever-goroutine. The Acquire
	// above is paired with a Release on whichever exit path runs.
	go pollForMaster(alias, sock, 30*time.Second, func(ok bool) {
		views.CallSoon(func() {
			m.sshPool.Release(alias)
			if !ok {
				return // user gave up / auth failed; pane is still there.
			}
			m.openSftpBrowser(alias)
		})
	})
}

// pollForMaster looks for the ControlMaster socket file at sock,
// every 500 ms up to timeout. Calls done(true) when found, done(false)
// on timeout. Runs in a goroutine — done is called from the goroutine,
// caller marshals onto the UI thread.
func pollForMaster(alias, sock string, timeout time.Duration, done func(ok bool)) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			done(true)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	done(false)
}
