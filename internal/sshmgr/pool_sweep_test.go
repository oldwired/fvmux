package sshmgr

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSweepStale_RemovesDeadKeepsLive(t *testing.T) {
	dir := t.TempDir()

	// A stale socket file with nobody listening.
	dead := filepath.Join(dir, "dead.sock")
	if err := os.WriteFile(dead, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// A live socket with a real listener — must survive the sweep.
	live := filepath.Join(dir, "live.sock")
	ln, err := net.Listen("unix", live)
	if err != nil {
		t.Skipf("cannot create unix listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	SweepStale(dir)

	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Errorf("stale socket should have been removed; stat err = %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("live socket should have survived; stat err = %v", err)
	}
}

func TestShutdown_ThenAcquireDoesNotPanic(t *testing.T) {
	p := NewPool(func(alias string) string { return filepath.Join(t.TempDir(), alias+".sock") })
	p.Acquire("h1")
	p.Shutdown() // no live sockets ⇒ just resets the map
	// A late Acquire (e.g. from an in-flight poll) must not panic on a
	// nil map.
	if sock := p.Acquire("h2"); sock == "" {
		t.Fatal("Acquire after Shutdown returned empty")
	}
}
