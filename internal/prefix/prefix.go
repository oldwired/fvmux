// Package prefix implements fvmux's prefix-key dispatcher — the
// OfPreProcess view that watches every keyboard event, arms on Ctrl-G,
// and resolves the following keystroke against the commands.Registry.
package prefix

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
)

// armTimeout: if no second keystroke arrives within this window after
// the prefix is pressed, the next keystroke disarms but is NOT treated
// as part of a chord. Matches tmux's default 2-second behaviour.
const armTimeout = 2 * time.Second

// tripleWindow: three prefix presses within this window in a row fire
// the OnTriplePress callback. Easter egg — splash replay.
const tripleWindow = 1500 * time.Millisecond

// View is the OfPreProcess listener. Insert one into Desktop's children
// (NOT as the current child) so it intercepts keyboard events before
// the focused window receives them.
type View struct {
	views.Base

	reg  *commands.Registry
	ctx  *commands.Ctx
	spec Spec

	// suspended, when true, makes HandleEvent pass every event through
	// untouched. Set while a modal sub-mode (resize) owns the keyboard so
	// the prefix key reaches that mode instead of arming a chord.
	suspended bool

	armedAt time.Time // zero value ⇒ not armed

	// Last two prefix-press timestamps (newest at [1]). Three presses
	// within tripleWindow trigger OnTriplePress (splash replay).
	lastPresses [2]time.Time

	// OnTriplePress, if non-nil, is called on the third prefix press
	// within tripleWindow. Cleared by the caller (typically a one-shot
	// installer). Re-entry safe.
	OnTriplePress func()
}

// New returns a freshly-constructed prefix view armed on spec.KeyCode.
// Caller is responsible for inserting it into the desktop.
func New(reg *commands.Registry, ctx *commands.Ctx, spec Spec) *View {
	v := &View{
		Base: views.NewBase(geom.NewRect(0, 0, 0, 0)),
		reg:  reg,
		ctx:  ctx,
		spec: spec,
	}
	v.Base.Options |= consts.OfPreProcess
	v.SetSelf(v)
	return v
}

// SetSpec switches which key arms the prefix listener. Used when the
// user changes prefix_key via the first-run wizard or a reset.
func (v *View) SetSpec(spec Spec) { v.spec = spec }

// Spec returns the spec the listener currently arms on. Commands that
// depend on the live prefix (double-tap literal forward) read it at
// fire time so a rebind is never stale.
func (v *View) Spec() Spec { return v.spec }

// SetSuspended toggles passthrough mode. While suspended the prefix view
// ignores (and does not consume) every event, so a sticky sub-mode like
// resize — whose listener sits behind the prefix view in z-order — sees
// the prefix key itself instead of having it swallowed into a chord.
func (v *View) SetSuspended(s bool) {
	v.suspended = s
	if s {
		v.armedAt = time.Time{} // drop any half-entered chord.
	}
}

// Armed reports whether a prefix has been seen and is awaiting a chord.
// Surfaced for the status-line PREFIX indicator (sub-step 6).
func (v *View) Armed() bool {
	if v.armedAt.IsZero() {
		return false
	}
	return time.Since(v.armedAt) <= armTimeout
}

// Draw is a no-op: the prefix view has zero size and produces no pixels.
func (v *View) Draw() {}

// HandleEvent runs the prefix state machine. Only keyboard events are
// inspected; everything else falls through unchanged.
func (v *View) HandleEvent(ev *drivers.Event) {
	if v.suspended {
		v.Base.HandleEvent(ev)
		return
	}
	if ev.What&consts.EvKeyboard == 0 {
		v.Base.HandleEvent(ev)
		return
	}

	// Lazily expire an unfollowed prefix.
	if !v.armedAt.IsZero() && time.Since(v.armedAt) > armTimeout {
		v.armedAt = time.Time{}
	}

	if v.armedAt.IsZero() {
		if ev.KeyCode == v.spec.KeyCode {
			now := time.Now()
			v.armedAt = now
			v.recordPress(now)
			ev.Clear()
		}
		return
	}

	// Second keystroke: disarm regardless of match.
	v.armedAt = time.Time{}

	// A repeated prefix press lands here (the double-tap that sends a
	// literal prefix). Count it too, so three taps in a row fire
	// OnTriplePress — recording only in the arm branch would miss every
	// even-numbered tap and the gesture could never reach three.
	if ev.KeyCode == v.spec.KeyCode {
		v.recordPress(time.Now())
	}

	chord := v.chordOf(ev)
	if chord == "" {
		// Unbound key — let it through to the focused view.
		return
	}
	c := v.reg.LookupChord(chord)
	if c == nil {
		return
	}
	if c.Enabled != nil && !c.Enabled(v.ctx) {
		// The chord is bound — a currently-disabled command must still
		// swallow its keystroke. Without this the second key leaks into
		// the focused pane (e.g. C-g D outside tmux typed a literal 'D'
		// at the shell prompt).
		ev.Clear()
		return
	}
	if c.Action != nil {
		c.Action(v.ctx)
	}
	ev.Clear()
}

// recordPress shifts the press-timestamp ring and fires OnTriplePress
// whenever three consecutive presses fall within tripleWindow.
func (v *View) recordPress(now time.Time) {
	prev, prev2 := v.lastPresses[1], v.lastPresses[0]
	v.lastPresses[0] = prev
	v.lastPresses[1] = now
	if v.OnTriplePress == nil {
		return
	}
	if !prev.IsZero() && !prev2.IsZero() &&
		now.Sub(prev2) <= tripleWindow {
		// Reset to avoid re-firing on the next single press.
		v.lastPresses = [2]time.Time{}
		v.OnTriplePress()
	}
}

// chordOf formats the second keystroke as the chord string used in the
// registry. Returns "" for keys that can't appear in a chord (e.g.,
// modifier-only events).
//
// Special keys (Tab, Space, Esc, Enter, Backspace, 0–9 digits) are
// canonicalised to their named tokens so registry chords stay
// readable.
func (v *View) chordOf(ev *drivers.Event) string {
	if ev.KeyCode == v.spec.KeyCode {
		return v.spec.ChordToken + " " + v.spec.ChordToken
	}
	if atom := specialAtom(ev.KeyCode); atom != "" {
		return v.spec.ChordToken + " " + atom
	}
	// A Ctrl-modified second key must canonicalise to a "C-x" token, not
	// fall through to its bare letter — otherwise C-g C-c would collide
	// with C-g c (e.g. a stray Ctrl firing New Window).
	if atom := ctrlAtom(ev.KeyCode); atom != "" {
		return v.spec.ChordToken + " " + atom
	}
	if ev.UnicodeChar != 0 {
		return v.spec.ChordToken + " " + string(ev.UnicodeChar)
	}
	return ""
}

// ctrlAtom maps a Ctrl+letter key code to its canonical "C-x" chord step.
// Returns "" for anything that isn't a Ctrl-letter. Tab/Enter/Backspace
// share code points with Ctrl-I/M/H at the byte level but have distinct
// key codes handled by specialAtom first, so they never reach here.
func ctrlAtom(code uint16) string {
	if l, ok := ctrlLetters[code]; ok {
		return "C-" + string(l)
	}
	return ""
}

var ctrlLetters = map[uint16]rune{
	consts.KbCtrlA: 'a', consts.KbCtrlB: 'b', consts.KbCtrlC: 'c',
	consts.KbCtrlD: 'd', consts.KbCtrlE: 'e', consts.KbCtrlF: 'f',
	consts.KbCtrlG: 'g', consts.KbCtrlH: 'h', consts.KbCtrlI: 'i',
	consts.KbCtrlJ: 'j', consts.KbCtrlK: 'k', consts.KbCtrlL: 'l',
	consts.KbCtrlM: 'm', consts.KbCtrlN: 'n', consts.KbCtrlO: 'o',
	consts.KbCtrlP: 'p', consts.KbCtrlQ: 'q', consts.KbCtrlR: 'r',
	consts.KbCtrlS: 's', consts.KbCtrlT: 't', consts.KbCtrlU: 'u',
	consts.KbCtrlV: 'v', consts.KbCtrlW: 'w', consts.KbCtrlX: 'x',
	consts.KbCtrlY: 'y', consts.KbCtrlZ: 'z',
}

// DispatchableChord reports whether chord — in canonical registry form
// "<prefix> <step>" — can actually be produced by the dispatcher's
// state machine. Two requirements:
//
//   - The first step must be the default prefix token ("C-g"):
//     keybindings.toml is authored in the default-prefix space and
//     rebound afterwards, so a chord led by anything else ("C-x a")
//     applies fine but is never emitted — while having already
//     stripped the command's working factory chord.
//   - The second step must be one of the atoms chordOf emits: a
//     C-letter token, one of the five special atoms (Tab, Esc, Enter,
//     Backspace, Space), or a bare printable character. Arrows,
//     F-keys, and A-/S- modified steps parse but can never fire.
//
// Callers reject failing bindings with a warning instead of applying
// them.
func DispatchableChord(chord string) bool {
	steps := strings.Fields(chord)
	if len(steps) != 2 {
		return false
	}
	if steps[0] != Default.ChordToken {
		return false
	}
	return dispatchableStep(steps[1])
}

func dispatchableStep(step string) bool {
	if utf8.RuneCountInString(step) == 1 {
		return true // bare printable character (including "-")
	}
	switch step {
	case "Tab", "Esc", "Enter", "Backspace", "Space":
		return true
	}
	if len(step) == 3 && strings.HasPrefix(step, "C-") {
		return step[2] >= 'a' && step[2] <= 'z'
	}
	return false
}

// specialAtom maps fv-go key codes to canonical chord-step names. We
// match against the registered chord strings (e.g., "Tab" in "C-g Tab").
func specialAtom(code uint16) string {
	switch code {
	case consts.KbTab:
		return "Tab"
	case consts.KbEsc:
		return "Esc"
	case consts.KbEnter:
		return "Enter"
	case consts.KbBack:
		return "Backspace"
	case consts.KbSpaceBar:
		return "Space"
	}
	return ""
}
