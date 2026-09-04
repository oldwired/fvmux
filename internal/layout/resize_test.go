package layout

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func nodeSplitPos(n *PaneNode) int {
	return splitPosFor(n.RenderRect, n.Orientation, n.Ratio)
}

func TestResizeTowardMovesRootDividerByExactCellsFromBothSides(t *testing.T) {
	tests := []struct {
		name   string
		orient views.SplitOrientation
		dir    Direction
		second bool
		cells  int
		sign   int
	}{
		{name: "vertical first right one", orient: views.SplitVertical, dir: Right, cells: 1, sign: 1},
		{name: "vertical second left five", orient: views.SplitVertical, dir: Left, second: true, cells: 5, sign: -1},
		{name: "horizontal first down one", orient: views.SplitHorizontal, dir: Down, cells: 1, sign: 1},
		{name: "horizontal second up five", orient: views.SplitHorizontal, dir: Up, second: true, cells: 5, sign: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := newTermPane(), newTermPane()
			leafA, leafB := Leaf(a), Leaf(b)
			root := Split(tt.orient, leafA, leafB)
			Materialize(root, geom.NewRect(0, 0, 80, 40), nil)
			before := nodeSplitPos(root)
			focus := leafA
			if tt.second {
				focus = leafB
			}
			if !ResizeToward(focus, tt.dir, tt.cells) {
				t.Fatal("ResizeToward reported no movement")
			}
			if got, want := nodeSplitPos(root), before+tt.sign*tt.cells; got != want {
				t.Fatalf("divider moved from %d to %d, want %d", before, got, want)
			}
			Materialize(root, geom.NewRect(0, 0, 80, 40), nil)
			if got, want := nodeSplitPos(root), before+tt.sign*tt.cells; got != want {
				t.Fatalf("rerendered divider = %d, want %d", got, want)
			}
		})
	}
}

func TestResizeTowardUsesNestedSplitOwnAxis(t *testing.T) {
	tests := []struct {
		name   string
		orient views.SplitOrientation
		dir    Direction
		second bool
		cells  int
		sign   int
	}{
		{name: "nested vertical first", orient: views.SplitVertical, dir: Right, cells: 1, sign: 1},
		{name: "nested vertical second", orient: views.SplitVertical, dir: Left, second: true, cells: 5, sign: -1},
		{name: "nested horizontal first", orient: views.SplitHorizontal, dir: Down, cells: 1, sign: 1},
		{name: "nested horizontal second", orient: views.SplitHorizontal, dir: Up, second: true, cells: 5, sign: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outerPane := newTermPane()
			a, b := newTermPane(), newTermPane()
			leafA, leafB := Leaf(a), Leaf(b)
			inner := Split(tt.orient, leafA, leafB)
			// The outer split uses the same orientation, making it essential to
			// select the requested-side nearest boundary rather than its ancestor.
			root := Split(tt.orient, Leaf(outerPane), inner)
			Materialize(root, geom.NewRect(0, 0, 100, 50), nil)
			rootRatio := root.Ratio
			before := nodeSplitPos(inner)
			focus := leafA
			if tt.second {
				focus = leafB
			}
			if !ResizeToward(focus, tt.dir, tt.cells) {
				t.Fatal("ResizeToward reported no movement")
			}
			if got, want := nodeSplitPos(inner), before+tt.sign*tt.cells; got != want {
				t.Fatalf("nested divider moved from %d to %d, want %d", before, got, want)
			}
			if root.Ratio != rootRatio {
				t.Fatalf("nested resize changed ancestor ratio from %v to %v", rootRatio, root.Ratio)
			}
		})
	}
}

func TestResizeTowardClampsAndRejectsMissingBoundary(t *testing.T) {
	a, b := newTermPane(), newTermPane()
	leafA, leafB := Leaf(a), Leaf(b)
	root := Split(views.SplitVertical, leafA, leafB)
	root.Ratio = float64(minResizePaneCells) / 40
	Materialize(root, geom.NewRect(0, 0, 40, 20), nil)
	before := nodeSplitPos(root)
	if ResizeToward(leafB, Left, 5) {
		t.Fatal("resize moved divider past Panel1 minimum")
	}
	if got := nodeSplitPos(root); got != before {
		t.Fatalf("clamped divider changed from %d to %d", before, got)
	}
	if ResizeToward(leafA, Left, 1) {
		t.Fatal("leftmost pane found a nonexistent divider on its left")
	}
	if ResizeToward(leafB, Right, 1) {
		t.Fatal("rightmost pane found a nonexistent divider on its right")
	}
}
