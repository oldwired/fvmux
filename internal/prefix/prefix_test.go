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
