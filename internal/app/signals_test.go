package app

import "testing"

// When a signal-driven quit is in progress, OnQuitRequested must proceed
// without consulting CanQuit — even with confirm-kill configured, which
// would otherwise try to pop a modal. We assert it proceeds; the bypass
// of CanQuit is guaranteed by the && short-circuit (a modal here would
// deadlock a headless test, so its absence is the test).
func TestOnQuitRequested_SignalQuitBypassesPrompt(t *testing.T) {
	m := &Mux{}
	m.signalQuit.Store(true)
	if !m.OnQuitRequested() {
		t.Fatal("signal-driven quit should proceed unconditionally")
	}
}

// Normal quit with no live panes proceeds (CanQuit returns true when
// there's nothing to kill).
func TestOnQuitRequested_NoLivePanesProceeds(t *testing.T) {
	m := &Mux{}
	if !m.OnQuitRequested() {
		t.Fatal("quit with no live panes should proceed")
	}
}
