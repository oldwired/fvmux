package logs

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// resetForTest restores package-level state so each test starts from a
// clean slate. Init uses sync.Once which we can't re-arm, so the tests
// poke the unexported fields directly.
func resetForTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	ring = nil
	ringHead = 0
	ringCap = 0
	sink = nil
	sinkClose = nil
	once = sync.Once{}
	initErr = nil
}

func TestRing_OverflowKeepsLatestInOrder(t *testing.T) {
	resetForTest(t)
	// Init clamps below 16; ringSize=20 keeps the math simple.
	if err := Init("", 20); err != nil {
		t.Fatal(err)
	}
	// Init logged one entry; push enough to wrap past cap.
	for i := 0; i < 25; i++ {
		slog.Info("m", "i", i)
	}
	got := Entries()
	if len(got) != 20 {
		t.Fatalf("entries: got %d want 20", len(got))
	}
	// After overflow, the oldest surviving record should be index 6
	// (init + 25 messages = 26 entries; cap 20 keeps the last 20).
	// Verify chronological ordering by checking timestamps.
	for i := 1; i < len(got); i++ {
		if got[i].Time.Before(got[i-1].Time) {
			t.Fatalf("entries out of order at %d", i)
		}
	}
}

func TestSink_WritesEveryRecord(t *testing.T) {
	resetForTest(t)
	var buf bytes.Buffer
	if err := Init("", 16); err != nil {
		t.Fatal(err)
	}
	// Swap the sink to a buffer the test can read.
	mu.Lock()
	sink = &buf
	sinkClose = func() error { return nil }
	mu.Unlock()

	slog.Info("alpha", "k", "v")
	slog.Warn("beta")

	out := buf.String()
	if !strings.Contains(out, "alpha k=v") {
		t.Errorf("expected 'alpha k=v' in sink output, got:\n%s", out)
	}
	if !strings.Contains(out, "WARN") || !strings.Contains(out, "beta") {
		t.Errorf("expected WARN beta in sink output, got:\n%s", out)
	}
}

func TestWithAttrs_ReachesSink(t *testing.T) {
	resetForTest(t)
	var buf bytes.Buffer
	if err := Init("", 16); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	sink = &buf
	sinkClose = func() error { return nil }
	mu.Unlock()

	logger := slog.Default().With("request_id", "abc123")
	logger.Info("handled")

	out := buf.String()
	if !strings.Contains(out, "request_id=abc123") {
		t.Fatalf("WithAttrs attr lost; sink output:\n%s", out)
	}
	if !strings.Contains(out, "handled") {
		t.Fatalf("message body missing; sink output:\n%s", out)
	}
}

func TestClose_Idempotent(t *testing.T) {
	resetForTest(t)
	if err := Init("", 16); err != nil {
		t.Fatal(err)
	}
	closes := 0
	mu.Lock()
	sinkClose = func() error { closes++; return nil }
	sink = &bytes.Buffer{}
	mu.Unlock()

	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if closes != 1 {
		t.Fatalf("expected sinkClose called once, got %d", closes)
	}
}

func TestClose_RingStillReadable(t *testing.T) {
	resetForTest(t)
	if err := Init("", 8); err != nil {
		t.Fatal(err)
	}
	slog.Info("survives-close")
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	entries := Entries()
	found := false
	for _, e := range entries {
		if e.Msg == "survives-close" {
			found = true
		}
	}
	if !found {
		t.Fatal("Close() should not wipe the ring buffer")
	}
}
