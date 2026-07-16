package prefix

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/debug"
)

// MouseView is a topmost, full-desktop-sized child of the desktop that
// observes every mouse-down event before the windows and splitters
// under it do. fv-go dispatches mouse events to the topmost child whose
// MouseInView contains the click; if that child returns without
// clearing ev.What, dispatch falls through to the next child below —
// which is exactly the propagate-vs-consume model we want for
// click-to-focus, right-click context menus, and wheel-forwarding.
//
// Note: OfPreProcess on Base only affects keyboard routing. What makes
// this work for mouse is the bounds: a 0-size view receives no clicks
// (which is why the original 0×0 MouseView was a silent no-op and the
// splitter ate wheel ticks as drags). GfGrowAll keeps the bounds in
// sync with desktop resizes.
type MouseView struct {
	views.Base
	OnMouse func(ev *drivers.Event) (consume bool)
}

// NewMouseView constructs the listener spanning bounds. GfGrowAll
// tracks desktop resizes from then on.
func NewMouseView(bounds geom.Rect, onMouse func(*drivers.Event) bool) *MouseView {
	v := &MouseView{
		Base:    views.NewBase(bounds),
		OnMouse: onMouse,
	}
	v.SetSelf(v)
	v.GrowMode = consts.GfGrowAll // follow the desktop on screen resize.
	return v
}

// Draw is a no-op so the listener is invisible.
func (v *MouseView) Draw() {}

// HandleEvent forwards mouse-down events to OnMouse. OnMouse returning
// true consumes the event (clears ev.What); false leaves it active so
// fv-go's dispatcher continues down through windows / splitters /
// terminals as normal.
//
// Every event (including the EvMouseMove deltas the splitter's drag
// loop eats) is logged when debug is on, so investigations can see
// whether the splitter got there first.
func (v *MouseView) HandleEvent(ev *drivers.Event) {
	if v.OnMouse == nil {
		return
	}
	if debug.Mouse() {
		debug.Logf("mouse",
			"MouseView.HandleEvent: what=%#04x buttons=%#02x where=(%d,%d) view-size=%dx%d origin=(%d,%d)",
			ev.What, ev.Buttons, ev.Where.X, ev.Where.Y,
			v.Size.X, v.Size.Y, v.Origin.X, v.Origin.Y)
	}
	if ev.What != consts.EvMouseDown {
		return
	}
	consumed := v.OnMouse(ev)
	if consumed {
		ev.Clear()
	}
	if debug.Mouse() {
		debug.Logf("mouse", "  → consumed=%t ev.What-after=%#04x",
			consumed, ev.What)
	}
}
