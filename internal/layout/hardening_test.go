package layout

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

func mkLeafPane() *session.Pane { return &session.Pane{ID: session.NewPaneID()} }

func TestClose_DetachesRemovedNodes(t *testing.T) {
	a, b := mkLeafPane(), mkLeafPane()
	root := SplitH(Leaf(a), Leaf(a), b) // a | b
	la := root.FindByID(a.ID)
	parent := la.Parent

	newRoot, removed := Close(root, la)
	if removed {
		t.Fatal("closing one of two panes should not remove the window")
	}
	// The surviving sibling becomes the new root.
	if newRoot == nil || !newRoot.IsLeaf() || newRoot.Pane.ID != b.ID {
		t.Fatalf("expected sibling b as new root, got %+v", newRoot)
	}
	// The removed leaf and its old parent are fully detached, so a stale
	// reference can't walk back into the live tree.
	if la.Parent != nil {
		t.Error("closed leaf still points at a parent")
	}
	if parent.A != nil || parent.B != nil || parent.Parent != nil {
		t.Error("collapsed parent split still wired to children")
	}
	if err := CheckInvariants(newRoot); err != nil {
		t.Fatalf("post-close invariants: %v", err)
	}
}

func TestCheckInvariants_CatchesAliasing(t *testing.T) {
	// Hand-build a malformed tree whose split aliases the same leaf into
	// both children — the kind of corruption a buggy Swap/Materialize
	// could introduce. The visited-set check must reject it.
	leaf := Leaf(mkLeafPane())
	bad := &PaneNode{Kind: NodeSplit, Ratio: 0.5, A: leaf, B: leaf}
	leaf.Parent = bad
	if err := CheckInvariants(bad); err == nil {
		t.Fatal("expected aliasing to be rejected")
	}
}

func TestChildRects_NeverNegativeOnTinyBounds(t *testing.T) {
	for total := 0; total <= 5; total++ {
		bounds := geom.NewRect(0, 0, total, 10)
		pos := splitPosFor(bounds, views.SplitVertical, 0.5)
		l, r := childRects(bounds, views.SplitVertical, pos)
		if l.Width() < 0 || r.Width() < 0 {
			t.Fatalf("total=%d: negative width (l=%d r=%d)", total, l.Width(), r.Width())
		}
		if l.B.X > bounds.B.X || r.B.X > bounds.B.X || r.A.X < bounds.A.X {
			t.Fatalf("total=%d: child escaped bounds (l=%v r=%v)", total, l, r)
		}
	}
}

// TestFocusDir_NestedGrid verifies directional focus across a 2×2 grid of
// panes (confirming the geometric matcher does the intuitive thing for a
// nested split, not just a flat row).
func TestFocusDir_NestedGrid(t *testing.T) {
	// Lay out four panes with explicit LastRects (FocusDir reads those):
	//   tl | tr      x: 0..9 (gap 10) 11..20
	//   ---+---      y: 0..4 (gap 5)  6..10
	//   bl | br
	tl, tr, bl, br := mkLeafPane(), mkLeafPane(), mkLeafPane(), mkLeafPane()
	tl.LastRect = geom.NewRect(0, 0, 10, 5)
	tr.LastRect = geom.NewRect(11, 0, 21, 5)
	bl.LastRect = geom.NewRect(0, 6, 10, 11)
	br.LastRect = geom.NewRect(11, 6, 21, 11)

	// Build a tree (shape doesn't matter to FocusDir, only LastRects do).
	left := SplitV(Leaf(tl), Leaf(tl), bl)  // tl stacked over bl
	right := SplitV(Leaf(tr), Leaf(tr), br) // tr stacked over br
	root := Split(views.SplitVertical, left, right)

	nTL := root.FindByID(tl.ID)
	nTR := root.FindByID(tr.ID)
	nBL := root.FindByID(bl.ID)
	nBR := root.FindByID(br.ID)

	check := func(from *PaneNode, dir Direction, want *PaneNode, label string) {
		got := FocusDir(root, from, dir)
		if got != want {
			t.Errorf("%s: got %v want %v", label, got, want)
		}
	}
	check(nTL, Right, nTR, "tl→right=tr")
	check(nTL, Down, nBL, "tl→down=bl")
	check(nTR, Left, nTL, "tr→left=tl")
	check(nBR, Up, nTR, "br→up=tr")
	check(nBL, Right, nBR, "bl→right=br")
	if got := FocusDir(root, nTL, Up); got != nil {
		t.Errorf("tl→up should be nil (nothing above), got %v", got)
	}
	if got := FocusDir(root, nTL, Left); got != nil {
		t.Errorf("tl→left should be nil (nothing left), got %v", got)
	}
}
