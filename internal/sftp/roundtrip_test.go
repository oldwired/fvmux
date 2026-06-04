package sftp

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// These exercise Manager.copy / remoteReplace end to end against a real
// pkg/sftp server (in-process). "Remote" paths are just temp-dir paths
// the server happens to serve.

func TestStart_UploadRoundTrip(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()

	src := filepath.Join(dir, "src.bin")
	want := bytes.Repeat([]byte("fvmux"), 50_000) // > one 64 KiB chunk
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "uploaded.bin")

	m := NewManager("test")
	tr, err := m.Start(c, Upload, src, dst)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusDone {
		t.Fatalf("status = %d (err=%q), want Done", s, tr.Error())
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read uploaded: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("uploaded bytes differ from source")
	}
	// The .part temp must not survive a successful transfer.
	if _, err := os.Stat(dst + partSuffix); !os.IsNotExist(err) {
		t.Fatalf("leftover part file: err=%v", err)
	}
}

func TestStart_DownloadRoundTrip(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()

	src := filepath.Join(dir, "remote.bin")
	want := bytes.Repeat([]byte("xyz"), 40_000)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "downloaded.bin")

	m := NewManager("test")
	tr, err := m.Start(c, Download, dst, src)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusDone {
		t.Fatalf("status = %d (err=%q), want Done", s, tr.Error())
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read downloaded: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("downloaded bytes differ from source")
	}
}

func TestStart_UploadOverwriteAtomic(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()

	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("new contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(dst, []byte("old contents that is longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager("test")
	tr, err := m.Start(c, Upload, src, dst)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s := waitTransfer(t, tr); s != StatusDone {
		t.Fatalf("status = %d (err=%q), want Done", s, tr.Error())
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new contents" {
		t.Fatalf("overwrite gave %q, want %q", got, "new contents")
	}
}

func TestStart_UploadMissingSourceFails(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	m := NewManager("test")
	if _, err := m.Start(c, Upload, filepath.Join(dir, "nope"), filepath.Join(dir, "dst")); err == nil {
		t.Fatal("Start should fail when the source file is missing")
	}
}
