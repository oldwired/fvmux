package sftp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStartTreeRejectsDuplicateRootUntilWholeTreeFinishes(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	writeTree(t, srcRoot, map[string]string{"one.txt": "one"})
	dstRoot := filepath.Join(base, "dst")

	m := NewManager("test")
	m.SetParallel(1)
	m.sem <- struct{}{} // keep the first tree's file transfer active.
	released := false
	defer func() {
		if !released {
			<-m.sem
		}
		m.CancelAll()
		m.Wait()
	}()

	if n, err := m.StartTree(c, Upload, srcRoot, dstRoot); err != nil || n != 1 {
		t.Fatalf("first StartTree = (%d, %v), want (1, nil)", n, err)
	}
	if _, err := m.StartTree(c, Upload, srcRoot, dstRoot); !errors.Is(err, ErrDestinationBusy) {
		t.Fatalf("duplicate StartTree error = %v, want ErrDestinationBusy", err)
	}

	<-m.sem
	released = true
	m.Wait()
}

// writeTree materialises files (rel-path → contents) under root, creating
// parent directories as needed.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// readTree returns every regular file under root as rel-path → contents.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStartTree_UploadNested(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	files := map[string]string{
		"top.txt":        "top",
		"sub/a.txt":      "alpha",
		"sub/b.txt":      "bravo",
		"sub/deep/c.txt": "charlie",
		"sub/deep/d.bin": "delta-delta",
	}
	writeTree(t, srcRoot, files)

	dstRoot := filepath.Join(base, "dst")
	m := NewManager("test")
	n, err := m.StartTree(c, Upload, srcRoot, dstRoot)
	if err != nil {
		t.Fatalf("StartTree: %v", err)
	}
	if n != len(files) {
		t.Fatalf("enqueued %d files, want %d", n, len(files))
	}
	m.Wait()

	for _, tr := range m.Snapshot() {
		if tr.Status() != StatusDone {
			t.Fatalf("transfer %s not Done: status=%d err=%q", tr.RemotePath, tr.Status(), tr.Error())
		}
	}
	if got := readTree(t, dstRoot); !reflect.DeepEqual(got, files) {
		t.Fatalf("uploaded tree mismatch:\n got=%v\nwant=%v", got, files)
	}
}

func TestStartTree_DownloadNested(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "remote-src")
	files := map[string]string{
		"x.txt":       "ex",
		"d1/y.txt":    "why",
		"d1/d2/z.txt": "zee",
	}
	writeTree(t, srcRoot, files)

	dstRoot := filepath.Join(base, "local-dst")
	m := NewManager("test")
	n, err := m.StartTree(c, Download, dstRoot, srcRoot)
	if err != nil {
		t.Fatalf("StartTree: %v", err)
	}
	if n != len(files) {
		t.Fatalf("enqueued %d files, want %d", n, len(files))
	}
	m.Wait()

	if got := readTree(t, dstRoot); !reflect.DeepEqual(got, files) {
		t.Fatalf("downloaded tree mismatch:\n got=%v\nwant=%v", got, files)
	}
}

func TestStartTree_EmptyDirsPreserved(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	// Two empty subdirs, no files anywhere.
	for _, d := range []string{"empty1", "nested/empty2"} {
		if err := os.MkdirAll(filepath.Join(srcRoot, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dstRoot := filepath.Join(base, "dst")
	m := NewManager("test")
	n, err := m.StartTree(c, Upload, srcRoot, dstRoot)
	if err != nil {
		t.Fatalf("StartTree: %v", err)
	}
	if n != 0 {
		t.Fatalf("enqueued %d files, want 0 for an all-empty tree", n)
	}
	for _, d := range []string{"empty1", "nested/empty2"} {
		fi, err := os.Stat(filepath.Join(dstRoot, filepath.FromSlash(d)))
		if err != nil || !fi.IsDir() {
			t.Fatalf("empty dir %s not recreated: err=%v", d, err)
		}
	}
}

func TestStartTree_MergeOverwrites(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	writeTree(t, srcRoot, map[string]string{
		"shared.txt": "new",
		"fresh.txt":  "added",
	})

	// Destination already has shared.txt (stale) plus an untouched file.
	dstRoot := filepath.Join(base, "dst")
	writeTree(t, dstRoot, map[string]string{
		"shared.txt":    "stale",
		"untouched.txt": "keep me",
	})

	m := NewManager("test")
	if _, err := m.StartTree(c, Upload, srcRoot, dstRoot); err != nil {
		t.Fatalf("StartTree: %v", err)
	}
	m.Wait()

	want := map[string]string{
		"shared.txt":    "new",     // overwritten
		"fresh.txt":     "added",   // added
		"untouched.txt": "keep me", // preserved
	}
	if got := readTree(t, dstRoot); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged tree mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestStartTreeDownloadRejectsBackslashRemoteName(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	unsafeName := `..\..\escaped.txt`
	localRoot := filepath.Join(base, "local")
	m := NewManager("test")
	// A real Windows filesystem cannot represent a remote POSIX entry whose
	// name contains backslashes: it interprets them as separators first. Feed
	// the hostile server response through the injectable directory boundary so
	// the integration test exercises the same sanitizer on every host OS.
	m.readRemoteDirectory = func(context.Context, string) (remoteDirectory, error) {
		return sanitizeRemoteDirectory([]os.FileInfo{remoteInfo{name: unsafeName}}, false), nil
	}
	n, err := m.StartTree(c, Download, localRoot, "/remote")
	if !errors.Is(err, ErrUnsafeRemoteName) {
		t.Fatalf("StartTree error = %v, want ErrUnsafeRemoteName", err)
	}
	if n != 0 {
		t.Fatalf("StartTree enqueued %d files, want 0", n)
	}
	m.Wait()
	if got := len(m.Snapshot()); got != 0 {
		t.Fatalf("manager retained %d transfers, want 0", got)
	}
}

func TestStartTreeDownloadRejectsDestinationDirectorySymlink(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	remoteRoot := filepath.Join(base, "remote")
	writeTree(t, remoteRoot, map[string]string{"linked/escape.txt": "attacker"})

	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	localRoot := filepath.Join(base, "local")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(localRoot, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	m := NewManager("test")
	if _, err := m.StartTree(c, Download, localRoot, remoteRoot); err == nil {
		t.Fatal("StartTree succeeded through a destination-directory symlink")
	}
	m.Wait()
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file stat = %v, want not-exist", err)
	}
}

func TestStartTreeDownloadRejectsRelativeSymlinkOutsideSelectedTree(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	remoteRoot := filepath.Join(base, "remote")
	writeTree(t, remoteRoot, map[string]string{"linked/escape.txt": "attacker"})

	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	localRoot := filepath.Join(base, "local")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(localRoot, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	m := NewManager("test")
	if _, err := m.StartTree(c, Download, localRoot, remoteRoot); err == nil {
		t.Fatal("StartTree followed a relative link outside the selected destination")
	}
	m.Wait()
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file stat = %v, want not-exist", err)
	}
}

func TestStartTreeUploadRejectsFileSymlink(t *testing.T) {
	c := newTestClient(t)
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	if err := os.MkdirAll(srcRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(base, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(srcRoot, "linked.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	remoteRoot := filepath.Join(base, "remote")
	m := NewManager("test")
	n, err := m.StartTree(c, Upload, srcRoot, remoteRoot)
	if !errors.Is(err, ErrUnsafeLocalLink) {
		t.Fatalf("StartTree error = %v, want ErrUnsafeLocalLink", err)
	}
	if n != 0 {
		t.Fatalf("StartTree enqueued %d files, want 0", n)
	}
	m.Wait()
	if _, err := os.Stat(filepath.Join(remoteRoot, "linked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remote linked file stat = %v, want not-exist", err)
	}
}

func TestStartTreeUsesAndReapsOperationOwnedConnection(t *testing.T) {
	shared := newTestClient(t)
	dedicated := newTestClient(t)
	owned := &fakeConn{client: dedicated, closedCh: make(chan struct{})}
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	writeTree(t, srcRoot, map[string]string{"file.txt": "payload"})
	remoteRoot := filepath.Join(base, "remote")

	m := NewManager("test")
	m.openDedicated = func() (dedicatedConn, error) { return owned, nil }
	if _, err := m.StartTree(shared, Upload, srcRoot, remoteRoot); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if got, err := os.ReadFile(filepath.Join(remoteRoot, "file.txt")); err != nil || string(got) != "payload" {
		t.Fatalf("operation-owned transfer result = %q, %v", got, err)
	}
	select {
	case <-owned.closedCh:
	case <-time.After(time.Second):
		t.Fatal("operation-owned SFTP connection was not reaped")
	}
}
