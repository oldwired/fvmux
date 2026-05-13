package layout

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// Materialize converts root into an fv-go view tree fitting bounds and
// returns the top-level view (a Terminal for a single-leaf root, or a
// SplitGroup hierarchy otherwise). Every leaf's Pane.LastRect is
// updated to the global rect it now occupies.
//
// If zoomed is non-nil and points at a leaf in the tree, only that
// leaf is rendered, full-bounds. The rest of the tree is left alone
// (siblings are skipped, but their cached LastRects are untouched).
//
// Each leaf's terminal is detached from its previous fv-go parent
// before re-insertion, so callers can rebuild the tree on every layout
// change without orphaning or double-parenting the running PTYs.
func Materialize(root *PaneNode, bounds geom.Rect, zoomed *session.PaneID) views.View {
	if root == nil {
		return nil
	}
	if zoomed != nil {
		if l := root.FindByID(*zoomed); l != nil && l.Pane != nil {
			l.Pane.LastRect = bounds
			detach(l.Pane.Term)
			return l.Pane.Term
		}
	}
	return materializeView(root, bounds)
}

func materializeView(n *PaneNode, bounds geom.Rect) views.View {
	if n.Kind == NodeLeaf {
		n.Pane.LastRect = bounds
		v := views.View(n.Pane.Term)
		detach(v)
		return v
	}
	splitPos := splitPosFor(bounds, n.Orientation, n.Ratio)
	leftR, rightR := childRects(bounds, n.Orientation, splitPos)
	p1 := materializeView(n.A, leftR)
	p2 := materializeView(n.B, rightR)
	sg := views.NewSplitGroup(bounds, n.Orientation, splitPos)
	sg.SetPanels(p1, p2)
	return sg
}

func splitPosFor(bounds geom.Rect, orient views.SplitOrientation, ratio float64) int {
	total := bounds.Width()
	if orient == views.SplitHorizontal {
		total = bounds.Height()
	}
	if total < 4 {
		return total / 2
	}
	pos := int(ratio * float64(total))
	if pos < 2 {
		pos = 2
	}
	if pos > total-2 {
		pos = total - 2
	}
	return pos
}

func childRects(bounds geom.Rect, orient views.SplitOrientation, splitPos int) (geom.Rect, geom.Rect) {
	if orient == views.SplitVertical {
		return geom.NewRect(bounds.A.X, bounds.A.Y, bounds.A.X+splitPos, bounds.B.Y),
			geom.NewRect(bounds.A.X+splitPos+1, bounds.A.Y, bounds.B.X, bounds.B.Y)
	}
	return geom.NewRect(bounds.A.X, bounds.A.Y, bounds.B.X, bounds.A.Y+splitPos),
		geom.NewRect(bounds.A.X, bounds.A.Y+splitPos+1, bounds.B.X, bounds.B.Y)
}

// detach removes v from its current parent group, if any. Safe to call
// on freshly-created or nil views (no-op).
func detach(v views.View) {
	if v == nil {
		return
	}
	bv := v.BaseView()
	if bv == nil || bv.Owner == nil {
		return
	}
	bv.Owner.Delete(v)
}
