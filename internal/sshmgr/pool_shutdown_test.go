package sshmgr

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// TestShutdown_RemovesDeadKeepsLiveAndResets pins the fix for finding #2:
// Pool.Shutdown must no longer run `ssh -O exit`. It only removes socket
// FILES with no live master listening (per sockAlive); a live socket —
// possibly shared with another fvmux instance — is left untouched, and
// the pool's bookkeeping is reset.
func TestShutdown_RemovesDeadKeepsLiveAndResets(t *testing.T) {
	// A short temp dir keeps the unix socket path well under the ~104-byte
	// limit that t.TempDir()'s long test-name paths can blow on darwin.
	dir, err := os.MkdirTemp("", "fvp")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	livePath := filepath.Join(dir, "live.sock")
	stalePath := filepath.Join(dir, "stale.sock")

	// "live" has a real listener accepting connections — a master is up.
	ln, err := net.Listen("unix", livePath)
	if err != nil {
		t.Skipf("cannot create unix listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	// "stale" is just a leftover file with nobody listening.
	if err := os.WriteFile(stalePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	sockFor := map[string]string{"live": livePath, "stale": stalePath}
	p := NewPool(func(alias string) string { return sockFor[alias] })
	p.Acquire("live")
	p.Acquire("stale")

	p.Shutdown()

	if _, err := os.Stat(livePath); err != nil {
		t.Errorf("live socket should survive Shutdown; stat err = %v", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("stale socket should be removed by Shutdown; stat err = %v", err)
	}
	if snap := p.Snapshot(); len(snap) != 0 {
		t.Errorf("Snapshot after Shutdown = %d entries; want 0", len(snap))
	}
}
