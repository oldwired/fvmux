// SSH ControlMaster pool. One refcounted master per alias; subsequent
// connections (interactive ssh, SFTP) plug into the existing channel
// via `-S <socket>` and skip re-authentication.
package sshmgr

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Pool keeps a long-running `ssh -M -N` per alias and tracks how many
// fvmux features are currently using each. Masters survive after the
// refcount hits zero — ssh's own ControlPersist handles teardown after
// a quiet period, so the next Acquire for the same alias can still
// piggy-back on the warm session if it arrives quickly enough.
type Pool struct {
	socketFor func(alias string) string

	mu      sync.Mutex
	masters map[string]*master
}

type master struct {
	alias    string
	sockPath string
	cmd      *exec.Cmd
	started  time.Time
	refcount int
}

// ActiveConn is one row in Pool.Snapshot, used by the Active
// Connections menu/palette entry.
type ActiveConn struct {
	Alias   string
	Sock    string
	Started time.Time
	Refs    int
}

// NewPool constructs a Pool that resolves alias → socket-path via sockFor.
func NewPool(sockFor func(alias string) string) *Pool {
	return &Pool{socketFor: sockFor, masters: map[string]*master{}}
}

// Acquire returns the ControlPath callers should use with
// `ssh -S <path> alias ...`. The first call spawns the master and
// blocks (up to 10 s) waiting for the socket to appear; subsequent
// calls bump the refcount and return immediately. The caller MUST
// pair every Acquire with a Release.
func (p *Pool) Acquire(alias string) (string, error) {
	p.mu.Lock()
	if m, ok := p.masters[alias]; ok {
		m.refcount++
		path := m.sockPath
		p.mu.Unlock()
		return path, nil
	}
	sock := p.socketFor(alias)
	cmd := exec.Command("ssh",
		"-M", "-N",
		"-o", "ControlMaster=yes",
		"-o", "ControlPath="+sock,
		"-o", "ControlPersist=600",
		alias,
	)
	// Unlock while we Start + wait — Start blocks on fork/exec; the
	// other goroutines should see an empty map and treat that as
	// "we'll race to spawn it". Acceptable for fvmux's interactive
	// cadence (humans don't double-click open).
	p.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("ssh control master start: %w", err)
	}
	if err := waitForSocket(sock, 10*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", err
	}

	p.mu.Lock()
	// Re-check after Start: another goroutine might have raced.
	if existing, ok := p.masters[alias]; ok {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		existing.refcount++
		path := existing.sockPath
		p.mu.Unlock()
		return path, nil
	}
	p.masters[alias] = &master{
		alias:    alias,
		sockPath: sock,
		cmd:      cmd,
		started:  time.Now(),
		refcount: 1,
	}
	p.mu.Unlock()
	return sock, nil
}

// Release decrements the refcount. The master is left alive — ssh's
// ControlPersist controls teardown.
func (p *Pool) Release(alias string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if m, ok := p.masters[alias]; ok && m.refcount > 0 {
		m.refcount--
	}
}

// Snapshot lists every master the pool tracks. Read-only.
func (p *Pool) Snapshot() []ActiveConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ActiveConn, 0, len(p.masters))
	for _, m := range p.masters {
		out = append(out, ActiveConn{
			Alias: m.alias, Sock: m.sockPath, Started: m.started, Refs: m.refcount,
		})
	}
	return out
}

// Shutdown kills every active master. Called on Mux shutdown so we
// don't leak orphan ssh processes when fvmux quits before
// ControlPersist expires.
func (p *Pool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, m := range p.masters {
		if m.cmd != nil && m.cmd.Process != nil {
			_ = m.cmd.Process.Kill()
			_ = m.cmd.Wait()
		}
		_ = os.Remove(m.sockPath)
	}
	p.masters = nil
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("ssh control socket %s not ready within %s", path, timeout)
}
