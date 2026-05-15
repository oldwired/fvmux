package sftp

import (
	"fmt"
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

// Show opens an SFTP browser modal against alias. Five regions:
//
//   - upper-left tree  : remote folders, lazy-expanded via OnExpand.
//   - upper-left list  : remote cwd's contents (../ + folders + files).
//   - lower-left tree  : local folders, rooted at $HOME.
//   - lower-left list  : local cwd's contents.
//   - right            : preview pane (markdown / hex / image),
//     driven by whichever listing just highlighted
//     a file.
//
// Bottom strip: TaskProgress widget for active transfers, then a hint
// row + Close button.
//
// Tab cycles focus across the four panes (fv-go's standard
// selectable-view rotation). F5 / F6 copy the file highlighted in the
// focused listing to the other side's cwd — direction is derived from
// focus, so the same chord works both ways. Del cancels the most
// recent in-flight transfer. Enter in a listing dives into a folder
// (../ goes up); Enter on a file is a no-op (preview is already current).
func Show(a *fvapp.Application, alias, controlPath string) {
	c, err := Open(alias, controlPath)
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't open SFTP to %s:\n%s",
			[]any{alias, err.Error()}, msgbox.OKOnly)
		return
	}
	defer c.Close()

	remoteCwd, err := c.SFTP().Getwd()
	if err != nil {
		// Getwd failing after a successful Open usually means the SSH
		// channel died between handshake and the first SFTP request —
		// surface the error rather than presenting an empty "/" view
		// that looks like a working session.
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"SFTP session lost while opening %s:\n%s",
			[]any{alias, err.Error()}, msgbox.OKOnly)
		return
	}
	localCwd := defaultLocalRoot()

	desk := a.Desktop.BaseView()
	w, h := 110, 32
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
		fmt.Sprintf("SFTP — %s", alias),
	)

	// Column widths inside the dialog (inner = w-2 usable).
	// Layout: |tree(22)|listing(32)|preview(rest)|. Two columns on
	// the left stack remote (top) over local (bottom).
	treeW := 22
	listW := 32
	if treeW+listW > w-12 {
		// Tiny terminals — shrink each proportionally so something
		// still draws.
		treeW = (w - 12) / 3
		listW = (w - 12) - treeW
	}
	treeX0 := 2
	treeX1 := treeX0 + treeW
	listX0 := treeX1 + 1
	listX1 := listX0 + listW
	previewX0 := listX1 + 1
	previewX1 := w - 2

	tpRows := 4
	areaBottom := h - 3 - tpRows
	midY := 2 + (areaBottom-2)/2

	// Headers (one per panel quadrant). Editable via panel.refreshHeader.
	remoteHeader := dialogs.NewStaticText(
		geom.NewRect(treeX0, 1, listX1, 2), "Remote — "+remoteCwd)
	localHeader := dialogs.NewStaticText(
		geom.NewRect(treeX0, midY, listX1, midY+1), "Local — "+localCwd)
	d.Insert(remoteHeader)
	d.Insert(localHeader)

	// Remote panel (upper-left).
	remote := newPanel(d, true, c.SFTP(), remoteCwd, remoteHeader, listX1-treeX0,
		geom.NewRect(treeX0, 2, treeX1, midY),
		geom.NewRect(listX0, 2, listX1, midY),
	)
	// Local panel (lower-left).
	local := newPanel(d, false, nil, localCwd, localHeader, listX1-treeX0,
		geom.NewRect(treeX0, midY+1, treeX1, areaBottom),
		geom.NewRect(listX0, midY+1, listX1, areaBottom),
	)
	// Initial header renders include cwd width-aware truncation.
	remote.refreshHeader()
	local.refreshHeader()

	// Preview spans the right column, full height of the panel area.
	pp := newPreviewPane(d, c.SFTP(), geom.NewRect(previewX0, 2, previewX1, areaBottom))
	remote.preview = pp
	local.preview = pp

	// Transfer manager + TaskProgress strip.
	mgr := NewManager()
	liveMgr = mgr
	defer func() {
		// Closing the dialog cancels every in-flight transfer so the
		// background goroutines wake up and exit cleanly instead of
		// writing into a Manager nobody is watching.
		mgr.CancelAll()
		liveMgr = nil
	}()

	tp := taskprogress.New(geom.NewRect(2, areaBottom+1, w-2, areaBottom+1+tpRows))
	d.Insert(tp)
	tt := &transferTicker{m: mgr, tp: tp, ok: true}
	anim.Register(tt, 200*time.Millisecond)
	defer func() { tt.ok = false; anim.Unregister(tt) }()

	// Hotkey handler — Enter for cd, F5/F6/Del for transfers.
	keys := newKeyHandler(a, mgr, remote, local)
	d.Insert(keys)

	// Hint + close.
	hint := dialogs.NewStaticText(
		geom.NewRect(2, h-3, w-15, h-2),
		"Tab  switch  ·  Enter  cd  ·  F5/F6  copy  ·  Del  cancel  ·  Esc  close",
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
	p.swap(next)
}

// showLocal is show() for local-FS paths — reads via os.Open instead
// of the SFTP client.
func (p *previewPane) showLocal(path string) {
	next := BuildLocalPreview(path, p.bounds)
	p.swap(next)
}

func (p *previewPane) swap(next views.View) {
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

// fileEntry is what we stash on each tree / listing Node so callbacks
// can find the path back.
//
//	Local  — true ⇒ local filesystem, false ⇒ remote SFTP side.
//	IsDir  — true ⇒ directory entry.
//	Parent — true ⇒ this is the synthetic "../" row in a listing;
//	         Path holds the parent directory.
type fileEntry struct {
	Path   string
	IsDir  bool
	Local  bool
	Parent bool
}
