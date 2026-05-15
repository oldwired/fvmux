package app

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduleEgg_FireRemovesFromList(t *testing.T) {
	m := &Mux{}
	var fired atomic.Int32
	m.scheduleEgg(5*time.Millisecond, func() { fired.Add(1) })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if fired.Load() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if fired.Load() != 1 {
		t.Fatal("timer never fired")
	}
	// Give the timer's bookkeeping closure a moment to remove itself.
	deadline = time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		m.eggMu.Lock()
		n := len(m.eggTimers)
		m.eggMu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("fired timer was not removed from eggTimers")
}

func TestStopEggs_CancelsPendingTimers(t *testing.T) {
	m := &Mux{}
	var fired atomic.Int32

	// Schedule a few long timers — far longer than the test runtime.
	for i := 0; i < 3; i++ {
		m.scheduleEgg(10*time.Second, func() { fired.Add(1) })
	}

	m.eggMu.Lock()
	if got := len(m.eggTimers); got != 3 {
		m.eggMu.Unlock()
		t.Fatalf("expected 3 pending timers, got %d", got)
	}
	m.eggMu.Unlock()

	m.StopEggs()

	// Wait long enough that any un-stopped timer would have fired.
	time.Sleep(50 * time.Millisecond)
	if fired.Load() != 0 {
		t.Fatalf("expected 0 fires after StopEggs, got %d", fired.Load())
	}

	m.eggMu.Lock()
	if got := len(m.eggTimers); got != 0 {
		m.eggMu.Unlock()
		t.Fatalf("expected eggTimers cleared, got %d", got)
	}
	m.eggMu.Unlock()
}

func TestStopEggs_Idempotent(t *testing.T) {
	m := &Mux{}
	m.StopEggs()
	m.StopEggs()
	m.StopEggs()
}
