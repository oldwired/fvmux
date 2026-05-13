// Package prefix implements fvmux's prefix-key dispatcher — the
// OfPreProcess view that watches every keyboard event, arms on Ctrl-G,
// and resolves the following keystroke against the commands.Registry.
package prefix

import (
	"time"

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

// View is the OfPreProcess listener. Insert one into Desktop's children
// (NOT as the current child) so it intercepts keyboard events before
// the focused window receives them.
type View struct {
	views.Base

	reg  *commands.Registry
	ctx  *commands.Ctx
	spec Spec

	armedAt time.Time // zero value ⇒ not armed
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
		// Unbound key — let it through to the focused view.
		return
	}
	c := v.reg.LookupChord(chord)
	if c == nil {
		return
	}
	if c.Enabled != nil && !c.Enabled(v.ctx) {
		return
	}
	if c.Action != nil {
		c.Action(v.ctx)
	}
	ev.Clear()
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
	if ev.UnicodeChar != 0 {
		return v.spec.ChordToken + " " + string(ev.UnicodeChar)
	}
	return ""
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
