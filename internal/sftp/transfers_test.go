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
// remoteReplace's fallback ladder (PosixRename → Rename → move-dest-
// aside → Rename → reap/restore backup) can be driven through each
// branch. Every call the code makes is recorded so tests can assert
// exactly what ran — in particular that a failed replace never deletes
// the freshly-transferred tmp file NOR the original destination.
type fakeRenameClient struct {
	posixRename func(old, new string) error
	rename      func(old, new string) error
	remove      func(path string) error

	renameCalls int
	renameArgs  [][2]string
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
	f.renameArgs = append(f.renameArgs, [2]string{old, new})
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

const testBackup = testDest + ".replaced-fvmux"

// (c) PosixRename fails, first Rename fails (dest exists) → dest is
// moved ASIDE (never Removed), tmp lands at dest, the backup is reaped.
// The only Remove calls ever allowed are the stale-backup sweep and the
// final backup reap — dest and tmp themselves must never be removed.
func TestRemoteReplace_BacksUpDestThenRenameSucceeds(t *testing.T) {
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
	}
	f.rename = func(_, _ string) error {
		if f.renameCalls == 1 { // tmp→dest fails (dest exists)
			return errors.New("dest exists")
		}
		return nil // dest→backup and retry tmp→dest succeed
	}
	if err := remoteReplace(f, testTmp, testDest); err != nil {
		t.Fatalf("remoteReplace = %v; want nil", err)
	}
	wantRenames := [][2]string{
		{testTmp, testDest},    // optimistic rename
		{testDest, testBackup}, // move original aside
		{testTmp, testDest},    // land the copy
	}
	if len(f.renameArgs) != 3 {
		t.Fatalf("renames = %v; want %v", f.renameArgs, wantRenames)
	}
	for i, want := range wantRenames {
		if f.renameArgs[i] != want {
			t.Errorf("rename[%d] = %v; want %v", i, f.renameArgs[i], want)
		}
	}
	// Exactly one Remove: the reap of the backup THIS call created. No
	// preemptive sweep — a pre-existing file at the backup path must
	// never be deleted.
	if len(f.removeCalls) != 1 || f.removeCalls[0] != testBackup {
		t.Errorf("Remove calls = %v; want exactly the backup reap %q", f.removeCalls, testBackup)
	}
}

// (c2) The first backup candidate is occupied — say by a preserved
// original from a previously failed restore, or an unrelated user
// file. It must NOT be removed; moveAside steps to the next candidate
// and the reap targets only the backup this call created.
func TestRemoteReplace_OccupiedBackupNeverClobbered(t *testing.T) {
	occupied := testBackup // pre-existing file at the first candidate
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
	}
	f.rename = func(old, new string) error {
		switch {
		case old == testTmp && f.renameCalls == 1:
			return errors.New("dest exists") // optimistic rename fails
		case new == occupied:
			return errors.New("target exists") // candidate 0 is taken
		default:
			return nil
		}
	}
	if err := remoteReplace(f, testTmp, testDest); err != nil {
		t.Fatalf("remoteReplace = %v; want nil", err)
	}
	wantBackup := testDest + ".replaced-fvmux.1"
	for _, rm := range f.removeCalls {
		if rm == occupied || rm == testDest || rm == testTmp {
			t.Errorf("Remove(%q) — occupied backup/dest/tmp must never be removed", rm)
		}
	}
	if len(f.removeCalls) != 1 || f.removeCalls[0] != wantBackup {
		t.Errorf("Remove calls = %v; want exactly the reap of %q", f.removeCalls, wantBackup)
	}
}

// (d) PosixRename fails and the optimistic Rename fails for ANY reason
// (could be transient, permission, or dest-exists — SFTP can't tell):
// the original must never be deleted. When even the move-aside rename
// fails, NOTHING has been touched: no Remove of dest or tmp, and the
// error names the preserved tmp path.
func TestRemoteReplace_RenameFailurePreservesDestAndTmp(t *testing.T) {
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
		rename:      func(_, _ string) error { return errors.New("connection reset") },
	}
	err := remoteReplace(f, testTmp, testDest)
	if err == nil {
		t.Fatal("remoteReplace = nil; want error when renames fail")
	}
	if !strings.Contains(err.Error(), testTmp) {
		t.Errorf("error %q does not mention preserved tmp path %q", err.Error(), testTmp)
	}
	// Nothing may be removed on a total failure — not dest, not tmp,
	// and no preemptive backup sweep either.
	if len(f.removeCalls) != 0 {
		t.Errorf("Remove calls = %v; want none on total failure", f.removeCalls)
	}
}

// (e) The move-aside succeeds but the final rename fails → the original
// is restored from the backup and the error still names tmp.
func TestRemoteReplace_FinalRenameFailureRestoresDest(t *testing.T) {
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
	}
	f.rename = func(old, new string) error {
		if old == testTmp { // both tmp→dest attempts fail
			return errors.New("write denied")
		}
		return nil // dest→backup and backup→dest succeed
	}
	err := remoteReplace(f, testTmp, testDest)
	if err == nil {
		t.Fatal("remoteReplace = nil; want error")
	}
	if !strings.Contains(err.Error(), "restored") || !strings.Contains(err.Error(), testTmp) {
		t.Errorf("error %q should report the restored original and the preserved tmp", err.Error())
	}
	last := f.renameArgs[len(f.renameArgs)-1]
	if last != [2]string{testBackup, testDest} {
		t.Errorf("last rename = %v; want backup restored to dest", last)
	}
	for _, rm := range f.removeCalls {
		if rm == testDest || rm == testTmp {
			t.Errorf("Remove(%q) — dest/tmp must never be removed", rm)
		}
	}
}

// (f) Worst case: move-aside succeeded, final rename fails, restore
// fails too → the error must tell the user where BOTH copies live.
func TestRemoteReplace_RestoreFailureNamesBothCopies(t *testing.T) {
	f := &fakeRenameClient{
		posixRename: func(_, _ string) error { return errors.New("no posix-rename ext") },
	}
	f.rename = func(old, new string) error {
		if old == testDest {
			return nil // move-aside succeeds
		}
		return errors.New("session gone") // everything after fails
	}
	err := remoteReplace(f, testTmp, testDest)
	if err == nil {
		t.Fatal("remoteReplace = nil; want error")
	}
	if !strings.Contains(err.Error(), testBackup) || !strings.Contains(err.Error(), testTmp) {
		t.Errorf("error %q should name both the backup and the tmp copy", err.Error())
	}
	for _, rm := range f.removeCalls {
		if rm == testDest || rm == testTmp {
			t.Errorf("Remove(%q) — dest/tmp must never be removed", rm)
		}
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
