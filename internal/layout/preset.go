package layout

import (
	"math"

	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// Preset enumerates fvmux's five built-in layouts. Ctrl-G Space cycles
// through them in declaration order.
type Preset uint8

const (
	PresetEvenHorizontal Preset = iota // panes side-by-side, equal widths.
	PresetEvenVertical                 // panes stacked, equal heights.
	PresetMainHorizontal               // big pane on top, rest below.
	PresetMainVertical                 // big pane on left, rest on right.
	PresetTiled                        // roughly-square grid.

	presetCount
)

// PresetCount is the number of distinct presets.
const PresetCount = int(presetCount)

// Name returns a short human label.
func (p Preset) Name() string {
	switch p {
	case PresetEvenHorizontal:
		return "even-horizontal"
	case PresetEvenVertical:
		return "even-vertical"
	case PresetMainHorizontal:
		return "main-horizontal"
	case PresetMainVertical:
		return "main-vertical"
	case PresetTiled:
		return "tiled"
	}
	return "?"
}

// ApplyPreset rebuilds a fresh layout tree containing exactly the given
// panes (in order) arranged per the preset. The caller materialises +
// rerenders.
func ApplyPreset(p Preset, panes []*session.Pane) *PaneNode {
	switch len(panes) {
	case 0:
		return nil
	case 1:
		return Leaf(panes[0])
	}
	switch p {
	case PresetEvenHorizontal:
		return evenChain(panesToLeaves(panes), views.SplitVertical)
	case PresetEvenVertical:
		return evenChain(panesToLeaves(panes), views.SplitHorizontal)
	case PresetMainHorizontal:
		main := Leaf(panes[0])
		rest := evenChain(panesToLeaves(panes[1:]), views.SplitVertical)
		root := Split(views.SplitHorizontal, main, rest)
		root.Ratio = 0.6
		return root
	case PresetMainVertical:
		main := Leaf(panes[0])
		rest := evenChain(panesToLeaves(panes[1:]), views.SplitHorizontal)
		root := Split(views.SplitVertical, main, rest)
		root.Ratio = 0.6
		return root
	case PresetTiled:
		return tile(panes)
	}
	return nil
}

func panesToLeaves(panes []*session.Pane) []*PaneNode {
	out := make([]*PaneNode, len(panes))
	for i, p := range panes {
		out[i] = Leaf(p)
	}
	return out
}

// evenChain folds nodes into a tree along orient with ratios that
// distribute area evenly: at step i (0-indexed of additions), the
// added child gets 1/(i+2) of the remaining space.
func evenChain(nodes []*PaneNode, orient views.SplitOrientation) *PaneNode {
	if len(nodes) == 0 {
		return nil
	}
	root := nodes[0]
	for i := 1; i < len(nodes); i++ {
		n := Split(orient, root, nodes[i])
		// Left/top subtree keeps i panes; right/bottom adds 1 new.
		// Even split: existing subtree takes i/(i+1) of the space.
		n.Ratio = float64(i) / float64(i+1)
		root = n
	}
	return root
}

// tile builds a roughly-square grid: ceil(sqrt(N)) columns × enough rows.
// Each row is an evenChain along the row axis; the rows are stacked.
func tile(panes []*session.Pane) *PaneNode {
	n := len(panes)
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	if cols < 1 {
		cols = 1
	}
	rowsTrees := make([]*PaneNode, 0, (n+cols-1)/cols)
	for i := 0; i < n; i += cols {
		end := i + cols
		if end > n {
			end = n
		}
		rowsTrees = append(rowsTrees, evenChain(panesToLeaves(panes[i:end]), views.SplitVertical))
	}
	return evenChain(rowsTrees, views.SplitHorizontal)
}
