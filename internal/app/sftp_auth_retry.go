package app

import (
	"io"
	"log/slog"
	"os/exec"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/sftp"
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
	// a failed auth doesn't leak a forever-goroutine. Registered with
	// sftpRestore so a session switch cancels a stale poll (otherwise it
	// could pop a browser into the wrong session). The Acquire above is
	// paired with a Release on whichever exit path runs.
	cancel := m.sftpRestore.start(alias)
	go func() {
		defer m.sftpRestore.finish(alias, cancel)
		alive := pollForMasterCancellable(alias, sock, 30*time.Second, cancel)
		views.CallSoon(func() {
			m.sshPool.Release(alias)
			if !alive {
				return // user gave up / auth failed; pane is still there.
			}
			m.openSftpBrowser(alias)
		})
	}()
}

// masterAlive runs `ssh -O check -o ControlPath=sock alias` and
// returns true iff exit status is 0. Stdout/stderr suppressed so the
// poll doesn't spam the outer terminal.
func masterAlive(alias, sock string) bool {
	cmd := exec.Command("ssh",
		"-O", "check",
		"-o", "ControlPath="+sock,
		alias,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// scheduleSftpRestore is the session-restore counterpart to
// offerAuthThenRetry. Same poll-then-act shape but no msgbox prompt
// — the user is presumed to be authenticating in the SSH pane that
// the session just reopened. When the master is alive AND
// authenticated, the SFTP browser opens. Unlike the picker path, a
// failed SFTP open here does NOT cascade into another offer-to-auth
// dialog (otherwise restore could spawn a duplicate ssh pane).
//
// Registered in m.sftpRestore so session-change surfaces can cancel
// in-flight restores — without this, a stale poll from the previous
// session could pop a SFTP browser the next time the user opens an
// SSH connection to the same alias.
//
// 30 s timeout (used to be 120 s) — enough time to type a passphrase
// without leaving a stale poll lingering past the user's attention.
func (m *Mux) scheduleSftpRestore(alias string) {
	sock := m.sshPool.Acquire(alias)
	cancel := m.sftpRestore.start(alias)
	go func() {
		defer m.sftpRestore.finish(alias, cancel)
		alive := pollForMasterCancellable(alias, sock, 30*time.Second, cancel)
		views.CallSoon(func() {
			m.sshPool.Release(alias)
			if !alive {
				return
			}
			// Direct sftp.Show — bypass openSftpBrowser to avoid its
			// offerAuthThenRetry fallback. If the master is alive but
			// SFTP somehow still fails, log and move on.
			sock := m.sshPool.Acquire(alias)
			// sftp.Show calls onClose (→ Release) itself on every error
			// path, so do NOT Release again here — that would double-count
			// and drive the alias refcount below its true value.
			if err := sftp.Show(m.App, alias, sock, func() { m.sshPool.Release(alias) }); err != nil {
				slog.Warn("session restore: sftp browser open failed",
					"alias", alias, "err", err)
			}
		})
	}()
}

// pollForMasterCancellable is pollForMaster with a cancel channel.
// Returns true iff the master became alive before the channel closed
// or the deadline elapsed.
func pollForMasterCancellable(alias, sock string, timeout time.Duration, cancel <-chan struct{}) bool {
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		if masterAlive(alias, sock) {
			return true
		}
		select {
		case <-cancel:
			return false
		case <-tick.C:
			if time.Now().After(deadline) {
				return false
			}
		}
	}
}
