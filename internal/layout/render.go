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
	var top views.View
	if zoomed != nil {
		if l := root.FindByID(*zoomed); l != nil && l.Pane != nil {
			l.Pane.LastRect = bounds
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
		// Too small for the usual ≥2 margins. Keep both sides
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
	if pos < 2 {
		pos = 2
	}
	if pos > total-2 {
		pos = total - 2
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
