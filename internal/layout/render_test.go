package layout

import (
	"math"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/session"
)

// newTermPane is newTestPane with a real (PTY-less) Terminal attached, so
// Materialize can wrap it without dereferencing a nil Term.
func newTermPane() *session.Pane {
	return &session.Pane{ID: session.NewPaneID(), Term: terminal.New(geom.Rect{})}
}

// fillGrow is the GrowMode a window body must carry: top-left pinned,
// bottom-right stretches to fill the interior on resize.
const fillGrow = consts.GfGrowHiX | consts.GfGrowHiY

// TestMaterializeBodyGrowMode locks in that both single-pane and split
// bodies are returned with the fill-and-pin-top-left GrowMode. A split
// body is a SplitGroup, which defaults to GfGrowAll — that default
// translates the whole split toward the bottom-right on resize ("glued to
// the lower-right corner"), so Materialize must override it.
func TestMaterializeBodyGrowMode(t *testing.T) {
	bounds := geom.NewRect(1, 1, 41, 21)

	single := Materialize(Leaf(newTermPane()), bounds, nil)
	if got := single.BaseView().GrowMode; got != fillGrow {
		t.Errorf("single-pane body GrowMode = %#x, want %#x", got, fillGrow)
	}

	a, b := newTermPane(), newTermPane()
	ra := Leaf(a)
	root := SplitH(ra, ra, b)
	split := Materialize(root, bounds, nil)
	if _, ok := split.(*views.SplitGroup); !ok {
		t.Fatalf("split root should materialize to *SplitGroup, got %T", split)
	}
	if got := split.BaseView().GrowMode; got != fillGrow {
		t.Errorf("split body GrowMode = %#x, want %#x (GfGrowAll=%#x glues it to the corner)",
			got, fillGrow, consts.GfGrowAll)
	}
}

// TestMaterializeSplitAnchorsOnResize is the end-to-end regression for the
// reported bug: a split body must stay anchored to the interior top-left
// and grow to fill when its container resizes, exactly like a single pane.
// The parent Group stands in for the Window — a Window resizes its body
// through the same Group.ChangeBounds / GrowMode propagation.
func TestMaterializeSplitAnchorsOnResize(t *testing.T) {
	const ox, oy = 1, 1 // interior top-left (window border is one cell)
	parent := views.NewGroup(geom.NewRect(0, 0, 40, 20))
	interior := geom.NewRect(ox, oy, 40-ox, 20-oy)

	a, b := newTermPane(), newTermPane()
	ra := Leaf(a)
	root := SplitH(ra, ra, b)
	body := Materialize(root, interior, nil)
	parent.Insert(body)

	// Grow the container; the body must keep its top-left and stretch.
	parent.ChangeBounds(geom.NewRect(0, 0, 60, 30))

	bv := body.BaseView()
	if bv.Origin.X != ox || bv.Origin.Y != oy {
		t.Errorf("body drifted from interior top-left: origin = %v, want (%d,%d) — "+
			"GfGrowAll would translate it toward the bottom-right corner", bv.Origin, ox, oy)
	}
	wantW, wantH := 60-2*ox, 30-2*oy
	if bv.Size.X != wantW || bv.Size.Y != wantH {
		t.Errorf("body did not fill grown interior: size = %v, want (%d,%d)", bv.Size, wantW, wantH)
	}

	// Shrinking must anchor the same way.
	parent.ChangeBounds(geom.NewRect(0, 0, 30, 16))
	bv = body.BaseView()
	if bv.Origin.X != ox || bv.Origin.Y != oy {
		t.Errorf("after shrink, body origin = %v, want (%d,%d)", bv.Origin, ox, oy)
	}
	if wantW, wantH := 30-2*ox, 16-2*oy; bv.Size.X != wantW || bv.Size.Y != wantH {
		t.Errorf("after shrink, body size = %v, want (%d,%d)", bv.Size, wantW, wantH)
	}
}

// A zoomed pane fills the screen visually, but LastRect remains the logical
// split geometry used by FocusDir. Replacing it with the full bounds makes the
// first directional move alter the geometry and later moves unpredictable.
func TestMaterializeZoomPreservesLogicalRects(t *testing.T) {
	bounds := geom.NewRect(1, 1, 81, 21)
	a, b := newTermPane(), newTermPane()
	leafA := Leaf(a)
	root := SplitH(leafA, leafA, b)
	zid := a.ID

	got := Materialize(root, bounds, &zid)
	if got != views.View(a.Term) {
		t.Fatalf("zoomed body = %T, want pane A terminal", got)
	}
	leafB := root.FindByID(b.ID)
	if a.LastRect == bounds {
		t.Fatalf("zoom overwrote A's logical LastRect with full bounds: %v", a.LastRect)
	}
	if !a.LastRect.Contains(geom.Point{X: 10, Y: 10}) ||
		!b.LastRect.Contains(geom.Point{X: 70, Y: 10}) {
		t.Fatalf("logical split rects not retained: A=%v B=%v", a.LastRect, b.LastRect)
	}
	if next := FocusDir(root, leafA, Right); next != leafB {
		t.Fatalf("Right from zoomed A = %v, want logical neighbour B", next)
	}
}

func dragSplitGroup(t *testing.T, sg *views.SplitGroup, delta int) {
	t.Helper()
	q := drivers.NewQueue()
	views.SetEventQueue(q)
	t.Cleanup(func() { views.SetEventQueue(nil) })
	start := sg.Splitter.Origin
	move := start
	if sg.Orientation == views.SplitVertical {
		move.X += delta
	} else {
		move.Y += delta
	}
	if !q.Put(drivers.Event{What: consts.EvMouseMove, Where: move}) ||
		!q.Put(drivers.Event{What: consts.EvMouseUp, Where: move}) {
		t.Fatal("could not queue splitter drag events")
	}
	down := drivers.Event{What: consts.EvMouseDown, Where: start}
	sg.Splitter.HandleEvent(&down)
}

// The fv-go callback must update the exact nested PaneNode so both a fresh
// materialization and the session layout encoding retain the dragged ratio.
func TestDraggedRatioSurvivesRerenderAndSerialization(t *testing.T) {
	bounds := geom.NewRect(0, 0, 100, 40)
	a, b, c := newTermPane(), newTermPane(), newTermPane()
	inner := Split(views.SplitHorizontal, Leaf(b), Leaf(c))
	root := Split(views.SplitVertical, Leaf(a), inner)
	originalRootRatio := root.Ratio

	body := Materialize(root, bounds, nil)
	rootGroup, ok := body.(*views.SplitGroup)
	if !ok {
		t.Fatalf("root body = %T, want *views.SplitGroup", body)
	}
	innerGroup, ok := rootGroup.Panel2.(*views.SplitGroup)
	if !ok {
		t.Fatalf("nested body = %T, want *views.SplitGroup", rootGroup.Panel2)
	}
	dragSplitGroup(t, innerGroup, 5)
	want := innerGroup.GetRatio()
	if math.Abs(inner.Ratio-want) > 1e-9 {
		t.Fatalf("nested PaneNode ratio = %v, want dragged ratio %v", inner.Ratio, want)
	}
	if root.Ratio != originalRootRatio {
		t.Fatalf("nested drag changed root ratio: got %v want %v", root.Ratio, originalRootRatio)
	}
	if got := b.LastRect.Height(); got != innerGroup.SplitPos {
		t.Fatalf("dragged pane hit rect height = %d, want live split position %d", got, innerGroup.SplitPos)
	}

	rerendered, ok := Materialize(root, bounds, nil).(*views.SplitGroup)
	if !ok {
		t.Fatal("rerendered root is not a SplitGroup")
	}
	rerenderedInner, ok := rerendered.Panel2.(*views.SplitGroup)
	if !ok {
		t.Fatal("rerendered nested node is not a SplitGroup")
	}
	if got := rerenderedInner.GetRatio(); math.Abs(got-want) > 1e-9 {
		t.Fatalf("rerendered nested ratio = %v, want %v", got, want)
	}

	encoded := Marshal(root)
	restored, err := Unmarshal(encoded, func(spec LeafSpec) (*session.Pane, error) {
		return &session.Pane{ID: session.NewPaneID(), Profile: spec.Profile}, nil
	})
	if err != nil {
		t.Fatalf("Unmarshal(Marshal(root)): %v", err)
	}
	if got := restored.B.Ratio; got != inner.Ratio {
		t.Fatalf("serialized nested ratio = %v, want exact %v (layout %q)", got, inner.Ratio, encoded)
	}
}
