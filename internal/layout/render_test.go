package layout

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
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
