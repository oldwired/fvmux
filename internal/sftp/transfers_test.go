package sftp

import (
	"sync"
	"testing"
	"time"
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
	m := NewManager()
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
	m := NewManager()
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
	m := NewManager()
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
	m := NewManager()
	if m.CancelLast() {
		t.Fatal("CancelLast on empty manager should return false")
	}
}
