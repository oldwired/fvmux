package layout

import (
	"math"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// Direction names the four cardinal axes used by FocusDir.
type Direction uint8

const (
	Left Direction = iota
	Right
	Up
	Down
)

// FocusDir returns the leaf in root nearest to current along dir.
//
// Geometric rules (matches user intuition across nested splits):
//
//   - For each candidate leaf, classify by rectangle edges. "Right"
//     candidates have their left edge ≥ current's right edge AND their
//     Y interval overlaps current's Y interval. Analogous rules for
//     Left/Up/Down with axes swapped.
//   - Among candidates, pick the smallest *edge-to-edge gap* along the
//     direction axis. Ties go to the smallest perpendicular offset
//     between the leaves' centres.
//
// Returns nil if no candidate exists (e.g., "Up" from the topmost leaf).
func FocusDir(root, current *PaneNode, dir Direction) *PaneNode {
	if current == nil || current.Pane == nil {
		return nil
	}
	from := current.Pane.LastRect
	var best *PaneNode
	bestPrimary := math.MaxInt
	bestSecondary := math.MaxInt

	root.Leaves(func(l *PaneNode) {
		if l == current || l.Pane == nil {
			return
		}
		primary, secondary, ok := scoreFor(from, l.Pane.LastRect, dir)
		if !ok {
			return
		}
		if primary < bestPrimary || (primary == bestPrimary && secondary < bestSecondary) {
			best = l
			bestPrimary = primary
			bestSecondary = secondary
		}
	})
	return best
}

func scoreFor(from, to geom.Rect, dir Direction) (primary, secondary int, ok bool) {
	fromCx := (from.A.X + from.B.X) / 2
	fromCy := (from.A.Y + from.B.Y) / 2
	toCx := (to.A.X + to.B.X) / 2
	toCy := (to.A.Y + to.B.Y) / 2

	switch dir {
	case Right:
		if to.A.X < from.B.X {
			return 0, 0, false
		}
		if to.B.Y <= from.A.Y || to.A.Y >= from.B.Y {
			return 0, 0, false
		}
		return to.A.X - from.B.X, absInt(toCy - fromCy), true
	case Left:
		if to.B.X > from.A.X {
			return 0, 0, false
		}
		if to.B.Y <= from.A.Y || to.A.Y >= from.B.Y {
			return 0, 0, false
		}
		return from.A.X - to.B.X, absInt(toCy - fromCy), true
	case Down:
		if to.A.Y < from.B.Y {
			return 0, 0, false
		}
		if to.B.X <= from.A.X || to.A.X >= from.B.X {
			return 0, 0, false
		}
		return to.A.Y - from.B.Y, absInt(toCx - fromCx), true
	case Up:
		if to.B.Y > from.A.Y {
			return 0, 0, false
		}
		if to.B.X <= from.A.X || to.A.X >= from.B.X {
			return 0, 0, false
		}
		return from.A.Y - to.B.Y, absInt(toCx - fromCx), true
	}
	return 0, 0, false
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
