package prefix

import (
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"

	"github.com/oldwired/fvmux/internal/commands"
)

func newTestRegistry() (*commands.Registry, *bool, *bool) {
	r := commands.New()
	var fired, doubleTap bool

	r.Register(&commands.Command{
		ID:    1,
		Name:  "Test Cmd",
		Chord: "C-g x",
		Action: func(*commands.Ctx) {
			fired = true
		},
	})
	r.Register(&commands.Command{
		ID:    2,
		Name:  "Double Tap",
		Chord: "C-g C-g",
		Action: func(*commands.Ctx) {
			doubleTap = true
		},
	})
	return r, &fired, &doubleTap
}

func keyEvent(code uint16, unicode rune) *drivers.Event {
	return &drivers.Event{
		What:        consts.EvKeyboard,
		KeyCode:     code,
		UnicodeChar: unicode,
	}
}

func TestPrefix_SingleChordFires(t *testing.T) {
	reg, fired, _ := newTestRegistry()
	v := New(reg, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	if !v.Armed() {
		t.Fatal("prefix should be armed after Ctrl-G")
	}
	v.HandleEvent(keyEvent(0, 'x'))
	if !*fired {
		t.Fatal("C-g x should have fired the command")
	}
	if v.Armed() {
		t.Fatal("prefix should disarm after the chord completes")
	}
}

func TestPrefix_UnboundKeyDisarms(t *testing.T) {
	reg, fired, _ := newTestRegistry()
	v := New(reg, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	v.HandleEvent(keyEvent(0, 'q')) // not bound
	if *fired {
		t.Fatal("unrelated key should not fire the bound command")
	}
	if v.Armed() {
		t.Fatal("unbound second keystroke should still disarm")
	}
}

func TestPrefix_DoubleTapPassthrough(t *testing.T) {
	reg, _, doubleTap := newTestRegistry()
	v := New(reg, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	if !*doubleTap {
		t.Fatal("double-tap of prefix should fire 'C-g C-g' binding")
	}
}

func TestPrefix_ArmTimeoutExpires(t *testing.T) {
	reg, fired, _ := newTestRegistry()
	v := New(reg, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	// Force the armed timestamp into the past.
	v.armedAt = time.Now().Add(-3 * time.Second)

	v.HandleEvent(keyEvent(0, 'x'))
	if *fired {
		t.Fatal("chord must not fire after arm window has lapsed")
	}
}

func TestRecordPress_TriplePressFires(t *testing.T) {
	v := &View{}
	var fired int
	v.OnTriplePress = func() { fired++ }

	now := time.Now()
	v.recordPress(now)
	v.recordPress(now.Add(200 * time.Millisecond))
	v.recordPress(now.Add(400 * time.Millisecond))

	if fired != 1 {
		t.Fatalf("expected triple-press to fire once, got %d", fired)
	}
	// State resets — the next press alone must not re-fire.
	v.recordPress(now.Add(500 * time.Millisecond))
	if fired != 1 {
		t.Fatalf("triple-press fired again after reset; got %d", fired)
	}
}

func TestPrefix_TriplePressViaHandleEvent(t *testing.T) {
	reg, _, _ := newTestRegistry()
	v := New(reg, &commands.Ctx{}, Default)
	var fired int
	v.OnTriplePress = func() { fired++ }

	// Three real prefix taps through the state machine (not a direct
	// recordPress) — the gesture as a user actually performs it.
	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	if fired != 1 {
		t.Fatalf("three prefix taps should fire OnTriplePress once, got %d", fired)
	}
}

func TestPrefix_CtrlSecondKeyDistinctFromBareLetter(t *testing.T) {
	r := commands.New()
	var bare, ctrl bool
	r.Register(&commands.Command{ID: 1, Name: "bare", Chord: "C-g c",
		Action: func(*commands.Ctx) { bare = true }})
	r.Register(&commands.Command{ID: 2, Name: "ctrl", Chord: "C-g C-c",
		Action: func(*commands.Ctx) { ctrl = true }})
	v := New(r, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	v.HandleEvent(keyEvent(0, 'c')) // bare 'c'
	if !bare || ctrl {
		t.Fatalf("C-g c should fire bare only (bare=%v ctrl=%v)", bare, ctrl)
	}

	bare, ctrl = false, false
	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	// Some terminals report the bare letter in UnicodeChar alongside the
	// Ctrl key code; the chord must still resolve to C-c, not c.
	v.HandleEvent(keyEvent(consts.KbCtrlC, 'c'))
	if !ctrl || bare {
		t.Fatalf("C-g C-c should fire ctrl only (bare=%v ctrl=%v)", bare, ctrl)
	}
}

func TestPrefix_SuspendedPassesThrough(t *testing.T) {
	reg, fired, _ := newTestRegistry()
	v := New(reg, &commands.Ctx{}, Default)
	v.SetSuspended(true)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	if v.Armed() {
		t.Fatal("suspended prefix must not arm on the prefix key")
	}
	v.HandleEvent(keyEvent(0, 'x'))
	if *fired {
		t.Fatal("suspended prefix must not dispatch chords")
	}

	v.SetSuspended(false)
	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	if !v.Armed() {
		t.Fatal("resumed prefix should arm again")
	}
}

// TestPrefix_DisabledChordConsumesWithoutFiring is the regression for finding
// #24: a chord bound to a command whose Enabled predicate returns false must
// still CONSUME its second keystroke (ev.Clear) so the key can't leak into the
// focused pane — while NOT running the disabled Action. (The concrete bug: a
// disabled `C-g D` typed a literal 'D' at the shell prompt.)
func TestPrefix_DisabledChordConsumesWithoutFiring(t *testing.T) {
	r := commands.New()
	var fired bool
	r.Register(&commands.Command{
		ID:      1,
		Name:    "Disabled Cmd",
		Chord:   "C-g D",
		Enabled: func(*commands.Ctx) bool { return false },
		Action:  func(*commands.Ctx) { fired = true },
	})
	v := New(r, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	ev := keyEvent(0, 'D')
	v.HandleEvent(ev)

	if fired {
		t.Fatal("a disabled command's Action must not run")
	}
	if ev.What != consts.EvNothing {
		t.Fatalf("a bound-but-disabled chord must still consume its key; ev.What = %#x, want EvNothing (%#x)",
			ev.What, consts.EvNothing)
	}
	if v.Armed() {
		t.Fatal("prefix should disarm after a bound-but-disabled chord")
	}
}

// TestPrefix_EnabledChordRunsAndConsumes is the mirror of the disabled case:
// an enabled command both runs its Action and consumes the keystroke.
func TestPrefix_EnabledChordRunsAndConsumes(t *testing.T) {
	r := commands.New()
	var fired bool
	r.Register(&commands.Command{
		ID:      1,
		Name:    "Enabled Cmd",
		Chord:   "C-g D",
		Enabled: func(*commands.Ctx) bool { return true },
		Action:  func(*commands.Ctx) { fired = true },
	})
	v := New(r, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	ev := keyEvent(0, 'D')
	v.HandleEvent(ev)

	if !fired {
		t.Fatal("an enabled command's Action should run")
	}
	if ev.What != consts.EvNothing {
		t.Fatalf("a fired chord must consume its key; ev.What = %#x, want EvNothing (%#x)",
			ev.What, consts.EvNothing)
	}
}

// TestPrefix_UnboundChordPassesThrough documents the passthrough contract: a
// second key that forms a chord string with no registry binding is left
// untouched (NOT consumed) so it reaches the focused view.
func TestPrefix_UnboundChordPassesThrough(t *testing.T) {
	r := commands.New()
	r.Register(&commands.Command{
		ID:     1,
		Name:   "Bound Cmd",
		Chord:  "C-g D",
		Action: func(*commands.Ctx) {},
	})
	v := New(r, &commands.Ctx{}, Default)

	v.HandleEvent(keyEvent(consts.KbCtrlG, 0))
	ev := keyEvent(0, 'Q') // forms "C-g Q" — not bound
	v.HandleEvent(ev)

	if ev.What != consts.EvKeyboard {
		t.Fatalf("an unbound chord's second key must pass through (not be consumed); ev.What = %#x, want EvKeyboard (%#x)",
			ev.What, consts.EvKeyboard)
	}
	if v.Armed() {
		t.Fatal("prefix should disarm after an unbound second key")
	}
}

func TestRecordPress_OutsideWindowDoesNotFire(t *testing.T) {
	v := &View{}
	var fired int
	v.OnTriplePress = func() { fired++ }

	now := time.Now()
	v.recordPress(now)
	v.recordPress(now.Add(500 * time.Millisecond))
	// Third press well outside tripleWindow.
	v.recordPress(now.Add(5 * time.Second))

	if fired != 0 {
		t.Fatalf("press outside window should not fire; got %d", fired)
	}
}
