package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
)

// newCatMux builds a headless Mux (Desktop-backed Application, default
// config with confirm-kill off) whose only profile — and whose default
// shell ($SHELL) — is `cat`. cat with no stdin blocks forever: it never
// exits and emits no output, so each pane's read/wait goroutines stay
// parked and never fire the wireTerminalCallbacks handlers nor call
// views.CallSoon. That is what keeps these real-PTY tests clean under
// -race alongside the CallSoon-capturing tests in this package.
//
// The panes are intentionally NOT stopped at test end (mirroring
// session_pick_test.go): stopping cat would unblock waitLoop, which then
// calls views.CallSoon and races another test's views.SetCallSoon on the
// process-global scheduler. The children die when the test binary exits.
func newCatMux(t *testing.T) *Mux {
	t.Helper()
	// The unknown-named-profile fallback in NewWindow resolves to
	// profile.Defaults()[0], which runs $SHELL — pin it to cat too so that
	// path spawns a blocking pane rather than a real interactive shell.
	t.Setenv("SHELL", "cat")
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	cfg := config.Defaults()
	cfg.General.ConfirmKill = false
	cfg.General.DefaultProfile = "shell"
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	m.Opts.Config = cfg
	m.Opts.Profiles = []*profile.Profile{{Name: "shell", Command: "cat"}}
	return m
}

// TestNextWindowNumber_LowestFreePositive is the regression for finding
// #23: nextWindowNumber returns the lowest positive number no live window
// holds (the old len(windowOrder)+1 scheme duplicated numbers after a
// middle window closed). windowNumberInUse tracks membership.
func TestNextWindowNumber_LowestFreePositive(t *testing.T) {
	m := &Mux{windows: map[views.View]*windowState{}}

	// Empty registry → 1.
	if got := m.nextWindowNumber(); got != 1 {
		t.Fatalf("empty Mux nextWindowNumber() = %d, want 1", got)
	}

	keys := map[int]views.View{}
	add := func(n int) {
		w := views.NewWindow(geom.NewRect(0, 0, 10, 5), "w", n)
		key := w.Self()
		keys[n] = key
		m.windows[key] = &windowState{Number: n}
	}
	add(1)
	add(2)
	add(3)

	if got := m.nextWindowNumber(); got != 4 {
		t.Fatalf("with 1,2,3 live, nextWindowNumber() = %d, want 4", got)
	}
	if !m.windowNumberInUse(2) {
		t.Error("windowNumberInUse(2) = false, want true")
	}

	// Drop #2: the freed gap is reused ahead of 4.
	delete(m.windows, keys[2])
	if m.windowNumberInUse(2) {
		t.Error("windowNumberInUse(2) = true after delete, want false")
	}
	if got := m.nextWindowNumber(); got != 2 {
		t.Fatalf("after deleting #2, nextWindowNumber() = %d, want 2 (lowest free)", got)
	}
}

// TestNewWindow_UnknownProfileWarnsAndFallsBack is the regression for
// finding #28: an unknown profile name (a typo'd -profile flag or stale
// default_profile) must not silently open a generic shell. NewWindow
// schedules a warning msgbox via views.CallSoon and falls back to the
// default shell. We capture CallSoon (and never run it — running would
// pop a modal) and assert the unknown-profile path queues at least one
// callback beyond whatever the valid path queues, while still returning a
// live window.
func TestNewWindow_UnknownProfileWarnsAndFallsBack(t *testing.T) {
	m := newCatMux(t)

	var scheduled []func()
	views.SetCallSoon(func(fn func()) { scheduled = append(scheduled, fn) })
	defer views.SetCallSoon(nil)

	// Baseline: a valid default profile opens without a warning callback.
	wOK, err := m.NewWindow("")
	if err != nil {
		t.Fatalf("NewWindow(\"\") with a valid default: %v", err)
	}
	if wOK == nil {
		t.Fatal("NewWindow(\"\") returned a nil window")
	}
	baseline := len(scheduled)

	scheduled = nil
	wBad, err := m.NewWindow("definitely-nosuch")
	if err != nil {
		t.Fatalf("NewWindow(unknown) should still succeed via the default shell, got err %v", err)
	}
	if wBad == nil {
		t.Fatal("NewWindow(unknown) returned a nil window")
	}
	if len(scheduled) < baseline+1 {
		t.Fatalf("unknown profile scheduled %d callbacks; want at least %d "+
			"(baseline %d + the warning msgbox)", len(scheduled), baseline+1, baseline)
	}
	// Both windows fell back to the live cat default shell.
	if len(m.windowOrder) != 2 {
		t.Errorf("windowOrder = %d, want 2 spawned windows", len(m.windowOrder))
	}
	// Never run scheduled[*]: the warning callback opens a modal msgbox.
}

// TestBuildWindowRoot_ValidLayoutTwoLeaves is the regression for finding
// #44 (happy path): a profile whose Layout DSL declares a split yields a
// window whose Root is a live split with both leaves spawned.
func TestBuildWindowRoot_ValidLayoutTwoLeaves(t *testing.T) {
	m := newCatMux(t)
	prof := &profile.Profile{
		Name:    "split",
		Command: "cat",
		Layout:  "split-v:0.5{leaf:profile=shell}{leaf:profile=shell}",
	}

	w, err := m.openWindowFromProfile(prof)
	if err != nil {
		t.Fatalf("openWindowFromProfile(valid layout): %v", err)
	}
	ws := m.windows[w.Self()]
	if ws == nil {
		t.Fatal("window not registered in m.windows")
	}
	if ws.Root == nil || ws.Root.Kind != layout.NodeSplit {
		t.Fatalf("Root should be a split, got %+v", ws.Root)
	}
	leaves := ws.Root.CollectLeaves()
	if len(leaves) != 2 {
		t.Fatalf("layout produced %d leaves, want 2", len(leaves))
	}
	for i, l := range leaves {
		if l.Pane == nil || l.Pane.Term == nil {
			t.Errorf("leaf %d has no live pane/terminal", i)
		}
	}
}

// TestBuildWindowRoot_InvalidLayoutDegradesWithWarning is the regression
// for finding #44 (degrade path): an invalid Layout ("leaf:nope" is
// missing the '=') must not block the window — buildWindowRoot reaps any
// partial spawns, schedules a warning via CallSoon, and opens a single
// plain pane instead. We capture CallSoon and never run it (it opens a
// modal).
func TestBuildWindowRoot_InvalidLayoutDegradesWithWarning(t *testing.T) {
	m := newCatMux(t)

	var scheduled []func()
	views.SetCallSoon(func(fn func()) { scheduled = append(scheduled, fn) })
	defer views.SetCallSoon(nil)

	prof := &profile.Profile{
		Name:    "bad",
		Command: "cat",
		Layout:  "leaf:nope", // no '=' → parse error → degrade
	}

	w, err := m.openWindowFromProfile(prof)
	if err != nil {
		t.Fatalf("invalid layout should degrade to a single pane, not error: %v", err)
	}
	ws := m.windows[w.Self()]
	if ws == nil {
		t.Fatal("window not registered in m.windows")
	}
	if ws.Root == nil || ws.Root.Kind != layout.NodeLeaf {
		t.Fatalf("degraded Root should be a single leaf, got %+v", ws.Root)
	}
	if got := len(ws.Root.CollectLeaves()); got != 1 {
		t.Fatalf("degraded window has %d leaves, want 1", got)
	}
	if leaf := ws.Root; leaf.Pane == nil || leaf.Pane.Term == nil {
		t.Error("degraded single leaf has no live pane/terminal")
	}
	if len(scheduled) < 1 {
		t.Fatal("invalid layout scheduled no warning callback")
	}
	// Never run scheduled[*]: the warning callback opens a modal msgbox.
}

// TestDoSplitWith_SplitsFocusedPane is the regression for finding #37:
// doSplitWith (shared by the split chords and connect_split) divides the
// focused pane, spawning the sibling from the given profile and moving
// focus to the new leaf. Driven headlessly — InsertWindow gives the fresh
// window desktop focus, so currentWindow() resolves it. (The connect_split
// host-picker leg is modal and is not exercised here.)
func TestDoSplitWith_SplitsFocusedPane(t *testing.T) {
	m := newCatMux(t)
	catProf := m.Opts.Profiles[0]

	w, err := m.openWindowFromProfile(catProf)
	if err != nil {
		t.Fatalf("openWindowFromProfile: %v", err)
	}
	ws := m.windows[w.Self()]
	if ws == nil {
		t.Fatal("window not registered in m.windows")
	}
	if m.currentWindow() != ws {
		t.Fatalf("currentWindow() = %v, want the freshly opened window", m.currentWindow())
	}
	if ws.Root.Kind != layout.NodeLeaf {
		t.Fatalf("precondition: fresh window should have a single leaf, got kind %v", ws.Root.Kind)
	}
	origID := ws.Focus.Pane.ID

	m.doSplitWith(catProf, true) // fvmux "vertical" split → panes stacked

	if ws.Root.Kind != layout.NodeSplit {
		t.Fatalf("after doSplitWith, Root kind = %v, want a split", ws.Root.Kind)
	}
	// Orientation pin (finding #37): fvmux's "vertical split" stacks panes
	// top/bottom, which fv-go models as a SplitHorizontal splitter (a
	// horizontal divider bar). The layout algebra inverts the H/V names
	// exactly once: doSplitWith(true) → SplitV → views.SplitHorizontal (see
	// internal/layout/{node,ops}.go and fv-go's SplitOrientation constants).
	// Asserting the concrete constant here stops a doc/comment edit elsewhere
	// from silently flipping the split axis without a failing test.
	if ws.Root.Orientation != views.SplitHorizontal {
		t.Errorf("doSplitWith(vertical=true) Root.Orientation = %v, want views.SplitHorizontal (stacked)", ws.Root.Orientation)
	}
	leaves := ws.Root.CollectLeaves()
	if len(leaves) != 2 {
		t.Fatalf("after doSplitWith, %d leaves, want 2", len(leaves))
	}
	if ws.Focus == nil || ws.Focus.Pane == nil {
		t.Fatal("focus lost after split")
	}
	if ws.Focus.Pane.ID == origID {
		t.Error("focus stayed on the original pane; want the new split sibling")
	}
	for i, l := range leaves {
		if l.Pane == nil || l.Pane.Term == nil {
			t.Errorf("leaf %d missing a live terminal", i)
		}
	}

	// The other axis: a second fresh window split with vertical=false must
	// produce a side-by-side views.SplitVertical splitter (SplitH). Pinning
	// both directions locks the full mapping, not just one leg.
	w2, err := m.openWindowFromProfile(catProf)
	if err != nil {
		t.Fatalf("openWindowFromProfile (second window): %v", err)
	}
	ws2 := m.windows[w2.Self()]
	if ws2 == nil {
		t.Fatal("second window not registered in m.windows")
	}
	if m.currentWindow() != ws2 {
		t.Fatalf("currentWindow() = %v, want the second window", m.currentWindow())
	}
	if ws2.Root.Kind != layout.NodeLeaf {
		t.Fatalf("precondition: second fresh window should have a single leaf, got kind %v", ws2.Root.Kind)
	}

	m.doSplitWith(catProf, false) // fvmux "horizontal" split → panes side-by-side

	if ws2.Root.Kind != layout.NodeSplit {
		t.Fatalf("after doSplitWith(false), Root kind = %v, want a split", ws2.Root.Kind)
	}
	if ws2.Root.Orientation != views.SplitVertical {
		t.Errorf("doSplitWith(vertical=false) Root.Orientation = %v, want views.SplitVertical (side-by-side)", ws2.Root.Orientation)
	}
}

// TestRegisterWindow_ShadowRespectsConfig is the regression for finding
// #45: registerWindow honours [appearance] window_shadow. fv-go's
// NewWindow sets SfShadow by default; with window_shadow=false the flag
// must be cleared at the single registration chokepoint, and with the
// default true it must survive.
func TestRegisterWindow_ShadowRespectsConfig(t *testing.T) {
	m := newCatMux(t)
	if !m.Opts.Config.Appearance.WindowShadow {
		t.Fatal("precondition: default config should enable window_shadow")
	}

	// Default config (shadow on): SfShadow survives registration.
	w := views.NewWindow(geom.NewRect(0, 0, 20, 10), "shadowed", 1)
	if !w.GetState(consts.SfShadow) {
		t.Fatal("precondition: fv-go NewWindow should set SfShadow by default")
	}
	m.registerWindow(w, &windowState{ID: session.NewWindowID(), Number: 1, Frame: w})
	if !w.GetState(consts.SfShadow) {
		t.Error("SfShadow cleared even though window_shadow=true")
	}

	// window_shadow=false: registerWindow clears SfShadow.
	m.Opts.Config.Appearance.WindowShadow = false
	w2 := views.NewWindow(geom.NewRect(0, 0, 20, 10), "flat", 2)
	if !w2.GetState(consts.SfShadow) {
		t.Fatal("precondition: fresh window should start with SfShadow set")
	}
	m.registerWindow(w2, &windowState{ID: session.NewWindowID(), Number: 2, Frame: w2})
	if w2.GetState(consts.SfShadow) {
		t.Error("window_shadow=false but SfShadow still set after registerWindow")
	}
}
