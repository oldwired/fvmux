// Package sysmon polls CPU + RAM usage on a 1s cadence and exposes
// the readings via thread-safe getters for the status bar's sparkline
// + bar sections.
//
// One global Monitor is started by Start() at process boot; the 1s
// ticker calls Tick() once per interval. Tick is cheap and idempotent
// — safe to call on a goroutine. Readings stay at zero until the
// first successful sample.
package sysmon

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/oldwired/fvmux/internal/ring"
)

const historySize = 10

var (
	mu      sync.Mutex
	cpuRing = ring.New[float64](historySize)
	ramPct  float64
	stopCh  chan struct{}
	doneCh  chan struct{} // closed by the sampler goroutine on exit
)

// Start spins up a goroutine that samples CPU + RAM every interval.
// Idempotent — multiple calls are no-ops after the first.
func Start(interval time.Duration) {
	mu.Lock()
	if stopCh != nil {
		mu.Unlock()
		return
	}
	stopCh = make(chan struct{})
	done := make(chan struct{})
	doneCh = done
	// Capture local references; Stop will later nil the package vars
	// without disrupting the goroutine's view of the close signal.
	local := stopCh
	mu.Unlock()

	// Prime cpu.Percent — first call returns zeros, second gives a
	// delta against the first. Discard the first reading.
	_, _ = cpu.Percent(0, false)

	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				sample()
			case <-local:
				return
			}
		}
	}()
}

// Stop ends the sampler goroutine and waits for it to exit so the
// next test or process step doesn't see a lingering sample() call.
func Stop() {
	mu.Lock()
	if stopCh == nil {
		mu.Unlock()
		return
	}
	close(stopCh)
	stopCh = nil
	done := doneCh
	doneCh = nil
	mu.Unlock()
	if done != nil {
		<-done
	}
}

// CPUHistory returns the last N CPU samples (oldest first), each in
// [0.0, 1.0]. Length may be < historySize if the sampler is still
// warming up.
func CPUHistory() []float64 {
	mu.Lock()
	defer mu.Unlock()
	return cpuRing.Items()
}

// RAMUsage returns the current RAM utilisation in [0.0, 1.0].
func RAMUsage() float64 {
	mu.Lock()
	defer mu.Unlock()
	return ramPct
}

func sample() {
	percents, err := cpu.Percent(0, false)
	if err == nil && len(percents) > 0 {
		push(percents[0] / 100.0)
	}
	if v, err := mem.VirtualMemory(); err == nil && v.Total > 0 {
		mu.Lock()
		ramPct = float64(v.Used) / float64(v.Total)
		mu.Unlock()
	}
}

func push(v float64) {
	mu.Lock()
	defer mu.Unlock()
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	cpuRing.Push(v)
}
