package sftp

import (
	"fmt"
	"sort"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"
	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"
)

const (
	maxPreviewBytes = 64 * 1024
)

// liveMgr is the most recently opened browser's transfer manager.
// Exposed so global commands (CmdActiveTransfers, CmdClearCompleted)
// can act on whichever browser is currently up. Reset when a browser
// closes — a no-op when no browser is open.
var liveMgr *Manager

// LiveManager returns the active SFTP manager, or nil if no browser
// is open. Read-only — callers must not mutate it directly.
func LiveManager() *Manager { return liveMgr }

// Show opens an SFTP browser modal against alias. The browser is
// single-pane (remote tree) with a swappable preview area on the
// right: text/markdown rendered as markdown, image decoded into an
// ImageView, binary into a hex viewer. F5 downloads the focused file,
// F6 uploads a local file into the remote cwd, Del cancels the most
// recent in-flight transfer; progress lands in the TaskProgress strip
// at the bottom of the dialog.
//
// Dual-pane (local + remote) is not yet wired — the F5/F6 prompts
// substitute by taking a typed local path; tracked as a Stage 2 polish.
func Show(a *fvapp.Application, alias, controlPath string) {
	c, err := Open(alias, controlPath)
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't open SFTP to %s:\n%s",
			[]any{alias, err.Error()}, msgbox.OKOnly)
		return
	}
	defer c.Close()

	cwd, err := c.SFTP().Getwd()
	if err != nil {
		cwd = "/"
	}

	desk := a.Desktop.BaseView()
	w, h := 100, 30
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	d := dialogs.NewDialog(
		geom.NewRect(x, y, x+w, y+h),
		fmt.Sprintf("SFTP — %s — %s", alias, cwd),
	)

	// Top region: tree on the left, preview on the right.
	// Bottom region: transfer progress strip + key hints + close button.
	tpRows := 4
	treeBottom := h - 3 - tpRows
	treeW := w / 2

	tree := treeview.New(
		geom.NewRect(2, 2, treeW-1, treeBottom),
		buildRoots(c.SFTP(), cwd),
	)
	tree.OnExpand = func(n *treeview.Node) { expandNode(c.SFTP(), n) }
	d.Insert(tree)

	previewBounds := geom.NewRect(treeW+1, 2, w-2, treeBottom)
	pp := newPreviewPane(d, c.SFTP(), previewBounds)
	tree.OnSelect = func(n *treeview.Node) {
		if n == nil {
			return
		}
		e, ok := n.Data.(*fileEntry)
		if !ok || e.IsDir {
			return
		}
		pp.show(e.Path)
	}

	// Transfer manager + TaskProgress strip.
	mgr := NewManager()
	liveMgr = mgr
	defer func() { liveMgr = nil }()

	tp := taskprogress.New(geom.NewRect(2, treeBottom+1, w-2, treeBottom+1+tpRows))
	d.Insert(tp)
	tt := &transferTicker{m: mgr, tp: tp, ok: true}
	anim.Register(tt, 200*time.Millisecond)
	defer func() { tt.ok = false; anim.Unregister(tt) }()

	// Hotkey handler — F5/F6/Del.
	keys := newKeyHandler(a, c.SFTP(), mgr, tree, cwd)
	d.Insert(keys)

	// Hint line + close button.
	hint := dialogs.NewStaticText(
		geom.NewRect(2, h-3, w-15, h-2),
		"F5 download  F6 upload  Del cancel  Esc close",
	)
	d.Insert(hint)
	d.Insert(dialogs.NewButton(
		geom.NewRect(w-12, h-3, w-2, h-2),
		"Cl~o~se", consts.CmCancel, dialogs.BfDefault,
	))

	a.Desktop.ExecView(d)
}

// previewPane owns the swappable widget on the right side of the
// browser. show(path) tears down the previous widget and inserts a
// fresh one built from BuildPreview.
type previewPane struct {
	d       *dialogs.Dialog
	c       *pkgsftp.Client
	bounds  geom.Rect
	current views.View
}

func newPreviewPane(d *dialogs.Dialog, c *pkgsftp.Client, bounds geom.Rect) *previewPane {
	mv := markdown.New(bounds, nil)
	mv.SetMarkdown("# SFTP\n\nSelect a file in the tree to preview.\n\n" +
		"`F5` download · `F6` upload · `Del` cancel transfer · `Esc` close")
	d.Insert(mv)
	return &previewPane{d: d, c: c, bounds: bounds, current: mv}
}

func (p *previewPane) show(path string) {
	next := BuildPreview(p.c, path, p.bounds)
	if next == nil {
		return
	}
	if p.current != nil {
		p.d.Delete(p.current)
	}
	p.current = next
	p.d.Insert(p.current)
	views.MarkDirty()
}

// transferTicker is registered with the anim loop while the browser
// is open; its Tick rebuilds the TaskProgress widget's task list from
// the Manager's atomic-counter snapshot.
type transferTicker struct {
	m  *Manager
	tp *taskprogress.TaskProgress
	ok bool
}

func (t *transferTicker) Tick(now time.Time) bool {
	if !t.ok {
		return false
	}
	t.m.SyncWidget(t.tp)
	return true
}

func (t *transferTicker) Alive() bool { return t.ok }

// fileEntry is what we stash on each tree Node so OnExpand / preview
// can find the path back.
type fileEntry struct {
	Path  string
	IsDir bool
}

func buildRoots(s *pkgsftp.Client, cwd string) []*treeview.Node {
	entries, err := s.ReadDir(cwd)
	if err != nil {
		return []*treeview.Node{
			{Label: "<error: " + err.Error() + ">"},
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})
	out := make([]*treeview.Node, 0, len(entries))
	for _, e := range entries {
		path := cwd + "/" + e.Name()
		if cwd == "/" {
			path = "/" + e.Name()
		}
		label := e.Name()
		if e.IsDir() {
			label += "/"
		} else {
			label += fmt.Sprintf("  (%d bytes)", e.Size())
		}
		out = append(out, &treeview.Node{
			Label:       label,
			Data:        &fileEntry{Path: path, IsDir: e.IsDir()},
			HasChildren: e.IsDir(),
		})
	}
	return out
}

func expandNode(s *pkgsftp.Client, n *treeview.Node) {
	if n == nil {
		return
	}
	if len(n.Children) > 0 {
		return // already populated.
	}
	entry, ok := n.Data.(*fileEntry)
	if !ok || !entry.IsDir {
		return
	}
	for _, child := range buildRoots(s, entry.Path) {
		child.Parent = n
		n.Children = append(n.Children, child)
	}
}
