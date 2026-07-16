package sftp

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"
)

// Residual #1 regression tests: expanding a collapsed remote folder must
// not read the directory on the UI event goroutine. expandRemoteAsync
// attaches a "Loading…" placeholder synchronously and does the ReadDir on
// a worker, delivering the real children back via views.CallSoon.

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// newRemoteTreePanel wires a remote panel around a folder tree rooted at
// cwd, with expandRemoteAsync as OnExpand — exactly as newPanel does.
func newRemoteTreePanel(t *testing.T, root string) (*panel, *atomic.Bool, *sync.WaitGroup) {
	t.Helper()
	c := newTestClient(t)
	var closed atomic.Bool
	var wg sync.WaitGroup
	p := &panel{isRemote: true, c: c, cwd: root, closed: &closed, refreshWG: &wg}
	p.tree = treeview.New(geom.NewRect(0, 0, 24, 12), buildRemoteTree(c, root))
	p.tree.OnExpand = func(n *treeview.Node) { p.expandRemoteAsync(n) }
	return p, &closed, &wg
}

func rootChildLabels(n *treeview.Node) []string {
	out := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		out = append(out, c.Label)
	}
	return out
}

func findByLabel(roots []*treeview.Node, label string) *treeview.Node {
	for _, r := range roots {
		if r.Label == label {
			return r
		}
	}
	return nil
}

// TestExpandRemoteAsync_PlaceholderThenChildren proves the placeholder is
// attached synchronously (so the row expands at once) and is replaced by
// the real folders — folders only, sorted — after the CallSoon delivers.
func TestExpandRemoteAsync_PlaceholderThenChildren(t *testing.T) {
	scheduled := make(chan func(), 4)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)

	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	mkdirAll(t, filepath.Join(sub, "alpha"))
	mkdirAll(t, filepath.Join(sub, "beta"))
	// A plain file inside sub must NOT show up: the tree is folders-only.
	if err := os.WriteFile(filepath.Join(sub, "afile.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, _, wg := newRemoteTreePanel(t, root)
	subNode := findByLabel(p.tree.Roots, "sub/")
	if subNode == nil {
		t.Fatalf("sub node missing; roots=%v", rootChildLabels(&treeview.Node{Children: p.tree.Roots}))
	}

	p.expandRemoteAsync(subNode)

	// Synchronous placeholder — no network read happened on this goroutine.
	if got := rootChildLabels(subNode); len(got) != 1 || got[0] != "Loading…" {
		t.Fatalf("expected a single Loading… placeholder, got %v", got)
	}

	// The worker ran buildRemoteTree off-thread and queued its CallSoon.
	deliver := waitDeliver(t, scheduled)
	deliver()
	wg.Wait()

	got := rootChildLabels(subNode)
	want := []string{"alpha/", "beta/"}
	if len(got) != len(want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children = %v, want %v", got, want)
		}
	}
	for _, ch := range subNode.Children {
		if ch.Parent != subNode {
			t.Errorf("child %q not reparented onto sub", ch.Label)
		}
	}
}

// TestExpandRemoteAsync_NoDoubleSchedule: a second expand while the first
// is still pending (placeholder present) must not kick off a second read.
func TestExpandRemoteAsync_NoDoubleSchedule(t *testing.T) {
	scheduled := make(chan func(), 4)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "sub", "alpha"))

	p, _, wg := newRemoteTreePanel(t, root)
	subNode := findByLabel(p.tree.Roots, "sub/")

	p.expandRemoteAsync(subNode) // schedules the one and only read.
	p.expandRemoteAsync(subNode) // placeholder present ⇒ must be a no-op.

	deliver := waitDeliver(t, scheduled)
	deliver()
	wg.Wait()

	select {
	case <-scheduled:
		t.Fatal("a second read was scheduled — double-expand was not guarded")
	case <-time.After(150 * time.Millisecond):
	}
}

// TestExpandRemoteAsync_ClosedGuards: once the browser is tearing down,
// expand must neither attach a placeholder nor start a worker; and a
// result arriving after teardown must be dropped rather than mutate the
// node.
func TestExpandRemoteAsync_ClosedGuards(t *testing.T) {
	scheduled := make(chan func(), 4)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "sub", "alpha"))

	p, closed, wg := newRemoteTreePanel(t, root)
	subNode := findByLabel(p.tree.Roots, "sub/")

	// (a) closed before expand: no placeholder, no worker.
	closed.Store(true)
	p.expandRemoteAsync(subNode)
	if len(subNode.Children) != 0 {
		t.Fatalf("closed panel attached children: %v", rootChildLabels(subNode))
	}
	select {
	case <-scheduled:
		t.Fatal("closed panel scheduled a read")
	case <-time.After(100 * time.Millisecond):
	}

	// (b) closed mid-fetch: the delivered result is dropped.
	closed.Store(false)
	p.expandRemoteAsync(subNode) // placeholder + worker.
	deliver := waitDeliver(t, scheduled)
	closed.Store(true) // browser tears down before the result lands.
	deliver()
	wg.Wait()
	if got := rootChildLabels(subNode); len(got) != 1 || got[0] != "Loading…" {
		t.Fatalf("result should be dropped when closed mid-fetch; children=%v", got)
	}
}

// TestFlatIndexOf mirrors treeview's visible-row ordering (pre-order,
// honoring Expanded) so rebuildTreePreservingFocus can restore the cursor.
func TestFlatIndexOf(t *testing.T) {
	a2a := &treeview.Node{Label: "A2a"}
	a2 := &treeview.Node{Label: "A2", Expanded: true, Children: []*treeview.Node{a2a}}
	a1 := &treeview.Node{Label: "A1"}
	a := &treeview.Node{Label: "A", Expanded: true, Children: []*treeview.Node{a1, a2}}
	b := &treeview.Node{Label: "B"}
	roots := []*treeview.Node{a, b}
	// Flattened: A(0) A1(1) A2(2) A2a(3) B(4).
	cases := []struct {
		node *treeview.Node
		want int
	}{
		{a, 0}, {a1, 1}, {a2, 2}, {a2a, 3}, {b, 4},
		{&treeview.Node{Label: "orphan"}, -1},
	}
	for _, c := range cases {
		if got := flatIndexOf(roots, c.node); got != c.want {
			t.Errorf("flatIndexOf(%q) = %d, want %d", c.node.Label, got, c.want)
		}
	}

	// A collapsed ancestor hides its descendants entirely.
	a.Expanded = false
	if got := flatIndexOf(roots, a2a); got != -1 {
		t.Errorf("hidden descendant reachable: got %d, want -1", got)
	}
	if got := flatIndexOf(roots, b); got != 1 {
		t.Errorf("B after collapsing A: got %d, want 1", got)
	}
}

// TestRebuildTreePreservingFocus: mutating a node's children off-thread
// and rebuilding must keep the cursor on the node it was on, even as rows
// above it shift.
func TestRebuildTreePreservingFocus(t *testing.T) {
	views.SetCallSoon(nil)
	a2a := &treeview.Node{Label: "A2a"}
	a2 := &treeview.Node{Label: "A2", Expanded: true, HasChildren: true, Children: []*treeview.Node{a2a}}
	a := &treeview.Node{Label: "A", Expanded: true, HasChildren: true, Children: []*treeview.Node{a2}}
	b := &treeview.Node{Label: "B"}

	p := &panel{isRemote: true}
	p.tree = treeview.New(geom.NewRect(0, 0, 24, 12), []*treeview.Node{a, b})
	// Flattened: A(0) A2(1) A2a(2) B(3). Put the cursor on B.
	p.tree.Focused = 3
	if p.tree.CurrentNode() != b {
		t.Fatalf("precondition: cursor should be on B, got %v", p.tree.CurrentNode())
	}

	// Grow A2a with a child (as an async expand would), pushing B down a row.
	a2a.Expanded = true
	a2a.HasChildren = true
	a2a.Children = []*treeview.Node{{Label: "A2a-1"}}

	p.rebuildTreePreservingFocus()

	if p.tree.CurrentNode() != b {
		t.Fatalf("cursor drifted off B after rebuild: got %v", p.tree.CurrentNode())
	}
}

// waitDeliver receives one scheduled CallSoon closure or fails the test.
func waitDeliver(t *testing.T, ch <-chan func()) func() {
	t.Helper()
	select {
	case fn := <-ch:
		return fn
	case <-time.After(2 * time.Second):
		t.Fatal("expected a CallSoon delivery that never came")
		return nil
	}
}
