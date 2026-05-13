package sftp

import (
	"fmt"
	"io"
	"os/exec"

	pkgsftp "github.com/pkg/sftp"
)

// Client wraps a pkg/sftp.Client whose underlying transport is a
// long-running `ssh -s <alias> sftp` subprocess. Delegating to the
// system ssh keeps known_hosts verification, ssh-agent, and ProxyCommand
// behaving exactly as the user expects from their other ssh-based tools.
type Client struct {
	cmd  *exec.Cmd
	sftp *pkgsftp.Client
	in   io.WriteCloser
	out  io.ReadCloser
}

// Open spawns ssh and negotiates an SFTP session against alias. The
// alias is resolved by the system ssh client according to ~/.ssh/config.
func Open(alias string) (*Client, error) {
	cmd := exec.Command("ssh", "-s", alias, "sftp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ssh stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ssh start: %w", err)
	}
	sc, err := pkgsftp.NewClientPipe(stdout, stdin)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("sftp negotiate: %w", err)
	}
	return &Client{cmd: cmd, sftp: sc, in: stdin, out: stdout}, nil
}

// SFTP returns the underlying pkg/sftp client for direct calls.
func (c *Client) SFTP() *pkgsftp.Client { return c.sftp }

// Close tears down the sftp session and the ssh subprocess.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.sftp != nil {
		_ = c.sftp.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	return nil
}
