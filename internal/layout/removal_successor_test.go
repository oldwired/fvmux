package layout

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

func namedLeaf(name string) *PaneNode {
	return Leaf(&session.Pane{ID: session.NewPaneID(), Title: name})
}

func simpleRemovalTree() (*PaneNode, map[string]*PaneNode) {
	a, b := namedLeaf("A"), namedLeaf("B")
	root := Split(views.SplitVertical, a, b)
	updateLeafRects(root, geom.NewRect(0, 0, 120, 60))
	return root, map[string]*PaneNode{"A": a, "B": b}
}

func nestedThreeRemovalTree() (*PaneNode, map[string]*PaneNode) {
	a, b, c := namedLeaf("A"), namedLeaf("B"), namedLeaf("C")
	inner := Split(views.SplitHorizontal, b, c)
	root := Split(views.SplitVertical, a, inner)
	updateLeafRects(root, geom.NewRect(0, 0, 120, 60))
	return root, map[string]*PaneNode{"A": a, "B": b, "C": c}
}

func nestedFourRemovalTree() (*PaneNode, map[string]*PaneNode) {
	a, b := namedLeaf("A"), namedLeaf("B")
	c, d := namedLeaf("C"), namedLeaf("D")
	cd := Split(views.SplitHorizontal, c, d)
	right := Split(views.SplitVertical, b, cd)
	root := Split(views.SplitVertical, a, right)
	updateLeafRects(root, geom.NewRect(0, 0, 120, 60))
	return root, map[string]*PaneNode{"A": a, "B": b, "C": c, "D": d}
}

func TestRemovalSuccessorForEverySimpleAndNestedPane(t *testing.T) {
	tests := []struct {
		name  string
		build func() (*PaneNode, map[string]*PaneNode)
		want  map[string]string
	}{
		{
			name:  "simple two-pane",
			build: simpleRemovalTree,
			want:  map[string]string{"A": "B", "B": "A"},
		},
		{
			name:  "nested three-pane",
			build: nestedThreeRemovalTree,
			want:  map[string]string{"A": "B", "B": "C", "C": "B"},
		},
		{
			name:  "nested four-pane",
			build: nestedFourRemovalTree,
			want:  map[string]string{"A": "B", "B": "C", "C": "D", "D": "C"},
		},
	}

	for _, tt := range tests {
		for removed, wanted := range tt.want {
			t.Run(tt.name+"/close "+removed, func(t *testing.T) {
				root, leaves := tt.build()
				target := leaves[removed]
				successor := RemovalSuccessor(target)
				if successor != leaves[wanted] {
					t.Fatalf("RemovalSuccessor(%s) = %v, want %s", removed, successor, wanted)
				}
				newRoot, removedWindow := Close(root, target)
				if removedWindow {
					t.Fatal("multi-pane close unexpectedly removed the window")
				}
				if newRoot.FindByID(successor.Pane.ID) != successor {
					t.Fatal("chosen successor did not survive Close")
				}
				if err := CheckInvariants(newRoot); err != nil {
					t.Fatalf("invariants after close: %v", err)
				}
			})
		}
	}
}
