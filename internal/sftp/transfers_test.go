package sftp

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"
)

// newActiveTransfer constructs a Transfer in the same shape Start
// would, but without touching pkgsftp.Client — sufficient for testing
// cancel semantics in isolation.
func newActiveTransfer() *Transfer {
	t := &Transfer{
		Direction:  Upload,
		LocalPath:  "/tmp/local",
		RemotePath: "/tmp/remote",
		Size:       1024,
		StartedAt:  time.Now(),
		cancel:     make(chan struct{}),
	}
	// Default atomic.Int32 is 0 == StatusActive — no extra setup.
	return t
}

func TestTransfer_RequestCancel_Idempotent(t *testing.T) {
	tr := newActiveTransfer()
	tr.requestCancel()
	tr.requestCancel() // must not panic on closed channel
	tr.requestCancel()

	select {
	case <-tr.cancel:
		// good
	default:
		t.Fatal("cancel channel should be closed after requestCancel")
	}
}

func TestCancelLast_DoubleCallSafe(t *testing.T) {
	m := NewManager("test-alias")
	tr := newActiveTransfer()
	m.mu.Lock()
	m.list = append(m.list, tr)
	m.mu.Unlock()

	if !m.CancelLast() {
		t.Fatal("first CancelLast should return true")
	}
	// Second call: status is still Active (we have no goroutine to
	// transition it). Must still not panic.
	if !m.CancelLast() {
		t.Fatal("second CancelLast on still-active transfer should return true")
	}

	// Channel should be closed exactly once — receive must succeed
	// without blocking.
	select {
	case <-tr.cancel:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("cancel channel not closed")
	}
}

func TestCancelLast_ConcurrentRace(t *testing.T) {
	m := NewManager("test-alias")
	tr := newActiveTransfer()
	m.mu.Lock()
	m.list = append(m.list, tr)
	m.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.CancelLast()
		}()
	}
	wg.Wait()

	select {
	case <-tr.cancel:
	default:
		t.Fatal("cancel channel should be closed after concurrent CancelLast")
	}
}

func TestCancelAll(t *testing.T) {
	m := NewManager("test-alias")
	a, b, c := newActiveTransfer(), newActiveTransfer(), newActiveTransfer()
	m.mu.Lock()
	m.list = append(m.list, a, b, c)
	m.mu.Unlock()

	m.CancelAll()
	m.CancelAll() // idempotent

	for _, tr := range []*Transfer{a, b, c} {
		select {
		case <-tr.cancel:
		default:
			t.Fatalf("transfer %p was not cancelled", tr)
		}
	}
}

func TestCancelLast_NoActive(t *testing.T) {
	m := NewManager("test-alias")
	if m.CancelLast() {
		t.Fatal("CancelLast on empty manager should return false")
	}
}

func TestTransfer_Cancelled(t *testing.T) {
	tr := newActiveTransfer()
	if tr.cancelled() {
		t.Fatal("fresh transfer should not report cancelled")
	}
	tr.requestCancel()
	if !tr.cancelled() {
		t.Fatal("transfer should report cancelled after requestCancel")
	}
}

func TestManagerWait_ReturnsWithNoTransfers(t *testing.T) {
	m := NewManager("a")
	done := make(chan struct{})
	go func() { m.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Wait hung with no transfers")
	}
}

func TestPump_CopiesAllBytesAndCountsThem(t *testing.T) {
	tr := newActiveTransfer()
	data := bytes.Repeat([]byte("xyz"), 100_000) // > one 64 KiB chunk
	var dst bytes.Buffer
	if err := pump(bytes.NewReader(data), &dst, tr); err != nil {
		t.Fatalf("pump: %v", err)
	}
	if !bytes.Equal(dst.Bytes(), data) {
		t.Fatal("pump did not copy bytes faithfully")
	}
	if tr.Bytes() != int64(len(data)) {
		t.Fatalf("byte counter = %d, want %d", tr.Bytes(), len(data))
	}
}

func TestPump_StopsOnCancel(t *testing.T) {
	tr := newActiveTransfer()
	tr.requestCancel() // cancelled before any chunk is processed.
	var dst bytes.Buffer
	if err := pump(bytes.NewReader(bytes.Repeat([]byte("a"), 1000)), &dst, tr); err != errCancelled {
		t.Fatalf("pump err = %v, want errCancelled", err)
	}
	if dst.Len() != 0 {
		t.Fatalf("cancelled pump copied %d bytes, want 0", dst.Len())
	}
}

func TestPump_WriteStallFailsLoudly(t *testing.T) {
	tr := newActiveTransfer()
	if err := pump(bytes.NewReader([]byte("data")), stallWriter{}, tr); err != errWriteStall {
		t.Fatalf("pump err = %v, want errWriteStall", err)
	}
}

func TestPump_PropagatesReadError(t *testing.T) {
	tr := newActiveTransfer()
	want := errors.New("boom")
	var dst bytes.Buffer
	if err := pump(errReader{err: want}, &dst, tr); err != want {
		t.Fatalf("pump err = %v, want %v", err, want)
	}
}

// stallWriter accepts no bytes and reports no error — the degenerate
// state pump must refuse to loop on.
type stallWriter struct{}

func (stallWriter) Write(p []byte) (int, error) { return 0, nil }

type errReader struct{ err error }

func (e errReader) Read(p []byte) (int, error) { return 0, e.err }

// fakeRenameClient scripts the three-method renameClient surface so
// remoteReplace's fallback ladder (PosixRename → Rename → Remove(dest) →
// Rename) can be driven through each branch. Every call the code makes is
// recorded so tests can assert exactly what ran — in particular that a
// failed replace never deletes the freshly-transferred tmp file.
type fakeRenameClient struct {
	posixRename func(old, new string) error
	rename      func(old, new string) error
	remove      func(path string) error

	renameCalls int
	removeCalls []string
}

func (f *fakeRenameClient) PosixRename(old, new string) error {
	if f.posixRename != nil {
		return f.posixRename(old, new)
	}
	return nil
}

func (f *fakeRenameClient) Rename(old, new string) error {
	f.renameCalls++
	if f.rename != nil {
		return f.rename(old, new)
	}
	return nil
}

func (f *fakeRenameClient) Remove(path string) error {
	f.removeCalls = append(f.removeCalls, path)
	if f.remove != nil {
		return f.remove(path)
	}
	return nil
}

const (
	testTmp  = "/data/file.txt.part-fvmux"
	testDest = "/data/file.txt"
)

// (a) PosixRename succeeds → the single-step path; no Rename/Remove.
func TestRemoteReplace_PosixRenameSucceeds(t *testing.T) {
	f := &fakeRenameClient{posixRename: func(_, _ string) error { return nil }}
	if err := remoteReplace(f, testTmp, testDest); err != nil {
		t.Fatalf("remoteReplace = %v; want nil", err)
	}
	if f.renameCalls != 0 {
		t.Errorf("Rename called %d times; want 0", f.renameCalls)
	}
	if len(f.removeCalls) != 0 {
		t.Errorf("Remove called %v; want none", f.removeCalls)
	}
}

// (b) PosixRename fails but the first plain Rename succeeds (dest didn't
// exist) → dest is never Removed.
func TestRemoteReplace_FirstRenameSucceeds(t *testing.T) {
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
		rename:      func(_, _ string) error { return nil },
	}
	if err := remoteReplace(f, testTmp, testDest); err != nil {
		t.Fatalf("remoteReplace = %v; want nil", err)
	}
	if f.renameCalls != 1 {
		t.Errorf("Rename called %d times; want 1", f.renameCalls)
	}
	if len(f.removeCalls) != 0 {
		t.Errorf("dest must not be Removed when the first rename works; got %v", f.removeCalls)
	}
}

// (c) PosixRename fails, first Rename fails, Remove(dest) runs, second
// Rename succeeds → ok; exactly one Remove and its argument is dest, not
// tmp.
func TestRemoteReplace_RemoveDestThenRenameSucceeds(t *testing.T) {
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
	}
	f.rename = func(_, _ string) error {
		if f.renameCalls == 1 { // first invocation fails (dest exists)
			return errors.New("dest exists")
		}
		return nil // retry after Remove(dest) succeeds
	}
	if err := remoteReplace(f, testTmp, testDest); err != nil {
		t.Fatalf("remoteReplace = %v; want nil", err)
	}
	if f.renameCalls != 2 {
		t.Errorf("Rename called %d times; want 2", f.renameCalls)
	}
	if len(f.removeCalls) != 1 {
		t.Fatalf("Remove called %d times; want exactly 1 (%v)", len(f.removeCalls), f.removeCalls)
	}
	if f.removeCalls[0] != testDest {
		t.Errorf("Remove(%q); want Remove(%q) — must remove dest, never tmp", f.removeCalls[0], testDest)
	}
}

// (d) PosixRename fails and both Renames fail → error mentions the tmp
// path, and Remove was called exactly once with dest. The regression:
// the tmp file (the only surviving copy of the data) must NOT be removed.
func TestRemoteReplace_BothRenamesFailPreservesTmp(t *testing.T) {
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
		rename:      func(_, _ string) error { return errors.New("rename failed") },
	}
	err := remoteReplace(f, testTmp, testDest)
	if err == nil {
		t.Fatal("remoteReplace = nil; want error when both renames fail")
	}
	if !strings.Contains(err.Error(), testTmp) {
		t.Errorf("error %q does not mention preserved tmp path %q", err.Error(), testTmp)
	}
	if f.renameCalls != 2 {
		t.Errorf("Rename called %d times; want 2", f.renameCalls)
	}
	if len(f.removeCalls) != 1 || f.removeCalls[0] != testDest {
		t.Errorf("Remove calls = %v; want exactly one Remove(%q) and tmp left intact", f.removeCalls, testDest)
	}
}

func TestSyncWidget_ClampsHugeSizesWithoutOverflow(t *testing.T) {
	m := NewManager("a")
	tr := &Transfer{
		Direction:  Download,
		RemotePath: "/big.iso",
		Size:       int64(math.MaxInt32) * 4, // ~8 GiB
		StartedAt:  time.Now(),
		cancel:     make(chan struct{}),
	}
	tr.bytes.Store(int64(math.MaxInt32) * 2) // ~halfway
	m.mu.Lock()
	m.list = append(m.list, tr)
	m.mu.Unlock()

	tp := taskprogress.New(geom.NewRect(0, 0, 20, 1))
	m.SyncWidget(tp)
	if len(tp.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tp.Tasks))
	}
	task := tp.Tasks[0]
	if task.Max < 1 || task.Max > math.MaxInt32 {
		t.Fatalf("Max out of range: %d", task.Max)
	}
	if task.Value < 0 || task.Value > task.Max {
		t.Fatalf("Value %d out of [0,Max=%d]", task.Value, task.Max)
	}
	// Progress ratio should survive the down-scaling (≈ 50%).
	ratio := float64(task.Value) / float64(task.Max)
	if ratio < 0.4 || ratio > 0.6 {
		t.Fatalf("progress ratio %.2f not ≈ 0.5", ratio)
	}
}
