package layout

import (
	"math"

	"github.com/oldwired/fv-go/pkg/fv/views"
)

const minResizePaneCells = 4

// ResizeToward moves the nearest divider on dir's side of leaf outward by
// cells. Thus Right grows the focused pane through a divider on its right,
// Left through one on its left, and likewise for Up/Down. The selected split's
// own rendered axis length converts cells to a persistent ratio.
func ResizeToward(leaf *PaneNode, dir Direction, cells int) bool {
	if leaf == nil || cells <= 0 {
		return false
	}
	orient := views.SplitVertical
	wantFirstChild := true
	sign := 1
	switch dir {
	case Right:
		wantFirstChild = true
		sign = 1
	case Left:
		wantFirstChild = false
		sign = -1
	case Down:
		orient = views.SplitHorizontal
		wantFirstChild = true
		sign = 1
	case Up:
		orient = views.SplitHorizontal
		wantFirstChild = false
		sign = -1
	default:
		return false
	}

	var split *PaneNode
	for child := leaf; child.Parent != nil; child = child.Parent {
		parent := child.Parent
		if parent.Orientation != orient {
			continue
		}
		if (wantFirstChild && parent.A == child) || (!wantFirstChild && parent.B == child) {
			split = parent
			break
		}
	}
	if split == nil {
		return false
	}

	total := split.RenderRect.Width()
	if orient == views.SplitHorizontal {
		total = split.RenderRect.Height()
	}
	// Both panels need four cells, plus the one-cell divider.
	minPos, maxPos := minResizePaneCells, total-minResizePaneCells-1
	if total <= 0 || maxPos < minPos {
		return false
	}
	current := splitPosFor(split.RenderRect, orient, split.Ratio)
	next := current + sign*cells
	if next < minPos {
		next = minPos
	}
	if next > maxPos {
		next = maxPos
	}
	if next == current {
		return false
	}

	ratio := float64(next) / float64(total)
	// Ensure splitPosFor's truncation lands on the requested cell even when
	// binary floating-point division/multiplication rounds just below it.
	if int(ratio*float64(total)) < next {
		ratio = math.Nextafter(ratio, 1)
	}
	split.Ratio = ratio
	return true
}
