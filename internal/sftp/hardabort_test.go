package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
)

// fakeConn is a dedicatedConn backed by an in-process client. Close is
// counted and signals on first call, so tests can assert the abort
// watcher closed the session.
type fakeConn struct {
	client   *pkgsftp.Client
	closes   atomic.Int32
	closedCh chan struct{}
	once     sync.Once
}

func (f *fakeConn) SFTP() *pkgsftp.Client { return f.client }

func (f *fakeConn) Close() error {
	f.closes.Add(1)
	f.once.Do(func() { close(f.closedCh) })
	return nil
}

func TestAbortWatcher_CancelClosesConnImmediately(t *testing.T) {
	m := NewManager("x")
	tr := &Transfer{cancel: make(chan struct{}), done: make(chan struct{})}
	fc := &fakeConn{closedCh: make(chan struct{})}

	m.wg.Add(1)
	go m.abortWatcher(tr, fc)

	tr.requestCancel()
	select {
	case <-fc.closedCh:
		// good: closed on cancel, before the transfer's run unwinds.
	case <-time.After(2 * time.Second):
		t.Fatal("dedicated conn not closed on cancel")
	}
	close(tr.done) // let run's side unwind.
	m.Wait()
	if fc.closes.Load() < 1 {
		t.Fatalf("expected ≥1 Close, got %d", fc.closes.Load())
	}
}

func TestAbortWatcher_ReapsOnCompletion(t *testing.T) {
	m := NewManager("x")
	tr := &Transfer{cancel: make(chan struct{}), done: make(chan struct{})}
	fc := &fakeConn{closedCh: make(chan struct{})}

	m.wg.Add(1)
	go m.abortWatcher(tr, fc)
	close(tr.done) // normal completion, never cancelled.
	m.Wait()

	if got := fc.closes.Load(); got != 1 {
		t.Fatalf("closes = %d, want exactly 1 (reaped once)", got)
	}
}

func TestStartDedicated_FallsBackToSharedWhenNoOpener(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "s")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "d")

	m := NewManager("x") // openDedicated is nil.
	tr, err := m.StartDedicated(c, Upload, src, dst, nil)
	if err != nil {
		t.Fatalf("StartDedicated: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusDone {
		t.Fatalf("status=%d, want Done", s)
	}
	if b, _ := os.ReadFile(dst); string(b) != "data" {
		t.Fatalf("dst = %q, want data", b)
	}
}

func TestStartDedicated_FallsBackWhenOpenerErrors(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "s")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "d")

	m := NewManager("x")
	m.openDedicated = func() (dedicatedConn, error) { return nil, errors.New("no ControlMaster") }
	tr, err := m.StartDedicated(c, Upload, src, dst, nil)
	if err != nil {
		t.Fatalf("StartDedicated: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusDone {
		t.Fatalf("status=%d, want Done via shared fallback", s)
	}
}

func TestStartDedicated_UsesAndReapsDedicatedConn(t *testing.T) {
	shared := newTestClient(t)
	dedicated := newTestClient(t)
	fc := &fakeConn{client: dedicated, closedCh: make(chan struct{})}

	dir := t.TempDir()
	src := filepath.Join(dir, "s")
	if err := os.WriteFile(src, []byte("via-dedicated"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "d")

	m := NewManager("x")
	m.openDedicated = func() (dedicatedConn, error) { return fc, nil }

	tr, err := m.StartDedicated(shared, Upload, src, dst, nil)
	if err != nil {
		t.Fatalf("StartDedicated: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusDone {
		t.Fatalf("status=%d, want Done", s)
	}
	m.Wait() // drains the abort watcher.

	if b, _ := os.ReadFile(dst); string(b) != "via-dedicated" {
		t.Fatalf("dst = %q, want via-dedicated", b)
	}
	select {
	case <-fc.closedCh:
		// good: dedicated session was reaped.
	default:
		t.Fatal("dedicated conn was not closed after completion")
	}
}
