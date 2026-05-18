// Per-side state for the dual-pane browser. Each panel owns a tree
// (folders only), a listing (flat cwd contents), and its current
// folder. Trees drive listings on selection; listings drive previews
// (for files) and panel cwd (for folders, via Enter).
package sftp

import (
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"
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
}

// newPanel builds tree + listing for one side, inserts them into d,
// and wires OnExpand / OnSelect. The listing's TreeView holds only
// leaf nodes (HasChildren = false on every entry) so Enter never
// triggers an inline expansion; the dialog-level key handler
// intercepts Enter first and routes it through listingEnter.
func newPanel(
	d *dialogs.Dialog,
	isRemote bool,
	c *pkgsftp.Client,
	cwd string,
	header *dialogs.StaticText,
	headerW int,
	treeBounds, listingBounds geom.Rect,
) *panel {
	p := &panel{
		isRemote: isRemote,
		c:        c,
		cwd:      cwd,
		header:   header,
		width:    headerW,
	}

	if isRemote {
		p.tree = treeview.New(treeBounds, buildRemoteTree(c, cwd))
		p.tree.OnExpand = func(n *treeview.Node) { expandRemoteTree(c, n) }
		p.listing = treeview.New(listingBounds, buildRemoteListing(c, cwd))
	} else {
		p.tree = treeview.New(treeBounds, buildLocalTree(cwd))
		p.tree.OnExpand = func(n *treeview.Node) { expandLocalTree(n) }
		p.listing = treeview.New(listingBounds, buildLocalListing(cwd))
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
// the user keeps whatever they had unfolded.
func (p *panel) setCwd(newCwd string) {
	if newCwd == "" || newCwd == p.cwd {
		return
	}
	p.cwd = newCwd
	if p.isRemote {
		p.listing.SetRoots(buildRemoteListing(p.c, newCwd))
	} else {
		p.listing.SetRoots(buildLocalListing(newCwd))
	}
	p.refreshHeader()
	views.MarkDirty()
}

// refresh re-reads the panel's current folder and rebuilds the listing
// in place. Called after a transfer completes against this side, and
// from the manual Refresh button / Ctrl-R hotkey. Header/cwd are
// unchanged; tree expansion state is preserved.
func (p *panel) refresh() {
	if p.listing == nil {
		return
	}
	if p.isRemote {
		p.listing.SetRoots(buildRemoteListing(p.c, p.cwd))
	} else {
		p.listing.SetRoots(buildLocalListing(p.cwd))
	}
	views.MarkDirty()
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
	path := p.cwd
	if len(path) > max {
		if max <= 3 {
			path = path[len(path)-max:]
		} else {
			path = "…" + path[len(path)-(max-1):]
		}
	}
	p.header.Text = side + " — " + path
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
