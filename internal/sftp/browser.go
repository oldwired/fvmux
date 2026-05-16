package sftp

import (
	"fmt"
	"sync"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/anim"
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"
	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"

	pkgsftp "github.com/pkg/sftp"
)

const (
	maxPreviewBytes = 64 * 1024
)

// Local Cm codes for the dialog's action buttons. Picked above the
// reserved fv-go range so they don't clash with CmOK/CmCancel/etc.;
// keyHandler intercepts them via OfPreProcess and routes to actions.
const (
	cmSftpCopy   uint16 = 0xF010
	cmSftpCancel uint16 = 0xF011
)

// liveMgrs tracks every browser's transfer manager so the global
// commands (CmdActiveTransfers, CmdClearCompleted) can act across all
// open browsers. Multiple browsers may run concurrently now that the
// dialog is non-modal — each registers on open, deregisters on close.
var (
	liveMu   sync.Mutex
	liveMgrs []*Manager
)

// LiveManager returns the most recently opened manager, or nil if no
// browser is open. Kept for callers that want a single handle.
func LiveManager() *Manager {
	liveMu.Lock()
	defer liveMu.Unlock()
	if n := len(liveMgrs); n > 0 {
		return liveMgrs[n-1]
	}
	return nil
}

// LiveManagers returns a snapshot of every active browser's manager.
// Safe to iterate without holding the mutex.
func LiveManagers() []*Manager {
	liveMu.Lock()
	defer liveMu.Unlock()
	out := make([]*Manager, len(liveMgrs))
	copy(out, liveMgrs)
	return out
}

func addLiveMgr(m *Manager) {
	liveMu.Lock()
	defer liveMu.Unlock()
	liveMgrs = append(liveMgrs, m)
}

func removeLiveMgr(m *Manager) {
	liveMu.Lock()
	defer liveMu.Unlock()
	out := liveMgrs[:0]
	for _, x := range liveMgrs {
		if x != m {
			out = append(out, x)
		}
	}
	liveMgrs = out
}

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
// Show returns nil on success (browser is now on the desktop) or an
// error from Open / Getwd. The caller — usually mux.sftpBrowser — is
// responsible for surfacing the error to the user; ErrAuthRequired
// in particular should be handled by offering to open an SSH pane
// rather than just msgbox'ing.
func Show(a *fvapp.Application, alias, controlPath string, onClose func()) error {
	c, err := Open(alias, controlPath)
	if err != nil {
		if onClose != nil {
			onClose()
		}
		return err
	}

	remoteCwd, err := c.SFTP().Getwd()
	if err != nil {
		_ = c.Close()
		if onClose != nil {
			onClose()
		}
		return fmt.Errorf("sftp getwd: %w", err)
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

	// Vertical layout (bottom-up):
	//
	//   h-1   bottom dialog border
	//   h-2   button shadow row (fv-go buttons cast a one-cell shadow)
	//   h-3   button content row
	//   h-4   one-row gap of dialog interior (kills the "blue squares"
	//         that appear if TaskProgress's strip reaches into the
	//         row the buttons sit on)
	//   h-8 .. h-5     TaskProgress strip (4 rows)
	//   2   .. h-9     tree / listing area
	tpRows := 4
	tpY1 := h - 4
	tpY0 := tpY1 - tpRows
	areaBottom := tpY0 - 1
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

	// Transfer manager + TaskProgress strip. Browser is non-modal, so
	// the manager + ticker are registered for the dialog's whole
	// lifetime and torn down from d.OnClose (set below).
	mgr := NewManager()
	addLiveMgr(mgr)

	tp := taskprogress.New(geom.NewRect(2, tpY0, w-2, tpY1))
	d.Insert(tp)
	tt := &transferTicker{m: mgr, tp: tp, ok: true}
	anim.Register(tt, 200*time.Millisecond)

	// Hotkey handler — Enter for cd, F5/Del for transfers, CmCancel
	// (Esc / Close button) routed to d.Close since non-modal dialogs
	// no-op on EndModal.
	keys := newKeyHandler(a, mgr, remote, local)
	keys.dlg = d
	d.Insert(keys)

	// Bottom row: navigation hints (Tab / Enter / Esc are behaviors,
	// not commands — they stay as text) on the left; three action
	// buttons packed flush to the right edge with 4-cell gaps so the
	// shadow column of each button (which extends one cell past the
	// button's right edge) doesn't run into the next button.
	closeX1 := w - 2
	closeX0 := closeX1 - 10
	cancelX1 := closeX0 - 4
	cancelX0 := cancelX1 - 14
	copyX1 := cancelX0 - 4
	copyX0 := copyX1 - 10
	hintX1 := copyX0 - 4

	hint := dialogs.NewStaticText(
		geom.NewRect(2, h-3, hintX1, h-2),
		"Tab switch  ·  Enter open  ·  Esc close",
	)
	d.Insert(hint)
	d.Insert(dialogs.NewButton(
		geom.NewRect(copyX0, h-3, copyX1, h-2),
		"~C~opy", cmSftpCopy, 0,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(cancelX0, h-3, cancelX1, h-2),
		"C~a~ncel xfer", cmSftpCancel, 0,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(closeX0, h-3, closeX1, h-2),
		"Cl~o~se", consts.CmCancel, dialogs.BfDefault,
	))

	// Single teardown for every close path — [✕] click, Esc, Close
	// button, or programmatic d.Close(). Window.OnClose fires before
	// the window is removed from the desktop, so we have a clean
	// window to act on.
	d.OnClose = func() {
		mgr.CancelAll()
		tt.ok = false
		anim.Unregister(tt)
		removeLiveMgr(mgr)
		_ = c.Close()
		if onClose != nil {
			onClose()
		}
	}

	// Non-modal: insert into the desktop and return immediately. The
	// browser stays up as a regular floating dialog the user can drag,
	// switch focus away from, or close at will. MakeFirst raises it
	// to the top of the z-order AND gives it focus — Insert alone
	// only focuses when no other window already holds it.
	a.Desktop.Insert(d)
	a.Desktop.MakeFirst(d)
	return nil
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
	mv.SetMarkdown("# SFTP\n\n" +
		"- **Tab** — switch between the four panes.\n" +
		"- **Enter** on a folder — change into it.\n" +
		"- **Enter** on a file — preview it here.\n" +
		"- **F5** or the **Copy** button — copy the focused listing's selection to the other side.\n" +
		"- **Del** or **Cancel xfer** — cancel the newest in-flight transfer.\n" +
		"- **Esc** or **Close** — dismiss this browser.")
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
