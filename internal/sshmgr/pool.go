// SSH ControlMaster pool. The pool itself spawns no ssh processes —
// instead it tracks alias → socket-path + refcounts, and individual
// connections (interactive ssh, SFTP) carry
//
//	-o ControlMaster=auto -o ControlPath=<sock> -o ControlPersist=600
//
// so the first ssh that connects to an alias becomes the master, and
// every subsequent connection (within ControlPersist) reuses it.
//
// This avoids the "headless ssh -M -N opens /dev/tty for password
// prompts and overwrites fvmux's TUI" problem that an out-of-band
// warmer would have, because the master is established inside a real
// fvmux PTY pane where prompts render correctly.
package sshmgr

import (
	"os"
	"os/exec"
	"sync"
	"time"
)

// Pool is the alias-to-master registry. Acquire is non-blocking: it
// just records intent and returns the path the ssh subprocess should
// pass via -o ControlPath. Liveness is derived from os.Stat on the
// socket file when callers want a snapshot.
type Pool struct {
	socketFor func(alias string) string

	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	alias    string
	sockPath string
	firstUse time.Time
	refcount int
}

// ControlOpts returns the ssh -o flags every connection through the
// pool should carry. Idempotent — safe to splice into any ssh argv.
func ControlOpts(controlPath string) []string {
	if controlPath == "" {
		return nil
	}
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlPath,
		"-o", "ControlPersist=600",
	}
}

// ActiveConn is one row in Pool.Snapshot, used by the Active
// Connections menu/palette entry.
type ActiveConn struct {
	Alias    string
	Sock     string
	Started  time.Time // first Acquire time for the alias.
	Refs     int
	SockLive bool // true ⇒ socket file currently exists (master is up).
}

// NewPool constructs a Pool that resolves alias → socket-path via sockFor.
func NewPool(sockFor func(alias string) string) *Pool {
	return &Pool{socketFor: sockFor, entries: map[string]*entry{}}
}

// Acquire returns the ControlPath to splice into the ssh command via
// ControlOpts. Always succeeds — the actual master comes up when the
// returned path is first used by an ssh subprocess that has
// ControlMaster=auto. Bumps the refcount.
func (p *Pool) Acquire(alias string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[alias]; ok {
		e.refcount++
		return e.sockPath
	}
	sock := p.socketFor(alias)
	p.entries[alias] = &entry{
		alias:    alias,
		sockPath: sock,
		firstUse: time.Now(),
		refcount: 1,
	}
	return sock
}

// Release decrements the refcount. The master is left alive — ssh's
// ControlPersist controls teardown.
func (p *Pool) Release(alias string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[alias]; ok && e.refcount > 0 {
		e.refcount--
	}
}

// Snapshot lists every alias the pool has tracked. SockLive comes
// from os.Stat on the socket file — true if the master is currently
// up. Read-only.
func (p *Pool) Snapshot() []ActiveConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ActiveConn, 0, len(p.entries))
	for _, e := range p.entries {
		live := false
		if _, err := os.Stat(e.sockPath); err == nil {
			live = true
		}
		out = append(out, ActiveConn{
			Alias: e.alias, Sock: e.sockPath,
			Started: e.firstUse, Refs: e.refcount, SockLive: live,
		})
	}
	return out
}

// Shutdown asks every live master to exit cleanly via `ssh -O exit`.
// Cheap and non-interactive. Skipped for aliases whose socket has
// already gone away (ControlPersist may have already cleaned up).
// Called from cmd/fvmux's deferred shutdown.
func (p *Pool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if _, err := os.Stat(e.sockPath); err != nil {
			continue
		}
		cmd := exec.Command("ssh",
			"-O", "exit",
			"-o", "ControlPath="+e.sockPath,
			e.alias,
		)
		_ = cmd.Run()
		_ = os.Remove(e.sockPath)
	}
	p.entries = nil
}
