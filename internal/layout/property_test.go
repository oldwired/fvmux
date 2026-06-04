package layout

import (
	"math/rand/v2"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

// TestPropertyRandomOps runs 100k random algebra ops against a tree (plus
// a set of broken-out single-leaf windows), asserting after every step
// that:
//
//   - CheckInvariants holds on the main tree AND every detached window
//     (so the new no-aliasing / distinct-children checks are exercised),
//   - the set of live PaneIDs in the main tree exactly matches an
//     independently-tracked expected set (catches a Close/BreakOut that
//     drops or duplicates a pane, or a Swap that corrupts the tree).
//
// The op mix now includes BreakOut and JoinFrom — the two operations most
// likely to leave a dangling parent pointer or alias a pane into two
// trees. Failure prints the op index and seed so the sequence replays.
func TestPropertyRandomOps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping property test in -short mode")
	}

	const seed1, seed2 uint64 = 0x5fb1d6dbbc1a8a89, 0xa3d70a3d70a3d70a
	const n = 100_000
	const maxLeaves = 50
	const maxDetached = 16

	rng := rand.New(rand.NewPCG(seed1, seed2))

	mkPane := func() *session.Pane {
		return &session.Pane{ID: session.NewPaneID(), LastRect: geom.NewRect(0, 0, 80, 24)}
	}

	root := Leaf(mkPane())
	mainIDs := map[session.PaneID]bool{root.Pane.ID: true}
	var detached []*PaneNode // each a single-leaf broken-out window.

	resetRoot := func() {
		p := mkPane()
		root = Leaf(p)
		mainIDs = map[session.PaneID]bool{p.ID: true}
	}

	for i := 0; i < n; i++ {
		leaves := root.CollectLeaves()
		if len(leaves) == 0 {
			resetRoot()
			leaves = root.CollectLeaves()
		}
		target := leaves[rng.IntN(len(leaves))]

		shouldGrow := len(leaves) < maxLeaves && (len(leaves) <= 2 || rng.IntN(3) != 0)
		shouldShrink := len(leaves) >= maxLeaves || (len(leaves) > 1 && rng.IntN(5) == 0)

		switch {
		case len(detached) > 0 && rng.IntN(6) == 0 && len(leaves) < maxLeaves:
			// JoinFrom: merge a detached single-leaf window back in.
			src := detached[len(detached)-1]
			detached = detached[:len(detached)-1]
			orient := views.SplitVertical
			if rng.IntN(2) == 0 {
				orient = views.SplitHorizontal
			}
			r, err := JoinFrom(root, target, src, orient)
			if err != nil {
				t.Fatalf("op %d JoinFrom: %v", i, err)
			}
			root = r
			mainIDs[src.Pane.ID] = true

		case shouldShrink && len(leaves) > 1 && len(detached) < maxDetached && rng.IntN(4) == 0:
			// BreakOut: detach target's pane into its own window.
			id := target.Pane.ID
			var newWin *PaneNode
			root, newWin = BreakOut(root, target)
			delete(mainIDs, id)
			detached = append(detached, newWin)

		case shouldShrink:
			id := target.Pane.ID
			var removed bool
			root, removed = Close(root, target)
			delete(mainIDs, id)
			if removed {
				resetRoot()
			}

		case shouldGrow:
			np := mkPane()
			if rng.IntN(2) == 0 {
				root = SplitH(root, target, np)
			} else {
				root = SplitV(root, target, np)
			}
			mainIDs[np.ID] = true

		default:
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
			t.Fatalf("op %d (seed=%x,%x): main tree: %v", i, seed1, seed2, err)
		}
		for di, d := range detached {
			if err := CheckInvariants(d); err != nil {
				t.Fatalf("op %d (seed=%x,%x): detached[%d]: %v", i, seed1, seed2, di, err)
			}
		}

		// Oracle: the main tree's live PaneIDs match the expected set.
		got := map[session.PaneID]bool{}
		for _, l := range root.CollectLeaves() {
			got[l.Pane.ID] = true
		}
		if len(got) != len(mainIDs) {
			t.Fatalf("op %d: main id-set size drift: got %d want %d", i, len(got), len(mainIDs))
		}
		for id := range mainIDs {
			if !got[id] {
				t.Fatalf("op %d: main tree missing expected pane %d", i, id)
			}
		}
	}
}
