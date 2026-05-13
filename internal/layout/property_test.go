package layout

import (
	"math/rand/v2"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"

	"github.com/oldwired/fvmux/internal/session"
)

// TestPropertyRandomOps runs 100k random algebra ops against a tree,
// asserting CheckInvariants after every step. The op mix is biased
// toward keeping the tree's leaf count in [1, maxLeaves] so each
// CheckInvariants call stays O(maxLeaves) and the whole test finishes
// in a couple of seconds. Failure prints the op index and seed so the
// sequence can be replayed.
func TestPropertyRandomOps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in -short mode")
	}

	const seed1, seed2 uint64 = 0x5fb1d6dbbc1a8a89, 0xa3d70a3d70a3d70a
	const n = 100_000
	const maxLeaves = 50

	rng := rand.New(rand.NewPCG(seed1, seed2))

	mkPane := func() *session.Pane {
		return &session.Pane{ID: session.NewPaneID(), LastRect: geom.NewRect(0, 0, 80, 24)}
	}

	root := Leaf(mkPane())

	for i := 0; i < n; i++ {
		leaves := root.CollectLeaves()
		if len(leaves) == 0 {
			root = Leaf(mkPane())
			continue
		}
		target := leaves[rng.IntN(len(leaves))]

		// Bias the op so the tree stays in [1, maxLeaves]: more splits
		// when small, more closes when large.
		shouldGrow := len(leaves) < maxLeaves && (len(leaves) <= 2 || rng.IntN(3) != 0)
		shouldShrink := len(leaves) >= maxLeaves || (len(leaves) > 1 && rng.IntN(5) == 0)

		switch {
		case shouldShrink:
			var removed bool
			root, removed = Close(root, target)
			if removed {
				root = Leaf(mkPane())
			}
		case shouldGrow:
			if rng.IntN(2) == 0 {
				root = SplitH(root, target, mkPane())
			} else {
				root = SplitV(root, target, mkPane())
			}
		default:
			// Either swap two leaves or tweak a ratio.
			if rng.IntN(2) == 0 && len(leaves) >= 2 {
				other := leaves[rng.IntN(len(leaves))]
				if other != target {
					Swap(target, other)
				}
			} else if target.Parent != nil {
				target.Parent.Ratio = 0.05 + 0.9*rng.Float64()
			}
		}

		if err := CheckInvariants(root); err != nil {
			t.Fatalf("op %d (seed=%x,%x): %v", i, seed1, seed2, err)
		}
	}
}
