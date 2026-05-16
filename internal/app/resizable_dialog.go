package app

import (
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// resizableDialog is dialogs.Dialog plus a SizeLimits override so
// fv-go's mouse-drag resize loop clamps the dialog to a sensible
// minimum. Without this, dragging a dialog too small lets children
// draw with negative bounds (some widgets then panic in their Draw,
// the editor / SFTP browser are the standout cases).
//
// SetSelf must be called on the outer wrapper so fv-go's virtual
// dispatch (Self().SizeLimits()) routes to this override; newResizable
// handles that.
type resizableDialog struct {
	*dialogs.Dialog
	minW, minH int
}

func newResizableDialog(bounds geom.Rect, title string, minW, minH int) *resizableDialog {
	rd := &resizableDialog{
		Dialog: dialogs.NewDialog(bounds, title),
		minW:   minW,
		minH:   minH,
	}
	rd.SetSelf(rd)
	return rd
}

// SizeLimits overrides Base.SizeLimits to enforce the layout-required
// minimum. fv-go's resizeLoop calls Self().SizeLimits() and uses the
// min component as a hard floor before applying its own 16×4 fallback.
func (r *resizableDialog) SizeLimits() (geom.Point, geom.Point) {
	return geom.Point{X: r.minW, Y: r.minH},
		geom.Point{X: 1 << 14, Y: 1 << 14}
}
