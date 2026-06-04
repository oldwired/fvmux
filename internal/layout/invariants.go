package layout

import (
	"fmt"

	"github.com/oldwired/fvmux/internal/session"
)

// CheckInvariants validates every safety property of root:
//
//  1. Root's Parent is nil.
//  2. Every Split has exactly two non-nil children.
//  3. No Leaf has a nil Pane.
//  4. Parent pointers are consistent (A.Parent == n && B.Parent == n).
//  5. Each PaneID appears once across the tree.
//  6. 0 < Ratio < 1 on every Split.
//  7. Leaf nodes have nil children; Split nodes have nil Pane.
//  8. No *PaneNode is reachable twice (no aliasing/DAG), and a Split's
//     two children are distinct nodes.
//
// Property tests assert this after every random mutation.
func CheckInvariants(root *PaneNode) error {
	if root == nil {
		return nil
	}
	if root.Parent != nil {
		return fmt.Errorf("root has non-nil parent")
	}
	return checkNode(root, map[session.PaneID]bool{}, map[*PaneNode]bool{})
}

func checkNode(n *PaneNode, seen map[session.PaneID]bool, seenNodes map[*PaneNode]bool) error {
	if seenNodes[n] {
		return fmt.Errorf("node %p reachable more than once (aliasing)", n)
	}
	seenNodes[n] = true
	switch n.Kind {
	case NodeLeaf:
		if n.Pane == nil {
			return fmt.Errorf("leaf with nil Pane")
		}
		if n.A != nil || n.B != nil {
			return fmt.Errorf("leaf with non-nil children")
		}
		if seen[n.Pane.ID] {
			return fmt.Errorf("duplicate PaneID %d", n.Pane.ID)
		}
		seen[n.Pane.ID] = true
	case NodeSplit:
		if n.Pane != nil {
			return fmt.Errorf("split with non-nil Pane")
		}
		if n.A == nil || n.B == nil {
			return fmt.Errorf("split missing child")
		}
		if n.A == n.B {
			return fmt.Errorf("split's two children are the same node %p", n.A)
		}
		if n.A.Parent != n {
			return fmt.Errorf("A.Parent mismatch at %p", n)
		}
		if n.B.Parent != n {
			return fmt.Errorf("B.Parent mismatch at %p", n)
		}
		if !(n.Ratio > 0 && n.Ratio < 1) {
			return fmt.Errorf("split ratio %f out of (0,1)", n.Ratio)
		}
		if err := checkNode(n.A, seen, seenNodes); err != nil {
			return err
		}
		if err := checkNode(n.B, seen, seenNodes); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown NodeKind %d", n.Kind)
	}
	return nil
}
