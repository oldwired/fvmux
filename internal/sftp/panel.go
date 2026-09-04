// Per-side state for the dual-pane browser. Each panel owns a tree
// (folders only), a listing (flat cwd contents), and its current
// folder. Trees drive listings on selection; listings drive previews
// (for files) and panel cwd (for folders, via Enter).
package sftp

import (
	"sync"
	"sync/atomic"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"

	"github.com/oldwired/fvmux/internal/ui"
)

// panel bundles tree + listing + cwd for one side (remote or local).
// preview is shared between both panels — set after construction by
// the dialog assembly code.
type panel struct {
	isRemote bool
	c        *pkgsftp.Client // nil for local panels.

	cwd string // current folder driving the listing.

	tree    *treeview.TreeView
	listing *treeview.TreeView

	header *dialogs.StaticText // "Remote — /path" or "Local — /path"
	width  int                 // header column width, for path truncation

	preview *previewPane

	// Async remote-listing refresh coordination (remote panels only).
	// closed and refreshWG are shared with the owning browser so its
	// teardown can stop new reads and wait for in-flight ones to drain
	// before closing the SFTP client. refMu guards the single-worker
	// latest-request-wins state below.
	closed    *atomic.Bool
	refreshWG *sync.WaitGroup

	refMu      sync.Mutex
	refRunning bool
	refPending bool
	refWantCwd string

	// opMu/pendingOps suppress repeated remote mutations against the same
	// path while their first network round-trip is still running.
	opMu       sync.Mutex
	pendingOps map[string]struct{}
}

// newPanel builds tree + listing for one side, inserts them into d,
// and wires OnExpand / OnSelect. treeRoots / listingRoots are the
// initial folder tree and cwd listing — the caller supplies them so the
// remote panel's first network reads happen off the UI goroutine (in
// the connect phase), not inside this constructor. The listing's
// TreeView holds only leaf nodes (HasChildren = false on every entry) so
// Enter never triggers an inline expansion; the dialog-level key handler
// intercepts Enter first and routes it through listingEnter.
func newPanel(
	d *dialogs.Dialog,
	isRemote bool,
	c *pkgsftp.Client,
	cwd string,
	header *dialogs.StaticText,
	headerW int,
	treeBounds, listingBounds geom.Rect,
	treeRoots, listingRoots []*treeview.Node,
) *panel {
	p := &panel{
		isRemote: isRemote,
		c:        c,
		cwd:      cwd,
		header:   header,
		width:    headerW,
	}

	p.tree = treeview.New(treeBounds, treeRoots)
	p.listing = treeview.New(listingBounds, listingRoots)
	if isRemote {
		// Remote expansion is async: a synchronous ReadDir on the UI
		// event goroutine would freeze every window while a slow or dead
		// link answers. expandRemoteAsync attaches a "Loading…" placeholder
		// and schedules the read off-thread.
		p.tree.OnExpand = func(n *treeview.Node) { p.expandRemoteAsync(n) }
	} else {
		p.tree.OnExpand = func(n *treeview.Node) { expandLocalTree(n) }
	}

	// Tree drives listing: highlighting a folder in the tree sets
	// that side's cwd and rebuilds the listing.
	p.tree.OnSelect = func(n *treeview.Node) { p.onTreeSelect(n) }
	// Listing OnSelect is intentionally NOT wired to preview. Auto-
	// previewing on arrow-nav would re-fetch every remote file the
	// cursor crosses — bad UX on a slow link. Enter on a file row
	// is the explicit "open it" action; see listingEnter below.

	d.Insert(p.tree)
	d.Insert(p.listing)
	return p
}

// setCwd updates this panel's current folder, rebuilds the listing,
// and refreshes the header. Tree expansion state is left intact —
// the user keeps whatever they had unfolded. The header updates
// immediately for instant feedback; the remote listing arrives a beat
// later via the async refresh (so a slow link can't freeze the UI).
func (p *panel) setCwd(newCwd string) {
	if newCwd == "" || newCwd == p.cwd {
		return
	}
	p.cwd = newCwd
	p.refreshHeader()
	if p.isRemote {
		p.requestRemoteRefresh(newCwd)
	} else {
		p.listing.SetRoots(buildLocalListing(newCwd))
	}
	views.MarkDirty()
}

// refresh re-reads the panel's current folder and rebuilds the listing
// in place. Called after a transfer completes against this side, and
// from the manual Refresh button / Ctrl-R hotkey. Header/cwd are
// unchanged; tree expansion state is preserved. The remote read runs
// off the UI goroutine; local reads stay synchronous (the local FS
// doesn't block, and async would only add stale-ordering risk).
func (p *panel) refresh() {
	if p.listing == nil {
		return
	}
	if p.isRemote {
		p.requestRemoteRefresh(p.cwd)
	} else {
		p.listing.SetRoots(buildLocalListing(p.cwd))
		views.MarkDirty()
	}
}

// asyncRemoteOp runs op off the UI goroutine and calls done back on it
// (via views.CallSoon) with op's error. Every remote mutation (F7
// mkdir, F8 delete, F6 move/rename, F5 tree walks and dedicated-session
// opens) must go through here: a synchronous SFTP round-trip on the UI
// event goroutine freezes the entire multiplexer for the length of a
// slow operation or dead-link TCP timeout. Tracked by p.refreshWG so
// the browser's teardown drains in-flight ops before closing the shared
// client; a no-op once the browser is closing, and done is skipped if
// it closed while op ran.
func (p *panel) asyncRemoteOp(op func() error, done func(error)) bool {
	if p.closed != nil && p.closed.Load() {
		return false
	}
	if p.refreshWG != nil {
		p.refreshWG.Add(1)
	}
	go func() {
		if p.refreshWG != nil {
			defer p.refreshWG.Done()
		}
		err := op()
		views.CallSoon(func() {
			if p.closed != nil && p.closed.Load() {
				return
			}
			if done != nil {
				done(err)
			}
		})
	}()
	return true
}

// asyncRemoteOpKey is asyncRemoteOp with an operation-level guard. It
// returns false when the same key is already running or teardown has begun.
// The key remains claimed through the UI completion callback, so a repeated
// F-key cannot race the first mutation before its listing refresh lands.
func (p *panel) asyncRemoteOpKey(key string, op func() error, done func(error)) bool {
	if key == "" {
		return p.asyncRemoteOp(op, done)
	}
	p.opMu.Lock()
	if p.pendingOps == nil {
		p.pendingOps = make(map[string]struct{})
	}
	if _, exists := p.pendingOps[key]; exists {
		p.opMu.Unlock()
		return false
	}
	p.pendingOps[key] = struct{}{}
	p.opMu.Unlock()

	started := p.asyncRemoteOp(op, func(err error) {
		p.opMu.Lock()
		delete(p.pendingOps, key)
		p.opMu.Unlock()
		if done != nil {
			done(err)
		}
	})
	if !started {
		p.opMu.Lock()
		delete(p.pendingOps, key)
		p.opMu.Unlock()
	}
	return started
}

// requestRemoteRefresh schedules an off-thread reload of cwd's remote
// listing. At most one network read runs at a time; a request arriving
// while one is in flight records the newest cwd and reruns when the
// current read finishes (latest-request-wins), so rapid navigation never
// strands the listing on a stale folder. A no-op once the browser is
// closing.
func (p *panel) requestRemoteRefresh(cwd string) {
	if p.closed != nil && p.closed.Load() {
		return
	}
	p.refMu.Lock()
	p.refWantCwd = cwd
	if p.refRunning {
		p.refPending = true
		p.refMu.Unlock()
		return
	}
	p.refRunning = true
	p.refMu.Unlock()

	if p.refreshWG != nil {
		p.refreshWG.Add(1) // gates the browser's client.Close on teardown.
	}
	go p.remoteRefreshLoop()
}

func (p *panel) remoteRefreshLoop() {
	if p.refreshWG != nil {
		defer p.refreshWG.Done()
	}
	for {
		if p.closed != nil && p.closed.Load() {
			p.refMu.Lock()
			p.refRunning = false
			p.refMu.Unlock()
			return
		}
		p.refMu.Lock()
		cwd := p.refWantCwd
		p.refPending = false
		p.refMu.Unlock()

		roots := buildRemoteListing(p.c, cwd) // network read, off the UI goroutine.
		views.CallSoon(func() {
			// Drop the result if the browser closed or the user navigated
			// away while the read was in flight.
			if p.closed != nil && p.closed.Load() {
				return
			}
			if p.cwd != cwd {
				return
			}
			p.listing.SetRoots(roots)
			views.MarkDirty()
		})

		p.refMu.Lock()
		if !p.refPending {
			p.refRunning = false
			p.refMu.Unlock()
			return
		}
		p.refMu.Unlock()
	}
}

// expandRemoteAsync lazily populates a remote folder node's children
// off the UI goroutine. It is the OnExpand callback for the remote tree.
//
// The treeview calls OnExpand from Toggle just before flipping the node's
// Expanded flag, and then only actually expands if the node has ≥1 child.
// So we attach a transient "Loading…" placeholder synchronously — that
// gives the node a child (the tree expands and shows the placeholder,
// immediate feedback) and doubles as the "a fetch is already in flight"
// marker: the len(Children) > 0 guard below then makes a repeat expand
// (collapse-then-expand, or a double Right-arrow) a no-op instead of a
// second network read. When the off-thread ReadDir returns we swap the
// placeholder for the real folders (or an error row) and rebuild the
// flattened view, preserving the user's cursor.
func (p *panel) expandRemoteAsync(n *treeview.Node) {
	if n == nil || len(n.Children) > 0 {
		return // already loaded, or a load is already pending — don't refetch.
	}
	e, ok := n.Data.(*fileEntry)
	if !ok || !e.IsDir {
		return
	}
	if p.closed != nil && p.closed.Load() {
		return
	}

	// Placeholder makes the node expandable (so Toggle flips Expanded and
	// the row shows immediately) and gates against a second scheduling.
	placeholder := &treeview.Node{Label: "Loading…", Parent: n}
	n.Children = []*treeview.Node{placeholder}

	c := p.c
	path := e.Path
	if p.refreshWG != nil {
		p.refreshWG.Add(1) // gates the browser's client.Close on teardown.
	}
	go func() {
		if p.refreshWG != nil {
			defer p.refreshWG.Done()
		}
		kids := buildRemoteTree(c, path) // network read, off the UI goroutine.
		views.CallSoon(func() {
			if p.closed != nil && p.closed.Load() {
				return // browser tore down while the read was in flight.
			}
			// Only replace if the node is still showing our placeholder.
			// If some other mutation intervened (unlikely, but honest to
			// check), drop this stale result rather than clobber it.
			if len(n.Children) != 1 || n.Children[0] != placeholder {
				return
			}
			for _, k := range kids {
				k.Parent = n
			}
			n.Children = kids
			p.rebuildTreePreservingFocus()
			views.MarkDirty()
		})
	}()
}

// rebuildTreePreservingFocus refreshes the tree's flattened row list
// after its node children changed off-thread. TreeView's only public
// rebuild trigger is SetRoots, which resets the cursor to row 0 — jarring
// after an expand deep in the tree. So we capture the focused node,
// rebuild, then re-derive its new row index. The viewport scroll offset
// is left untouched, so the expanded folder stays put on screen and its
// children appear below it.
func (p *panel) rebuildTreePreservingFocus() {
	if p.tree == nil {
		return
	}
	focused := p.tree.CurrentNode()
	p.tree.SetRoots(p.tree.Roots) // rebuilds the flat list; resets Focused to 0.
	if focused == nil {
		return
	}
	if idx := flatIndexOf(p.tree.Roots, focused); idx >= 0 {
		p.tree.Focused = idx
	}
}

// flatIndexOf returns the flattened-row index of target within roots,
// walking in the same pre-order-honoring-Expanded sequence TreeView uses
// to build its visible-row list. Returns -1 if target isn't reachable
// (e.g. the user collapsed an ancestor while a load was in flight).
func flatIndexOf(roots []*treeview.Node, target *treeview.Node) int {
	i := 0
	var walk func(n *treeview.Node) int
	walk = func(n *treeview.Node) int {
		if n == target {
			return i
		}
		i++
		if n.Expanded {
			for _, c := range n.Children {
				if idx := walk(c); idx >= 0 {
					return idx
				}
			}
		}
		return -1
	}
	for _, r := range roots {
		if idx := walk(r); idx >= 0 {
			return idx
		}
	}
	return -1
}

// onTreeSelect: highlighting a folder in the tree snaps the listing
// to that folder's contents.
func (p *panel) onTreeSelect(n *treeview.Node) {
	if n == nil {
		return
	}
	e, ok := n.Data.(*fileEntry)
	if !ok || !e.IsDir {
		return
	}
	p.setCwd(e.Path)
}

// listingEnter is Enter / double-click on a listing row. Folder /
// parent rows cd; file rows fire the preview (this is the explicit
// "open" gesture — auto-preview on highlight was dropped to avoid
// re-fetching remote files on every arrow press).
func (p *panel) listingEnter() {
	if p.listing == nil {
		return
	}
	n := p.listing.CurrentNode()
	if n == nil {
		return
	}
	e, ok := n.Data.(*fileEntry)
	if !ok {
		return
	}
	if e.IsDir {
		p.setCwd(e.Path)
		return
	}
	if p.preview == nil {
		return
	}
	if e.Local {
		p.preview.showLocal(e.Path)
	} else {
		p.preview.show(e.Path)
	}
}

// refreshHeader rewrites the panel's header label to reflect cwd.
func (p *panel) refreshHeader() {
	side := "Local"
	if p.isRemote {
		side = "Remote"
	}
	max := p.width - len(side) - 3
	if max < 4 {
		max = 4
	}
	p.header.Text = side + " — " + ui.TruncLeftPath(p.cwd, max)
}

// listingFocused / treeFocused report which view on this side has
// fv-go focus. Used by the dialog-level key handler to derive the
// transfer source when F5 / F6 fire.
func (p *panel) listingFocused() bool {
	return p.listing != nil && p.listing.GetState(consts.SfFocused)
}
func (p *panel) treeFocused() bool {
	return p.tree != nil && p.tree.GetState(consts.SfFocused)
}

// activeSelection returns this panel's currently-relevant entry plus
// a flag for whether it came from the listing (vs the tree). The
// listing's selection wins when focused; otherwise the tree's
// highlighted folder serves as the fallback.
func (p *panel) activeSelection() (*fileEntry, bool) {
	if p.listing != nil {
		if n := p.listing.CurrentNode(); n != nil {
			if e, ok := n.Data.(*fileEntry); ok {
				return e, true
			}
		}
	}
	if p.tree != nil {
		if n := p.tree.CurrentNode(); n != nil {
			if e, ok := n.Data.(*fileEntry); ok {
				return e, false
			}
		}
	}
	return nil, false
}
