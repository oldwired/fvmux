package prefix

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// SyncView is a non-consuming OfPreProcess listener: every keyboard
// event it sees is forwarded to OnKey, but the event is left active so
// the normal focused-pane path still runs.
//
// Used by the Mux's Ctrl-G ~ sync-input toggle: the callback re-dispatches
// the event (with a value copy) to every non-focused pane in the current
// window so all panes receive the same keystroke. The prefix.View runs
// earlier in the OfPreProcess chain — so prefix-chord keystrokes never
// reach the sync hook.
type SyncView struct {
	views.Base
	OnKey func(ev *drivers.Event)
}

// NewSyncView constructs a fallthrough listener; caller inserts it into
// the desktop. The view paints nothing.
func NewSyncView(onKey func(ev *drivers.Event)) *SyncView {
	v := &SyncView{
		Base:  views.NewBase(geom.NewRect(0, 0, 0, 0)),
		OnKey: onKey,
	}
	v.Base.Options |= consts.OfPreProcess
	v.SetSelf(v)
	return v
}

// Draw is a no-op.
func (v *SyncView) Draw() {}

// HandleEvent calls OnKey for every keyboard event without consuming.
func (v *SyncView) HandleEvent(ev *drivers.Event) {
	if ev.What&consts.EvKeyboard != 0 && v.OnKey != nil {
		v.OnKey(ev)
	}
}
