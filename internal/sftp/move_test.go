package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStartMove_UploadDeletesSource(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "moved.txt")

	m := NewManager("test")
	tr, err := m.StartMove(c, Upload, src, dst, func() error { return os.Remove(src) })
	if err != nil {
		t.Fatalf("StartMove: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusDone {
		t.Fatalf("status=%d err=%q, want Done", s, tr.Error())
	}

	if got, err := os.ReadFile(dst); err != nil || string(got) != "payload" {
		t.Fatalf("dest = %q err=%v, want payload", got, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be gone after a move, stat err=%v", err)
	}
}

func TestStartMove_DeleteFailureFailsTransfer(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst.txt")

	boom := errors.New("cannot remove source")
	m := NewManager("test")
	tr, err := m.StartMove(c, Upload, src, dst, func() error { return boom })
	if err != nil {
		t.Fatalf("StartMove: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusFailed {
		t.Fatalf("status=%d, want Failed when source delete errors", s)
	}
	if tr.Error() != boom.Error() {
		t.Fatalf("error = %q, want %q", tr.Error(), boom.Error())
	}
	// The copy itself still landed — data is never lost on a move.
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("dest missing after copy+delete-fail: %v", err)
	}
}

func TestStartTreeMove_MovesAndDeletesSourceTree(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	files := map[string]string{
		"a.txt":       "aa",
		"sub/b.txt":   "bb",
		"sub/c/d.txt": "dd",
	}
	writeTree(t, srcRoot, files)

	dstRoot := filepath.Join(base, "dst")
	m := NewManager("test")
	n, err := m.StartTreeMove(c, Upload, srcRoot, dstRoot, func() error { return os.RemoveAll(srcRoot) })
	if err != nil {
		t.Fatalf("StartTreeMove: %v", err)
	}
	if n != len(files) {
		t.Fatalf("enqueued %d, want %d", n, len(files))
	}
	m.Wait() // drains transfers AND the finishTree deletion goroutine.

	if got := readTree(t, dstRoot); !reflect.DeepEqual(got, files) {
		t.Fatalf("moved tree mismatch:\n got=%v\nwant=%v", got, files)
	}
	if _, err := os.Stat(srcRoot); !os.IsNotExist(err) {
		t.Fatalf("source tree should be gone after move, stat err=%v", err)
	}
}

func TestStartTreeMove_FailedFileKeepsSource(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	writeTree(t, srcRoot, map[string]string{"ok.txt": "fine"})

	// Destination root path is pre-occupied by a regular file, so creating
	// the destination directory skeleton fails up front.
	dstRoot := filepath.Join(base, "dst")
	if err := os.WriteFile(dstRoot, []byte("I am a file, not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	called := false
	m := NewManager("test")
	_, err := m.StartTreeMove(c, Upload, srcRoot, dstRoot, func() error { called = true; return nil })
	if err == nil {
		t.Fatal("StartTreeMove should error when the dest skeleton can't be created")
	}
	m.Wait()
	if called {
		t.Fatal("source tree must not be deleted when the move never ran")
	}
	if _, err := os.Stat(srcRoot); err != nil {
		t.Fatalf("source tree should survive a failed move: %v", err)
	}
}
