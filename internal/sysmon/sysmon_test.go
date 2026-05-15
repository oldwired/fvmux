package sysmon

import (
	"sync"
	"testing"
	"time"
)

func resetForTest(t *testing.T) {
	t.Helper()
	// If a previous test left a sampler running, drain it first.
	Stop()
	mu.Lock()
	defer mu.Unlock()
	cpuRing = [historySize]float64{}
	cpuHead = 0
	cpuLen = 0
	ramPct = 0
	stopCh = nil
	doneCh = nil
}

func TestPush_RingFillThenWrap(t *testing.T) {
	resetForTest(t)
	for i := 0; i < historySize; i++ {
		push(float64(i) / 10.0)
	}
	if cpuLen != historySize {
		t.Fatalf("cpuLen after fill: got %d want %d", cpuLen, historySize)
	}
	hist := CPUHistory()
	if len(hist) != historySize {
		t.Fatalf("history len: got %d want %d", len(hist), historySize)
	}
	for i, v := range hist {
		want := float64(i) / 10.0
		if v != want {
			t.Errorf("history[%d]: got %g want %g", i, v, want)
		}
	}

	// Push three more to wrap.
	push(1.0)
	push(0.95)
	push(0.9)
	hist = CPUHistory()
	if len(hist) != historySize {
		t.Fatalf("history len after wrap: got %d", len(hist))
	}
	// The first three samples should have been evicted; oldest now is
	// the sample that was at index 3 originally (= 0.3).
	if hist[0] != 0.3 {
		t.Errorf("after wrap, oldest = %g; want 0.3", hist[0])
	}
	if hist[len(hist)-1] != 0.9 {
		t.Errorf("after wrap, newest = %g; want 0.9", hist[len(hist)-1])
	}
}

func TestPush_ClampsOutOfRange(t *testing.T) {
	resetForTest(t)
	push(-0.5)
	push(2.0)
	hist := CPUHistory()
	if hist[0] != 0 {
		t.Errorf("negative input not clamped to 0; got %g", hist[0])
	}
	if hist[1] != 1 {
		t.Errorf("input >1 not clamped to 1; got %g", hist[1])
	}
}

func TestCPUHistory_EmptyOnColdStart(t *testing.T) {
	resetForTest(t)
	hist := CPUHistory()
	if len(hist) != 0 {
		t.Fatalf("cold-start history should be empty; got %v", hist)
	}
}

func TestStartStop_Idempotent(t *testing.T) {
	resetForTest(t)
	Start(10 * time.Millisecond)
	Start(10 * time.Millisecond) // second Start is a no-op
	time.Sleep(40 * time.Millisecond)
	Stop()
	Stop() // second Stop is a no-op
	// After Stop, no further samples should land. Capture current
	// length, sleep, ensure unchanged.
	mu.Lock()
	lenAfter := cpuLen
	mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if cpuLen != lenAfter {
		mu.Unlock()
		t.Fatalf("sampler still running after Stop: %d -> %d", lenAfter, cpuLen)
	}
	mu.Unlock()
}

func TestCPUHistory_ConcurrentReads(t *testing.T) {
	resetForTest(t)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				push(0.5)
			}
		}
	}()

	for i := 0; i < 200; i++ {
		_ = CPUHistory()
		_ = RAMUsage()
	}
	close(stop)
	wg.Wait()
}
