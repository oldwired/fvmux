// Package layout owns the pane-tree algebra. A PaneNode is either a
// Leaf (one pane) or a Split (orientation + ratio + two children).
// Trees are strictly binary; CheckInvariants asserts this and six
// other safety properties after every mutation.
//
// H/V naming convention follows tmux: SplitH produces a VERTICAL
// splitter (panes side-by-side); SplitV produces a HORIZONTAL splitter
// (panes stacked). Surprises every reader exactly once.
package layout

import (
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// NodeKind discriminates between leaf and split nodes.
type NodeKind uint8

const (
	NodeLeaf NodeKind = iota
	NodeSplit
)

// PaneNode is one node in the layout tree.
type PaneNode struct {
	Kind        NodeKind
	Pane        *session.Pane          // valid iff Kind == NodeLeaf
	Orientation views.SplitOrientation // valid iff Kind == NodeSplit
	Ratio       float64                // (0,1) iff Kind == NodeSplit
	A, B        *PaneNode              // valid iff Kind == NodeSplit
	Parent      *PaneNode
}

// Leaf wraps p in a freshly-constructed leaf node.
func Leaf(p *session.Pane) *PaneNode { return &PaneNode{Kind: NodeLeaf, Pane: p} }

// Split wraps two child trees in a split node with default ratio 0.5.
// Children's Parent pointers are rewired.
func Split(orient views.SplitOrientation, a, b *PaneNode) *PaneNode {
	n := &PaneNode{Kind: NodeSplit, Orientation: orient, Ratio: 0.5, A: a, B: b}
	a.Parent = n
	b.Parent = n
	return n
}

// IsLeaf reports whether n is a leaf node.
func (n *PaneNode) IsLeaf() bool { return n != nil && n.Kind == NodeLeaf }

// Leaves walks n in-order and calls f on every leaf.
func (n *PaneNode) Leaves(f func(*PaneNode)) {
	if n == nil {
		return
	}
	if n.Kind == NodeLeaf {
		f(n)
		return
	}
	n.A.Leaves(f)
	n.B.Leaves(f)
}

// CollectLeaves returns every leaf in n, in-order.
func (n *PaneNode) CollectLeaves() []*PaneNode {
	var out []*PaneNode
	n.Leaves(func(l *PaneNode) { out = append(out, l) })
	return out
}

// FindByID returns the leaf for paneID, or nil.
func (n *PaneNode) FindByID(id session.PaneID) *PaneNode {
	var found *PaneNode
	n.Leaves(func(l *PaneNode) {
		if found == nil && l.Pane != nil && l.Pane.ID == id {
			found = l
		}
	})
	return found
}

// Sibling returns the other child of n's Parent, or nil for the root.
func (n *PaneNode) Sibling() *PaneNode {
	if n == nil || n.Parent == nil {
		return nil
	}
	if n.Parent.A == n {
		return n.Parent.B
	}
	return n.Parent.A
}

// Root walks Parent pointers up to the topmost node.
func (n *PaneNode) Root() *PaneNode {
	for n != nil && n.Parent != nil {
		n = n.Parent
	}
	return n
}
