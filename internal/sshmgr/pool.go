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
	"net"
	"os"
	"path/filepath"
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

// Snapshot lists every alias the pool has tracked. SockLive reflects
// whether a master is actually listening on the socket (not merely that
// the socket file exists — a crashed master can leave a stale file
// behind). Read-only.
func (p *Pool) Snapshot() []ActiveConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ActiveConn, 0, len(p.entries))
	for _, e := range p.entries {
		out = append(out, ActiveConn{
			Alias: e.alias, Sock: e.sockPath,
			Started: e.firstUse, Refs: e.refcount, SockLive: sockAlive(e.sockPath),
		})
	}
	return out
}

// sockAlive reports whether a ControlMaster is currently listening on
// sockPath. A bare connect+close is enough: a live master accepts it,
// while a stale socket file (master gone) refuses the connection. This
// avoids the false-positive of os.Stat, which only proves the file
// exists.
func sockAlive(sockPath string) bool {
	c, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// SweepStale removes orphaned ControlMaster socket FILES left in dir by a
// previously-crashed fvmux — but only those with no live master (a
// connect is refused). Live sockets are left untouched: another fvmux
// instance may be sharing them, and any genuinely-orphaned-but-alive
// master self-terminates via ControlPersist. Called once at startup;
// never kills a process, so it is safe under the multi-instance design.
func SweepStale(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.sock"))
	if err != nil {
		return
	}
	for _, sock := range matches {
		if !sockAlive(sock) {
			_ = os.Remove(sock)
		}
	}
}

// Shutdown drops the pool's bookkeeping and removes only *dead* socket
// files (the same rule SweepStale applies). Live masters are
// deliberately left running: the socket path is a deterministic
// per-user hash, so masters are shared with other fvmux instances, and
// an `ssh -O exit` here would cut another instance's SSH panes and
// in-flight transfers. A master nobody is using reaps itself via
// ControlPersist=600. Called from cmd/fvmux's deferred shutdown.
func (p *Pool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if !sockAlive(e.sockPath) {
			_ = os.Remove(e.sockPath)
		}
	}
	// Reset to an empty (non-nil) map rather than nil: an in-flight poll
	// goroutine racing shutdown may still Acquire, and assigning into a
	// nil map panics.
	p.entries = map[string]*entry{}
}
