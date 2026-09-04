package layout

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// Materialize converts root into an fv-go view tree fitting bounds and
// returns the top-level view (a Terminal for a single-leaf root, or a
// SplitGroup hierarchy otherwise). Every leaf's Pane.LastRect is
// updated to the global rect it now occupies.
//
// If zoomed is non-nil and points at a leaf in the tree, only that leaf is
// rendered full-bounds. LastRect continues to describe every leaf's logical,
// unzoomed geometry so directional navigation remains predictable in zoom
// mode; zoom-aware mouse hit-testing treats the visible pane as full-bounds.
//
// Each leaf's terminal is detached from its previous fv-go parent
// before re-insertion, so callers can rebuild the tree on every layout
// change without orphaning or double-parenting the running PTYs.
func Materialize(root *PaneNode, bounds geom.Rect, zoomed *session.PaneID) views.View {
	if root == nil {
		return nil
	}
	updateLeafRects(root, bounds)
	var top views.View
	if zoomed != nil {
		if l := root.FindByID(*zoomed); l != nil && l.Pane != nil {
			detach(l.Pane.Term)
			top = l.Pane.Term
		}
	}
	if top == nil {
		top = materializeView(root, bounds)
	}
	if top != nil {
		// The window inserts this view directly and, on a mouse-drag
		// resize, stretches it through fv-go's GrowMode propagation (there
		// is no rerender on the drag). Pin the interior top-left and grow
		// only the bottom-right so the body fills the interior as the
		// window changes size. A single Terminal pane already defaults to
		// this mode, but NewSplitGroup defaults to GfGrowAll — which moves
		// all four corners by the delta and so translates the whole split
		// toward the bottom-right corner at constant size ("glued to the
		// lower-right"). Override it here, where every creation/rerender
		// path funnels through, so single and split bodies anchor alike.
		top.BaseView().GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	}
	return top
}

func materializeView(n *PaneNode, bounds geom.Rect) views.View {
	if n.Kind == NodeLeaf {
		v := views.View(n.Pane.Term)
		detach(v)
		return v
	}
	splitPos := splitPosFor(bounds, n.Orientation, n.Ratio)
	leftR, rightR := childRects(bounds, n.Orientation, splitPos)
	p1 := materializeView(n.A, leftR)
	p2 := materializeView(n.B, rightR)
	sg := views.NewSplitGroup(bounds, n.Orientation, splitPos)
	// fv-go v0.5.2 reports accepted mouse drags after synchronizing the
	// SplitGroup's own SplitPos. Capture this exact model node so nested
	// splitters persist independently across rerenders and session saves.
	sg.OnRatioChanged = func(ratio float64) {
		n.Ratio = ratio
		// The fv-go panels move live during the drag; keep fvmux's logical
		// hit-testing/navigation rectangles in sync without rebuilding the
		// active view tree from inside the splitter's event loop.
		updateLeafRects(n, n.RenderRect)
	}
	sg.SetPanels(p1, p2)
	return sg
}

func updateLeafRects(n *PaneNode, bounds geom.Rect) {
	if n == nil {
		return
	}
	n.RenderRect = bounds
	if n.Kind == NodeLeaf {
		if n.Pane != nil {
			n.Pane.LastRect = bounds
		}
		return
	}
	splitPos := splitPosFor(bounds, n.Orientation, n.Ratio)
	a, b := childRects(bounds, n.Orientation, splitPos)
	updateLeafRects(n.A, a)
	updateLeafRects(n.B, b)
}

func splitPosFor(bounds geom.Rect, orient views.SplitOrientation, ratio float64) int {
	total := bounds.Width()
	if orient == views.SplitHorizontal {
		total = bounds.Height()
	}
	if total < 2*minResizePaneCells+1 {
		// Too small for both four-cell panes plus a divider. Keep both sides
		// non-negative: left width = pos, right width = total-pos-1, so
		// pos must stay in [0, total-1].
		pos := total / 2
		if pos > total-1 {
			pos = total - 1
		}
		if pos < 0 {
			pos = 0
		}
		return pos
	}
	pos := int(ratio * float64(total))
	if pos < minResizePaneCells {
		pos = minResizePaneCells
	}
	if pos > total-minResizePaneCells-1 {
		pos = total - minResizePaneCells - 1
	}
	return pos
}

// childRects splits bounds at splitPos, leaving a one-cell gap for the
// splitter. The split coordinate is clamped inside bounds so a degenerate
// splitPos can never produce a negative-width child rect (it collapses to
// zero width instead) — FocusDir / mouse hit-testing then simply can't
// match that pane, rather than indexing a malformed rect.
func childRects(bounds geom.Rect, orient views.SplitOrientation, splitPos int) (geom.Rect, geom.Rect) {
	if orient == views.SplitVertical {
		mid := clampInt(bounds.A.X+splitPos, bounds.A.X, bounds.B.X)
		rstart := clampInt(mid+1, bounds.A.X, bounds.B.X)
		return geom.NewRect(bounds.A.X, bounds.A.Y, mid, bounds.B.Y),
			geom.NewRect(rstart, bounds.A.Y, bounds.B.X, bounds.B.Y)
	}
	mid := clampInt(bounds.A.Y+splitPos, bounds.A.Y, bounds.B.Y)
	rstart := clampInt(mid+1, bounds.A.Y, bounds.B.Y)
	return geom.NewRect(bounds.A.X, bounds.A.Y, bounds.B.X, mid),
		geom.NewRect(bounds.A.X, rstart, bounds.B.X, bounds.B.Y)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
