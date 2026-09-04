package sftp

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestManager_ParallelLimit verifies SetParallel gates concurrent payload
// movement. With Parallel=1 and the single slot held by the test, all
// three enqueued transfers must sit queued — no bytes moved — instead of
// fanning onto the shared session at once (the #34 regression). Releasing
// the slot lets them drain one at a time and complete intact.
func TestManager_ParallelLimit(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()

	// Sizeable sources so a moving transfer would report Bytes() > 0.
	payload := bytes.Repeat([]byte("fvmux"), 100_000) // > one 64 KiB chunk
	var srcs, dsts []string
	for i := 0; i < 3; i++ {
		src := filepath.Join(dir, fmt.Sprintf("src%d.bin", i))
		if err := os.WriteFile(src, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		srcs = append(srcs, src)
		dsts = append(dsts, filepath.Join(dir, fmt.Sprintf("dst%d.bin", i)))
	}

	m := NewManager("test")
	m.SetParallel(1)
	// Occupy the single slot from the test so every run() blocks on the
	// acquire before moving a byte.
	m.sem <- struct{}{}

	var trs []*Transfer
	for i := 0; i < 3; i++ {
		tr, err := m.Start(c, Upload, srcs[i], dsts[i])
		if err != nil {
			t.Fatalf("Start[%d]: %v", i, err)
		}
		trs = append(trs, tr)
	}

	// Give the goroutines time to reach (and block on) the acquire. While
	// the slot is held, none may move data — a broken gate would let all
	// three run and report progress.
	time.Sleep(50 * time.Millisecond)
	for i, tr := range trs {
		if got := tr.Bytes(); got != 0 {
			t.Fatalf("transfer %d moved %d bytes while the slot was held; want 0 (semaphore not gating)", i, got)
		}
		if s := tr.Status(); s != StatusQueued {
			t.Fatalf("transfer %d status = %d while queued; want StatusQueued", i, s)
		}
	}

	// Release the slot; the three drain one at a time (cap 1) and complete.
	<-m.sem
	for i, tr := range trs {
		if s := waitTransfer(t, tr); s != StatusDone {
			t.Fatalf("transfer %d status = %d (err=%q); want Done", i, s, tr.Error())
		}
	}
	for i, dst := range dsts {
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read dst %d: %v", i, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("dst %d bytes differ from source", i)
		}
	}
}

// TestSetParallel_ClampsAndUnboundedDefault documents the two boundary
// behaviours SetParallel promises: n < 1 clamps to a usable 1-slot gate,
// and a manager that never calls SetParallel stays unbounded (nil sem).
func TestSetParallel_ClampsAndUnboundedDefault(t *testing.T) {
	if m := NewManager("x"); m.sem != nil {
		t.Fatal("fresh manager should have a nil (unbounded) semaphore")
	}
	m := NewManager("x")
	m.SetParallel(0)
	if cap(m.sem) != 1 {
		t.Fatalf("SetParallel(0) gave cap %d; want 1 (clamped)", cap(m.sem))
	}
	m.SetParallel(4)
	if cap(m.sem) != 4 {
		t.Fatalf("SetParallel(4) gave cap %d; want 4", cap(m.sem))
	}
}
