package sftp

import (
	"fmt"
	"sync"
	"sync/atomic"
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
	cmSftpCopy    uint16 = 0xF010
	cmSftpCancel  uint16 = 0xF011
	cmSftpRefresh uint16 = 0xF012
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

// CloseAllBrowsers dismisses every open SFTP browser dialog. Used by
// File → New Session to clear desktop state (browsers are dialogs,
// not windows, so they aren't reached by the per-window close loop).
// Snapshots the close functions first so the OnClose hooks can
// mutate liveMgrs without invalidating our iteration.
func CloseAllBrowsers() {
	liveMu.Lock()
	closers := make([]func(), 0, len(liveMgrs))
	for _, m := range liveMgrs {
		if m != nil && m.closeFn != nil {
			closers = append(closers, m.closeFn)
		}
	}
	liveMu.Unlock()
	for _, c := range closers {
		c()
	}
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
	// Min 80×18 — tight but everything still draws: ~16-col tree,
	// ~22-col listing, ~20-col preview, plus the transfer strip and
	// button row.
	d := dialogs.NewDialog(
		geom.NewRect(x, y, x+w, y+h),
		fmt.Sprintf("SFTP — %s", alias),
	)
	d.SetSizeLimits(geom.Point{X: 80, Y: 18}, geom.Point{})

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

	// Remote panel (upper-left). Fixed: stays at the top of the
	// dialog at constant width/height — extra Y goes to the local
	// panel and TaskProgress strip, extra X goes to the preview.
	remote := newPanel(d, true, c.SFTP(), remoteCwd, remoteHeader, listX1-treeX0,
		geom.NewRect(treeX0, 2, treeX1, midY),
		geom.NewRect(listX0, 2, listX1, midY),
	)
	// Local panel (lower-left). The tree + listing grow vertically
	// so extra Y is consumed by the local side. The local header
	// sits at midY and stays put (fixed Y).
	local := newPanel(d, false, nil, localCwd, localHeader, listX1-treeX0,
		geom.NewRect(treeX0, midY+1, treeX1, areaBottom),
		geom.NewRect(listX0, midY+1, listX1, areaBottom),
	)
	local.tree.GrowMode = consts.GfGrowHiY
	local.listing.GrowMode = consts.GfGrowHiY
	// Initial header renders include cwd width-aware truncation.
	remote.refreshHeader()
	local.refreshHeader()

	// Shared async-refresh lifetime state: closed stops new remote reads
	// once teardown starts; refreshWG lets OnClose wait for an in-flight
	// read to finish before closing the SFTP client. Only the remote
	// panel does async reads, so only it needs them.
	var browserClosed atomic.Bool
	var refreshWG sync.WaitGroup
	remote.closed = &browserClosed
	remote.refreshWG = &refreshWG

	// Preview spans the right column, full height of the panel area.
	// Extra X and extra Y both flow into the preview.
	pp := newPreviewPane(d, c.SFTP(), geom.NewRect(previewX0, 2, previewX1, areaBottom))
	pp.growMode = consts.GfGrowHiX | consts.GfGrowHiY
	pp.applyGrowMode()
	pp.closed = &browserClosed // shared with the panels' refresh guards.
	pp.refreshWG = &refreshWG
	remote.preview = pp
	local.preview = pp

	// Transfer manager + TaskProgress strip. Browser is non-modal, so
	// the manager + ticker are registered for the dialog's whole
	// lifetime and torn down from d.OnClose (set below).
	mgr := NewManager(alias)
	mgr.SetCloseFn(func() { d.Close() })
	addLiveMgr(mgr)

	// TaskProgress strip: stays anchored to its row band but slides
	// down with the dialog (so the local panel can grow into the
	// freed Y), and stretches with width.
	tp := taskprogress.New(geom.NewRect(2, tpY0, w-2, tpY1))
	tp.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY | consts.GfGrowHiX
	d.Insert(tp)
	tt := &transferTicker{
		m:      mgr,
		tp:     tp,
		remote: remote,
		local:  local,
		seen:   make(map[*Transfer]int32),
		ok:     true,
	}
	anim.Register(tt, 200*time.Millisecond)

	// Hotkey handler — Enter for cd, F5/Del for transfers, CmCancel
	// (Esc / Close button) routed to d.Close since non-modal dialogs
	// no-op on EndModal.
	keys := newKeyHandler(a, mgr, remote, local)
	keys.dlg = d
	d.Insert(keys)

	// Bottom row: navigation hints (Tab / Enter / Esc are behaviors,
	// not commands — they stay as text) on the left; four action
	// buttons packed flush to the right edge with 4-cell gaps so the
	// shadow column of each button (which extends one cell past the
	// button's right edge) doesn't run into the next button.
	closeX1 := w - 2
	closeX0 := closeX1 - 10
	cancelX1 := closeX0 - 4
	cancelX0 := cancelX1 - 14
	copyX1 := cancelX0 - 4
	copyX0 := copyX1 - 10
	refreshX1 := copyX0 - 4
	refreshX0 := refreshX1 - 11
	hintX1 := refreshX0 - 4

	hint := dialogs.NewStaticText(
		geom.NewRect(2, h-3, hintX1, h-2),
		"F5 cp · F6 ren · F7 mkdir · F8 del · Ctrl-R refresh",
	)
	// Hint sticks to the bottom row (Y slides with parent) and
	// stretches horizontally so the full F-key cheat actually
	// becomes visible on wider terminals — without GfGrowHiX it
	// clips at construction width forever.
	hint.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY | consts.GfGrowHiX
	d.Insert(hint)

	refreshBtn := dialogs.NewButton(
		geom.NewRect(refreshX0, h-3, refreshX1, h-2),
		"~R~efresh", cmSftpRefresh, 0,
	)
	refreshBtn.GrowMode = consts.GfGrowAll
	d.Insert(refreshBtn)

	copyBtn := dialogs.NewButton(
		geom.NewRect(copyX0, h-3, copyX1, h-2),
		"~C~opy", cmSftpCopy, 0,
	)
	copyBtn.GrowMode = consts.GfGrowAll
	d.Insert(copyBtn)

	cancelBtn := dialogs.NewButton(
		geom.NewRect(cancelX0, h-3, cancelX1, h-2),
		"C~a~ncel xfer", cmSftpCancel, 0,
	)
	cancelBtn.GrowMode = consts.GfGrowAll
	d.Insert(cancelBtn)

	closeBtn := dialogs.NewButton(
		geom.NewRect(closeX0, h-3, closeX1, h-2),
		"Cl~o~se", consts.CmCancel, dialogs.BfDefault,
	)
	closeBtn.GrowMode = consts.GfGrowAll
	d.Insert(closeBtn)

	// Single teardown for every close path — [✕] click, Esc, Close
	// button, or programmatic d.Close(). Window.OnClose fires before
	// the window is removed from the desktop, so we have a clean
	// window to act on.
	d.OnClose = func() {
		mgr.CancelAll()           // cooperative cancel of in-flight transfers.
		browserClosed.Store(true) // stop new async remote reads.
		tt.ok = false
		anim.Unregister(tt)
		removeLiveMgr(mgr)
		// Close the SFTP client only after every transfer goroutine AND
		// any in-flight async listing refresh has drained — pkg/sftp's
		// Client is not safe to use concurrently with Close. Run on a
		// background goroutine so a network-stalled transfer can't freeze
		// the UI; the cancel above gets healthy transfers out promptly.
		go func() {
			mgr.Wait()
			refreshWG.Wait()
			_ = c.Close()
			if onClose != nil {
				onClose()
			}
		}()
	}

	// Non-modal: insert into the desktop and return immediately. The
	// browser stays up as a regular floating dialog the user can drag,
	// switch focus away from, or close at will.
	//
	// Insert appends d as the new last (topmost) child, so the z-order is
	// already right. MakeFirst would normally also focus it, but it
	// early-returns when the view is already the last child — which Insert
	// just made it — so on its own it leaves keyboard focus on whichever
	// Window held it. That's the bug behind "the SFTP browser opens behind
	// the ssh pane I just typed my passphrase into": the auth-then-retry
	// path spawns an ssh window (which takes focus), and MakeFirst can't
	// move it. Focus(d) takes keyboard focus unconditionally, so the
	// freshly-spawned browser is ready for input no matter what was
	// focused before.
	a.Desktop.Insert(d)
	a.Desktop.MakeFirst(d) // raise z-order (no-op when d is already last).
	a.Desktop.Focus(d)     // …and actually take keyboard focus.
	return nil
}

// previewPane owns the swappable widget on the right side of the
// browser. show(path) tears down the previous widget and inserts a
// fresh one built from BuildPreview.
type previewPane struct {
	d        *dialogs.Dialog
	c        *pkgsftp.Client
	bounds   geom.Rect
	current  views.View
	growMode byte

	// Async remote-preview coordination (shared with the owning browser,
	// same role as the panel fields). closed stops new reads once the
	// browser is tearing down; refreshWG gates client.Close until an
	// in-flight remote read has drained. gen is bumped on every show*
	// call so a slow remote load whose file the user has already
	// navigated away from is dropped instead of clobbering the newer
	// preview. All three are touched only on the UI goroutine except the
	// refreshWG counter, which is Add()ed on the UI goroutine before the
	// read goroutine starts and Done()ed by that goroutine.
	closed    *atomic.Bool
	refreshWG *sync.WaitGroup
	gen       uint64
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

// applyGrowMode pushes the configured GrowMode onto the current
// widget. Called after the initial set-up so a later swap re-applies
// the same flags; also re-applied inside swap() for each new widget.
func (p *previewPane) applyGrowMode() {
	if p.current != nil {
		p.current.BaseView().GrowMode = p.growMode
	}
}

// refreshBounds re-derives p.bounds from the current widget's actual
// rect — important after a dialog resize, since the next widget we
// build must use the new size, not the construction-time size.
func (p *previewPane) refreshBounds() {
	if p.current != nil {
		p.bounds = p.current.BaseView().GetBounds()
	}
}

// show previews a remote file. The remote open + read + image decode
// run on a background goroutine so a large file over a slow link can't
// freeze the whole browser (and with it the UI thread that drives every
// repaint). A "Loading…" placeholder swaps in immediately; the real
// widget replaces it when the read finishes. Results are dropped if the
// browser closed or the user previewed something else in the meantime.
func (p *previewPane) show(path string) {
	p.refreshBounds()
	if p.closed != nil && p.closed.Load() {
		return
	}
	p.gen++
	myGen := p.gen
	bounds := p.bounds
	c := p.c
	p.swap(loadingPreview(bounds, path))

	if p.refreshWG != nil {
		p.refreshWG.Add(1) // gates the browser's client.Close on teardown.
	}
	go func() {
		if p.refreshWG != nil {
			defer p.refreshWG.Done()
		}
		next := BuildPreview(c, path, bounds) // network read, off the UI goroutine.
		views.CallSoon(func() {
			if p.closed != nil && p.closed.Load() {
				return // browser tore down while the read was in flight.
			}
			if myGen != p.gen {
				return // user previewed another file meanwhile.
			}
			p.swap(next)
		})
	}()
}

// showLocal is show() for local-FS paths — reads via os.Open instead of
// the SFTP client. Local reads don't block meaningfully, so this stays
// synchronous; it still bumps gen so a slower remote load already in
// flight (e.g. the user previewed a remote file, then a local one)
// drops its stale result instead of clobbering this preview.
func (p *previewPane) showLocal(path string) {
	p.refreshBounds()
	p.gen++
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
	p.current.BaseView().GrowMode = p.growMode
	p.d.Insert(p.current)
	views.MarkDirty()
}

// transferTicker is registered with the anim loop while the browser
// is open; its Tick rebuilds the TaskProgress widget's task list from
// the Manager's atomic-counter snapshot. It also watches for transfers
// transitioning from Active to Done and refreshes the destination
// panel's listing so the freshly-copied file shows up without the user
// having to navigate away and back.
type transferTicker struct {
	m      *Manager
	tp     *taskprogress.TaskProgress
	remote *panel
	local  *panel
	seen   map[*Transfer]int32 // last-observed status per transfer.
	ok     bool
}

func (t *transferTicker) Tick(now time.Time) bool {
	if !t.ok {
		return false
	}
	snap := t.m.Snapshot()
	changed := false // a transfer appeared, vanished, or changed status.
	active := false  // ≥1 transfer still running (progress bar animates).
	for _, x := range snap {
		st := x.Status()
		prev, had := t.seen[x]
		t.seen[x] = st
		if st == StatusActive {
			active = true
		}
		if !had {
			changed = true // newly observed transfer.
			continue
		}
		if prev == st {
			continue
		}
		changed = true
		if prev != StatusActive || st != StatusDone {
			continue
		}
		// Active → Done: refresh the destination side. Uploads
		// land on the remote, downloads on the local FS.
		switch x.Direction {
		case Upload:
			if t.remote != nil {
				t.remote.refresh()
			}
		case Download:
			if t.local != nil {
				t.local.refresh()
			}
		}
	}
	// Drop entries whose transfers were cleared (ClearCompleted) so
	// the map doesn't grow unbounded over a long-lived browser.
	if len(t.seen) > len(snap) {
		changed = true
		alive := make(map[*Transfer]int32, len(snap))
		for _, x := range snap {
			if st, ok := t.seen[x]; ok {
				alive[x] = st
			}
		}
		t.seen = alive
	}
	// Only rebuild the widget and ask the program loop to repaint when a
	// transfer is actually moving or just changed state. On idle ticks
	// (no transfers, or all settled) we return false so the loop stays
	// quiescent — a forced repaint re-emits any on-screen SIXEL image
	// preview in this same browser, and at 5 Hz that reads as a flickering
	// image with the cursor darting around. Returning false here is what
	// keeps a still image preview still.
	if !changed && !active {
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
