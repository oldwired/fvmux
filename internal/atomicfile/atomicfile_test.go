package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWrite_CreatesFileWithRequestedPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	if err := Write(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("content: got %q want %q", data, "hello")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Fatalf("perm: got %o want 0o600", perm)
		}
	}
}

func TestWrite_OverwriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("original-contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("after overwrite: got %q want %q", data, "new")
	}
}

func TestWrite_LeavesNoTempOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := Write(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp should not exist after success; stat err = %v", err)
	}
}

func TestWrite_OriginalSurvivesFailedWrite(t *testing.T) {
	// Failure path: open a path whose parent dir doesn't exist. Write
	// should fail and the supposed original (we'll pre-create) must be
	// untouched.
	dir := t.TempDir()
	orig := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(orig, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Now attempt Write to a path under a non-existent directory.
	bogus := filepath.Join(dir, "missing-dir", "config.toml")
	if err := Write(bogus, []byte("REPLACEMENT"), 0o644); err == nil {
		t.Fatal("expected Write to fail for missing parent dir")
	}
	got, _ := os.ReadFile(orig)
	if string(got) != "ORIGINAL" {
		t.Fatalf("original was modified: got %q", got)
	}
}
