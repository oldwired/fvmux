package sftp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	pkgsftp "github.com/pkg/sftp"

	"github.com/oldwired/fvmux/internal/sshmgr"
)

// ErrAuthRequired is returned by Open when ssh refused because
// BatchMode=yes is set and auth would need interaction (no master,
// agent doesn't have the key, password auth, etc.). Callers can
// detect this via errors.Is and offer to spawn an interactive ssh
// session that warms the ControlMaster.
var ErrAuthRequired = errors.New("ssh authentication needs interaction")

// Client wraps a pkg/sftp.Client whose underlying transport is a
// long-running `ssh ... -s alias sftp` subprocess. Delegating to the
// system ssh keeps known_hosts verification, ssh-agent, and ProxyCommand
// behaving exactly as the user expects from their other ssh-based tools.
type Client struct {
	cmd       *exec.Cmd
	sftp      *pkgsftp.Client
	in        io.WriteCloser
	out       io.ReadCloser
	stderr    *bytes.Buffer
	closeOnce sync.Once
}

// Open spawns ssh and negotiates an SFTP session against alias.
// controlPath, when non-empty, threads ControlMaster=auto +
// ControlPath=<sock> so the session reuses a master if one is up.
// hostOpts carries a hosts.toml entry's HostName/User/Port overrides
// (sshmgr.Host.ConnectOpts) — without them a standalone hosts.toml
// host is unconnectable because the bare alias isn't a hostname.
//
// The ssh subprocess runs without a controlling TTY (this is a pipe-
// based subsystem call, not an interactive shell), so we force
// BatchMode=yes — preventing ssh from trying to write a password
// prompt to /dev/tty and corrupting fvmux's display. If auth would
// require interaction, ssh fails immediately and the surfaced error
// tells the user to authenticate via Ctrl-G H first (that path runs
// inside a real PTY pane where prompts are renderable).
func Open(alias, controlPath string, hostOpts []string) (*Client, error) {
	return OpenContext(context.Background(), alias, controlPath, hostOpts)
}

// OpenContext is Open with a cancellable ssh subprocess lifetime. Cancelling
// ctx interrupts both the ssh connection attempt and SFTP negotiation, which
// lets the owning workspace/session prevent a late browser from appearing
// after it has already been closed or replaced.
func OpenContext(ctx context.Context, alias, controlPath string, hostOpts []string) (*Client, error) {
	args := []string{}
	args = append(args, sshmgr.ControlOpts(controlPath)...)
	args = append(args, hostOpts...)
	args = append(args, "-o", "BatchMode=yes")
	args = append(args, "-s", alias, "sftp")
	cmd := exec.CommandContext(ctx, "ssh", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh stdout: %w", err)
	}
	// Capture stderr so an early auth failure surfaces in the dialog
	// instead of going to fvmux's outer terminal.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ssh start: %w", err)
	}
	sc, err := pkgsftp.NewClientPipe(stdout, stdin)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, classifyOpenError(alias, err, stderr.String())
	}
	return &Client{cmd: cmd, sftp: sc, in: stdin, out: stdout, stderr: &stderr}, nil
}

// classifyOpenError turns ssh's stderr blob into a friendlier error
// when the failure is "auth needed but BatchMode is on" — the
// common case for a first-time SFTP to an alias whose ControlMaster
// isn't warm yet. Auth-required errors wrap ErrAuthRequired so the
// caller can detect them via errors.Is and offer to spawn an
// interactive ssh pane to warm the master.
func classifyOpenError(alias string, err error, stderr string) error {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "permission denied"),
		strings.Contains(s, "password"),
		strings.Contains(s, "publickey"),
		strings.Contains(s, "host key verification failed"):
		return fmt.Errorf("%w: ssh refused for %s: %s",
			ErrAuthRequired, alias, strings.TrimSpace(stderr))
	}
	if strings.TrimSpace(stderr) == "" {
		return fmt.Errorf("sftp negotiate: %w", err)
	}
	return fmt.Errorf("sftp negotiate: %w\nssh stderr: %s", err, strings.TrimSpace(stderr))
}

// SFTP returns the underlying pkg/sftp client for direct calls.
func (c *Client) SFTP() *pkgsftp.Client { return c.sftp }

// Close tears down the sftp session and the ssh subprocess.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.sftp != nil {
			_ = c.sftp.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
	})
	return nil
}
