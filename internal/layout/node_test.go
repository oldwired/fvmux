package layout

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

func newTestPane() *session.Pane {
	return &session.Pane{ID: session.NewPaneID()}
}

func findLeaf(root *PaneNode, id session.PaneID) *PaneNode {
	var found *PaneNode
	root.Leaves(func(l *PaneNode) {
		if l.Pane.ID == id {
			found = l
		}
	})
	return found
}

func TestSplitHFromRoot(t *testing.T) {
	a, b := newTestPane(), newTestPane()
	root := Leaf(a)
	root = SplitH(root, root, b)
	if root.Kind != NodeSplit {
		t.Fatal("root should be a split")
	}
	if root.Orientation != views.SplitVertical {
		t.Errorf("SplitH must produce SplitVertical (panes side-by-side); got %v", root.Orientation)
	}
	if root.A.Pane.ID != a.ID || root.B.Pane.ID != b.ID {
		t.Errorf("wrong children: A=%v B=%v", root.A.Pane.ID, root.B.Pane.ID)
	}
	if err := CheckInvariants(root); err != nil {
		t.Fatal(err)
	}
}

func TestSplitVFromRoot(t *testing.T) {
	a, b := newTestPane(), newTestPane()
	root := Leaf(a)
	root = SplitV(root, root, b)
	if root.Orientation != views.SplitHorizontal {
		t.Errorf("SplitV must produce SplitHorizontal (panes stacked); got %v", root.Orientation)
	}
	if err := CheckInvariants(root); err != nil {
		t.Fatal(err)
	}
}

func TestSplitNested(t *testing.T) {
	a, b, c := newTestPane(), newTestPane(), newTestPane()
	root := Leaf(a)
	root = SplitH(root, root, b)
	leafB := findLeaf(root, b.ID)
	root = SplitV(root, leafB, c)

	bSub := findLeaf(root, b.ID).Parent
	if bSub == nil || bSub.Kind != NodeSplit || bSub.Orientation != views.SplitHorizontal {
		t.Fatalf("expected horizontal split holding b and c; got %v", bSub)
	}
	if err := CheckInvariants(root); err != nil {
		t.Fatal(err)
	}
	if got := len(root.CollectLeaves()); got != 3 {
		t.Errorf("expected 3 leaves; got %d", got)
	}
}

func TestCloseSiblingPromotes(t *testing.T) {
	a, b := newTestPane(), newTestPane()
	root := Leaf(a)
	root = SplitH(root, root, b)
	leafA := findLeaf(root, a.ID)

	newRoot, removed := Close(root, leafA)
	if removed {
		t.Fatal("not the last leaf; should not be removed")
	}
	if !newRoot.IsLeaf() || newRoot.Pane.ID != b.ID {
		t.Errorf("expected new root = leaf b; got %v", newRoot)
	}
	if newRoot.Parent != nil {
		t.Error("new root must have nil Parent")
	}
	if err := CheckInvariants(newRoot); err != nil {
		t.Fatal(err)
	}
}

func TestCloseLastLeafReturnsNil(t *testing.T) {
	a := newTestPane()
	root := Leaf(a)
	root2, removed := Close(root, root)
	if !removed {
		t.Fatal("should signal removed; was last leaf")
	}
	if root2 != nil {
		t.Errorf("expected nil root after closing last leaf; got %v", root2)
	}
}

func TestCloseNestedRewiresGrandparent(t *testing.T) {
	a, b, c := newTestPane(), newTestPane(), newTestPane()
	root := Leaf(a)
	root = SplitH(root, root, b)
	leafB := findLeaf(root, b.ID)
	root = SplitV(root, leafB, c)

	// Now root = Split(V, leaf(a), Split(H, leaf(b), leaf(c))).
	leafC := findLeaf(root, c.ID)
	newRoot, removed := Close(root, leafC)
	if removed {
		t.Fatal("expected non-removal")
	}
	if err := CheckInvariants(newRoot); err != nil {
		t.Fatalf("invariants: %v", err)
	}
	leaves := newRoot.CollectLeaves()
	if len(leaves) != 2 {
		t.Errorf("expected 2 leaves; got %d", len(leaves))
	}
}

func TestSwapExchangesPanes(t *testing.T) {
	a, b := newTestPane(), newTestPane()
	root := Leaf(a)
	root = SplitH(root, root, b)
	Swap(root.A, root.B)
	if root.A.Pane.ID != b.ID || root.B.Pane.ID != a.ID {
		t.Errorf("swap didn't exchange: A=%v B=%v", root.A.Pane.ID, root.B.Pane.ID)
	}
	if err := CheckInvariants(root); err != nil {
		t.Fatal(err)
	}
}

func TestFocusDirGeometric(t *testing.T) {
	// Layout:  +----+----+
	//          |  A |  B |
	//          +----+----+
	//          |    C    |
	//          +---------+
	a, b, c := newTestPane(), newTestPane(), newTestPane()
	a.LastRect = geom.NewRect(0, 0, 40, 20)
	b.LastRect = geom.NewRect(40, 0, 80, 20)
	c.LastRect = geom.NewRect(0, 20, 80, 40)

	root := Leaf(a)
	root = SplitH(root, root, b)
	leafA := findLeaf(root, a.ID)
	root = SplitV(root.Root(), leafA, c)
	root = root.Root()

	la := findLeaf(root, a.ID)
	lb := findLeaf(root, b.ID)
	lc := findLeaf(root, c.ID)

	if got := FocusDir(root, la, Right); got != lb {
		t.Errorf("Right from A should be B")
	}
	if got := FocusDir(root, lb, Left); got != la {
		t.Errorf("Left from B should be A")
	}
	if got := FocusDir(root, la, Down); got != lc {
		t.Errorf("Down from A should be C")
	}
	if got := FocusDir(root, lc, Up); got == nil {
		t.Errorf("Up from C should reach A or B; got nil")
	}
	if got := FocusDir(root, la, Up); got != nil {
		t.Errorf("Up from A should be nil; got %v", got)
	}
}

func TestBreakOutAndJoinFromRoundTrip(t *testing.T) {
	a, b, c := newTestPane(), newTestPane(), newTestPane()
	src := Leaf(a)
	src = SplitH(src, src, b)
	leafB := findLeaf(src, b.ID)
	src = SplitV(src, leafB, c)

	leafA := findLeaf(src, a.ID)
	newSrc, newWin := BreakOut(src, leafA)
	if err := CheckInvariants(newSrc); err != nil {
		t.Fatalf("src invariants: %v", err)
	}
	if err := CheckInvariants(newWin); err != nil {
		t.Fatalf("win invariants: %v", err)
	}
	if !newWin.IsLeaf() || newWin.Pane.ID != a.ID {
		t.Errorf("new window root should be leaf a; got %v", newWin)
	}

	dstC := findLeaf(newSrc, c.ID)
	joined, err := JoinFrom(newSrc, dstC, newWin, views.SplitVertical)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckInvariants(joined); err != nil {
		t.Fatalf("joined invariants: %v", err)
	}
	if got := len(joined.CollectLeaves()); got != 3 {
		t.Errorf("expected 3 leaves after rejoin; got %d", got)
	}
}

func TestJoinFromNonLeafRefused(t *testing.T) {
	a, b, c := newTestPane(), newTestPane(), newTestPane()
	src := Leaf(a)
	src = SplitH(src, src, b)
	dst := Leaf(c)

	_, err := JoinFrom(dst, dst, src, views.SplitVertical)
	if err != ErrJoinNonLeaf {
		t.Fatalf("expected ErrJoinNonLeaf; got %v", err)
	}
}

func TestInvariantsCatchBadRatio(t *testing.T) {
	a, b := newTestPane(), newTestPane()
	root := Leaf(a)
	root = SplitH(root, root, b)
	root.Ratio = 0
	if err := CheckInvariants(root); err == nil {
		t.Error("expected invariant violation for Ratio=0")
	}
}

func TestInvariantsCatchDuplicateID(t *testing.T) {
	id := session.NewPaneID()
	a := &session.Pane{ID: id}
	b := &session.Pane{ID: id}
	root := Leaf(a)
	root = SplitH(root, root, b)
	if err := CheckInvariants(root); err == nil {
		t.Error("expected invariant violation for duplicate PaneID")
	}
}
