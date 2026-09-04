package prefix

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/keys"
)

// ResizeView is a sticky-mode OfPreProcess listener: while installed,
// it captures every keyboard event and routes the rune + special atom
// (Esc, Tab, …) to OnKey, consuming the event when OnKey returns true.
//
// Used by Mux to implement Ctrl-G R "resize mode" — the host inserts
// a ResizeView, processes h/j/k/l keystrokes itself, and removes the
// view when Esc fires.
type ResizeView struct {
	views.Base
	OnKey func(r rune, special string) (handled bool)
}

// NewResizeView constructs an empty ResizeView; caller wires OnKey
// and inserts into the desktop.
func NewResizeView(onKey func(r rune, special string) (handled bool)) *ResizeView {
	v := &ResizeView{
		Base:  views.NewBase(geom.NewRect(0, 0, 0, 0)),
		OnKey: onKey,
	}
	v.Base.Options |= consts.OfPreProcess
	v.SetSelf(v)
	return v
}

// Draw is a no-op: the listener has zero size and paints nothing.
func (v *ResizeView) Draw() {}

// HandleEvent forwards keyboard events to OnKey. Non-keyboard events
// fall through unchanged.
func (v *ResizeView) HandleEvent(ev *drivers.Event) {
	if v.OnKey == nil || ev.What&consts.EvKeyboard == 0 {
		v.Base.HandleEvent(ev)
		return
	}
	id := ev.EffectiveKey()
	r := id.Rune
	special := ""
	if id.Key != term.KeyNone {
		if step, ok := keys.StepFromIdentity(id); ok {
			special = keys.FormatStep(step)
		}
	}
	if v.OnKey(r, special) {
		ev.Clear()
	}
}
