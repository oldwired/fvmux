// Package prefix implements fvmux's prefix-key dispatcher — the
// OfPreProcess view that watches every keyboard event, arms on Ctrl-G,
// and resolves the following keystroke against the commands.Registry.
package prefix

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/keys"
)

// armTimeout: if no second keystroke arrives within this window after
// the prefix is pressed, the next keystroke disarms but is NOT treated
// as part of a chord. Matches tmux's default 2-second behaviour.
const armTimeout = 2 * time.Second

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

	// OnUnknown reports an attempted chord that has no registry binding.
	// OnUnavailable reports a bound command whose live predicate vetoed it.
	// Both events are always consumed before either callback runs.
	OnUnknown     func(chord string)
	OnUnavailable func(command *commands.Command)
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
			v.armedAt = time.Now()
			ev.Clear()
		}
		return
	}

	// Second keystroke: disarm regardless of match.
	v.armedAt = time.Time{}

	chord := v.chordOf(ev)
	if chord == "" {
		if v.OnUnknown != nil {
			v.OnUnknown(v.describeChord(ev))
		}
		ev.Clear()
		return
	}
	c := v.reg.LookupChord(chord)
	if c == nil {
		if v.OnUnknown != nil {
			v.OnUnknown(chord)
		}
		ev.Clear()
		return
	}
	if c.Enabled != nil && !c.Enabled(v.ctx) {
		// The chord is bound — a currently-disabled command must still
		// swallow its keystroke. Without this the second key leaks into
		// the focused pane (e.g. C-g D outside tmux typed a literal 'D'
		// at the shell prompt).
		ev.Clear()
		if v.OnUnavailable != nil {
			v.OnUnavailable(c)
		}
		return
	}
	if c.Action != nil {
		c.Action(v.ctx)
	}
	ev.Clear()
}

// describeChord gives unknown non-dispatchable special keys a stable,
// readable status label. Dispatchable keys already use chordOf's canonical
// registry spelling; the numeric fallback keeps even future key codes honest.
func (v *View) describeChord(ev *drivers.Event) string {
	if chord := v.chordOf(ev); chord != "" {
		return chord
	}
	return fmt.Sprintf("%s key-%04x", v.spec.ChordToken, ev.KeyCode)
}

// chordOf formats the second keystroke as the chord string used in the
// registry. Returns "" for keys that can't appear in a chord (e.g.,
// modifier-only events).
//
// All non-prefix keys go through keys.StepFromEvent, which in turn uses
// Event.EffectiveKey. The repeated prefix is recognized directly because it
// is also the state-machine delimiter and must retain literal-prefix behavior.
func (v *View) chordOf(ev *drivers.Event) string {
	if ev.KeyCode == v.spec.KeyCode {
		return v.spec.ChordToken + " " + v.spec.ChordToken
	}
	if step, ok := keys.StepFromEvent(ev); ok {
		return v.spec.ChordToken + " " + keys.FormatStep(step)
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

// DispatchableChord is retained as a compatibility helper for callers that
// only need a boolean. Configuration uses keys.ValidateBindingChord directly
// so it can preserve actionable diagnostics.
func DispatchableChord(chord string) bool {
	canonical, diagnostics := keys.ValidateBindingChord(chord, Default.ChordToken)
	if canonical == "" {
		return false
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == keys.SeverityError {
			return false
		}
	}
	return true
}
