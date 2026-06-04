package sftp

import (
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"
)

// newTestTicker builds a transferTicker over m with no side panels, so
// Active→Done refreshes are nil-guarded no-ops. Mirrors the browser's
// construction minus the live panels.
func newTestTicker(m *Manager) *transferTicker {
	return &transferTicker{
		m:    m,
		tp:   taskprogress.New(geom.NewRect(0, 0, 10, 2)),
		seen: make(map[*Transfer]int32),
		ok:   true,
	}
}

func addTransfer(m *Manager, tr *Transfer) {
	m.mu.Lock()
	m.list = append(m.list, tr)
	m.mu.Unlock()
}

// TestTransferTicker_IdleReturnsFalse is the core regression for the
// SIXEL-preview flicker: with no transfers in flight, the 200 ms tick
// must NOT request a repaint. A true return forces a full flush, which
// re-emits any on-screen image preview and makes it strobe.
func TestTransferTicker_IdleReturnsFalse(t *testing.T) {
	tt := newTestTicker(NewManager("test"))
	if tt.Tick(time.Now()) {
		t.Fatal("Tick with no transfers returned true; idle ticks must not force a repaint")
	}
	// Idempotent: a second idle tick is still quiescent.
	if tt.Tick(time.Now()) {
		t.Fatal("second idle Tick returned true")
	}
}

// TestTransferTicker_ActiveRequestsRepaint: a running transfer needs the
// progress bar animated, so each tick requests a repaint.
func TestTransferTicker_ActiveRequestsRepaint(t *testing.T) {
	m := NewManager("test")
	addTransfer(m, newActiveTransfer())
	tt := newTestTicker(m)

	if !tt.Tick(time.Now()) {
		t.Fatal("first Tick observing an active transfer should return true")
	}
	if !tt.Tick(time.Now()) {
		t.Fatal("Tick while a transfer is still active should keep returning true")
	}
}

// TestTransferTicker_SettledStopsRepainting: once every transfer reaches
// a terminal state, the transition fires exactly one more repaint (to
// paint the final bar + refresh the destination panel), then the ticker
// goes quiet. This is what lets a still image preview stay still after a
// transfer finishes.
func TestTransferTicker_SettledStopsRepainting(t *testing.T) {
	m := NewManager("test")
	tr := newActiveTransfer()
	addTransfer(m, tr)
	tt := newTestTicker(m)

	if !tt.Tick(time.Now()) {
		t.Fatal("observing the active transfer should return true")
	}
	tr.status.Store(StatusDone)
	if !tt.Tick(time.Now()) {
		t.Fatal("the Active→Done transition should request one repaint")
	}
	if tt.Tick(time.Now()) {
		t.Fatal("settled transfer should stop forcing repaints (this was the flicker bug)")
	}
	if tt.Tick(time.Now()) {
		t.Fatal("settled transfer should keep returning false")
	}
}

// TestTransferTicker_ClearedEntryRepaintsOnce: dropping completed
// transfers (ClearCompleted) shrinks the snapshot below the seen-map
// size; the ticker must repaint once to drop the stale rows, then quiet.
func TestTransferTicker_ClearedEntryRepaintsOnce(t *testing.T) {
	m := NewManager("test")
	tr := newActiveTransfer()
	tr.status.Store(StatusDone)
	addTransfer(m, tr)
	tt := newTestTicker(m)

	if !tt.Tick(time.Now()) {
		t.Fatal("first observation of a transfer should return true")
	}
	if tt.Tick(time.Now()) {
		t.Fatal("already-settled transfer should be quiet on the next tick")
	}
	m.ClearCompleted()
	if !tt.Tick(time.Now()) {
		t.Fatal("clearing a transfer should request one repaint to drop the row")
	}
	if tt.Tick(time.Now()) {
		t.Fatal("after the cleared row is dropped, ticks should be quiet again")
	}
}
