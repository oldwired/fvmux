package layout

import (
	"errors"
	"math"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// RemovalSuccessor chooses the pane that should receive focus when target is
// removed. The immediate sibling subtree is always preferred because Close
// promotes that subtree into the removed split's slot. If it contains several
// leaves, choose the leaf nearest the shared split boundary, then the one with
// the greatest overlap with target along that boundary.
//
// This must run before Close, which deliberately detaches target and its
// parent. Empty/unmaterialized geometry falls back to the sibling subtree's
// first leaf.
func RemovalSuccessor(target *PaneNode) *PaneNode {
	if target == nil || target.Parent == nil || target.Pane == nil {
		return nil
	}
	parent := target.Parent
	sibling := target.Sibling()
	if sibling == nil {
		return nil
	}
	leaves := sibling.CollectLeaves()
	if len(leaves) == 0 {
		return nil
	}
	if len(leaves) == 1 || target.Pane.LastRect.Width() <= 0 || target.Pane.LastRect.Height() <= 0 {
		return leaves[0]
	}

	from := target.Pane.LastRect
	targetIsA := parent.A == target
	best := leaves[0]
	bestGap, bestOverlap, bestOffset := math.MaxInt, -1, math.MaxInt
	for _, leaf := range leaves {
		if leaf == nil || leaf.Pane == nil {
			continue
		}
		to := leaf.Pane.LastRect
		gap, overlap, offset := removalScore(from, to, parent.Orientation, targetIsA)
		if gap < bestGap ||
			(gap == bestGap && overlap > bestOverlap) ||
			(gap == bestGap && overlap == bestOverlap && offset < bestOffset) {
			best, bestGap, bestOverlap, bestOffset = leaf, gap, overlap, offset
		}
	}
	return best
}

func removalScore(from, to geom.Rect, orient views.SplitOrientation, targetIsA bool) (gap, overlap, offset int) {
	if orient == views.SplitVertical {
		if targetIsA {
			gap = to.A.X - from.B.X
		} else {
			gap = from.A.X - to.B.X
		}
		overlap = intervalOverlap(from.A.Y, from.B.Y, to.A.Y, to.B.Y)
		offset = absInt((from.A.Y+from.B.Y)/2 - (to.A.Y+to.B.Y)/2)
	} else {
		if targetIsA {
			gap = to.A.Y - from.B.Y
		} else {
			gap = from.A.Y - to.B.Y
		}
		overlap = intervalOverlap(from.A.X, from.B.X, to.A.X, to.B.X)
		offset = absInt((from.A.X+from.B.X)/2 - (to.A.X+to.B.X)/2)
	}
	if gap < 0 {
		gap = 0
	}
	return gap, overlap, offset
}

func intervalOverlap(a0, a1, b0, b1 int) int {
	lo, hi := a0, a1
	if b0 > lo {
		lo = b0
	}
	if b1 < hi {
		hi = b1
	}
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// SplitH replaces target (a leaf) with a vertical splitter — panes
// arranged SIDE-BY-SIDE. (tmux's "split-horizontal" convention.)
// Returns the (possibly new) root.
func SplitH(root, target *PaneNode, newPane *session.Pane) *PaneNode {
	return splitAt(root, target, views.SplitVertical, newPane)
}

// SplitV replaces target with a horizontal splitter — panes stacked.
func SplitV(root, target *PaneNode, newPane *session.Pane) *PaneNode {
	return splitAt(root, target, views.SplitHorizontal, newPane)
}

func splitAt(root, target *PaneNode, orient views.SplitOrientation, newPane *session.Pane) *PaneNode {
	if target == nil || target.Kind != NodeLeaf {
		panic("layout: split target must be a leaf")
	}
	newLeaf := Leaf(newPane)

	if target.Parent == nil {
		// target IS the root; the split becomes the new root.
		sub := Split(orient, target, newLeaf)
		return sub
	}

	parent := target.Parent
	sub := Split(orient, target, newLeaf)
	sub.Parent = parent
	if parent.A == target {
		parent.A = sub
	} else {
		parent.B = sub
	}
	return root
}

// Close removes target. If target was the only leaf in the window,
// returns (nil, true) and the caller closes the window; otherwise
// returns (newRoot, false). The parent split collapses into the sibling.
func Close(root, target *PaneNode) (*PaneNode, bool) {
	if target == nil {
		return root, false
	}
	if target.Parent == nil {
		// Root leaf — last pane in the window.
		return nil, true
	}

	parent := target.Parent
	sibling := target.Sibling()
	grandparent := parent.Parent
	sibling.Parent = grandparent

	var newRoot *PaneNode
	if grandparent == nil {
		// parent was the root; sibling becomes the new root.
		newRoot = sibling
	} else {
		if grandparent.A == parent {
			grandparent.A = sibling
		} else {
			grandparent.B = sibling
		}
		newRoot = root
	}
	// Detach the removed nodes so any retained reference fails loudly
	// (a nil deref or a CheckInvariants violation) rather than silently
	// walking a half-collapsed subtree that's no longer in the live tree.
	target.Parent = nil
	parent.A, parent.B, parent.Parent = nil, nil, nil
	return newRoot, false
}

// Swap exchanges the Panes at a and b without touching tree shape.
// Both must be leaves.
func Swap(a, b *PaneNode) {
	if a == nil || b == nil || a == b {
		return
	}
	if a.Kind != NodeLeaf || b.Kind != NodeLeaf {
		panic("layout: Swap requires two leaves")
	}
	a.Pane, b.Pane = b.Pane, a.Pane
}

// ErrJoinNonLeaf is returned by JoinFrom when the source tree is not a
// single leaf. v1 only supports flattening a single-pane source window
// into a destination tree; multi-pane joins are a future enhancement.
var ErrJoinNonLeaf = errors.New("layout: JoinFrom source must be a single leaf")

// BreakOut detaches target from srcRoot. Returns (newSrcRoot,
// newWindowRoot). If srcRoot's only leaf was target, newSrcRoot is nil
// — caller closes the source window.
func BreakOut(srcRoot, target *PaneNode) (newSrcRoot, newWindowRoot *PaneNode) {
	newWindowRoot = Leaf(target.Pane)
	newSrcRoot, removed := Close(srcRoot, target)
	if removed {
		return nil, newWindowRoot
	}
	return newSrcRoot, newWindowRoot
}

// JoinFrom merges srcRoot into dstRoot by splitting dstTarget. v1
// refuses non-leaf source trees.
func JoinFrom(dstRoot, dstTarget, srcRoot *PaneNode, orient views.SplitOrientation) (*PaneNode, error) {
	if srcRoot == nil || srcRoot.Kind != NodeLeaf {
		return nil, ErrJoinNonLeaf
	}
	return splitAt(dstRoot, dstTarget, orient, srcRoot.Pane), nil
}
