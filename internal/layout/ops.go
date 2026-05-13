package layout

import (
	"errors"

	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

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

	if grandparent == nil {
		// parent was the root; sibling becomes the new root.
		return sibling, false
	}
	if grandparent.A == parent {
		grandparent.A = sibling
	} else {
		grandparent.B = sibling
	}
	return root, false
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
