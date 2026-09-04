package sftp

import (
	"context"
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
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"

	"github.com/oldwired/fvmux/internal/ui"
)

const (
	maxPreviewBytes = 64 * 1024
)

// Local Cm codes for the dialog's action buttons. Picked above the
// reserved fv-go range so they don't clash with CmOK/CmCancel/etc.;
// keyHandler intercepts them via OfPreProcess and routes to actions.
const (
	cmSftpCopy     uint16 = 0xF010
	cmSftpCancel   uint16 = 0xF011
	cmSftpRefresh  uint16 = 0xF012
	cmSftpTerminal uint16 = 0xF013
)

// liveMgrs tracks every browser's transfer manager so the global
// commands (CmdActiveTransfers, CmdClearCompleted) can act across all
// open browsers. Multiple browsers may run concurrently now that the
// dialog is non-modal — each registers on open, deregisters on close.
var (
	liveMu   sync.Mutex
	liveMgrs []*Manager
)

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

// connectResult bundles everything the blocking network phase produces
// so the UI phase can build the dialog without any further round-trips.
type connectResult struct {
	client    *Client
	remoteCwd string
	tree      []*treeview.Node // initial remote folder tree.
	listing   []*treeview.Node // initial remote cwd listing.
}

// connectFn performs the blocking network phase: spawn ssh, negotiate
// SFTP, read the working directory and the first tree + listing. It is a
// package var so tests can inject a fake connector (Open spawns a real
// ssh subprocess, which a unit test can't).
var connectFn = defaultConnect

// buildBrowserFn is the UI-phase assembler; a package var so tests can
// stub the (heavy, Application-dependent) dialog build and assert the
// success routing in isolation.
var buildBrowserFn = buildBrowser

func defaultConnect(ctx context.Context, alias, controlPath string, hostOpts []string, requestedCwd string) (*connectResult, error) {
	c, err := OpenContext(ctx, alias, controlPath, hostOpts)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	remoteCwd := requestedCwd
	if remoteCwd == "" {
		remoteCwd, err = c.SFTP().Getwd()
		if err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("sftp getwd: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	if info, statErr := c.SFTP().Stat(remoteCwd); statErr != nil {
		_ = c.Close()
		return nil, fmt.Errorf("remote folder %s: %w", remoteCwd, statErr)
	} else if !info.IsDir() {
		_ = c.Close()
		return nil, fmt.Errorf("remote folder %s is not a directory", remoteCwd)
	}
	directory, err := c.ReadDirectory(ctx, remoteCwd)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("read remote folder %s: %w", remoteCwd, err)
	}
	tree := buildRemoteTreeEntries(remoteCwd, directory)
	listing := buildRemoteListingEntries(remoteCwd, directory)
	if err := ctx.Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &connectResult{client: c, remoteCwd: remoteCwd, tree: tree, listing: listing}, nil
}

// OpenOptions supplies workspace context that is not part of the SSH
// connection itself. Empty directory values use the remote account's home
// and the process's normal local starting directory respectively.
type OpenOptions struct {
	RemoteCWD   string
	LocalCWD    string
	FocusSide   string // "remote" or "local"
	OnTerminal  func()
	OnRemoteCWD func(string)
}

// Browser is the live contents of a first-class Files window. The app owns
// the Window and its workspace identity; Browser owns the SFTP client,
// panels, transfer manager, and their asynchronous teardown.
type Browser struct {
	Alias   string
	Window  *views.Window
	Manager *Manager

	client         *Client
	remote         *panel
	local          *panel
	ticker         *transferTicker
	closed         atomic.Bool
	refreshWG      sync.WaitGroup
	closeOnce      sync.Once
	lifetimeCancel context.CancelFunc
	focusSide      string
}

func (b *Browser) RemoteCWD() string {
	if b == nil || b.remote == nil {
		return ""
	}
	return b.remote.cwd
}

func (b *Browser) LocalCWD() string {
	if b == nil || b.local == nil {
		return ""
	}
	return b.local.cwd
}

func (b *Browser) FocusSide() string {
	if b == nil || b.Window == nil {
		return "remote"
	}
	cur := b.Window.Current()
	if b.local != nil && (cur == b.local.tree.Self() || cur == b.local.listing.Self()) {
		b.focusSide = "local"
		return "local"
	}
	if b.remote != nil && (cur == b.remote.tree.Self() || cur == b.remote.listing.Self()) {
		b.focusSide = "remote"
	}
	if b.focusSide == "" {
		return "remote"
	}
	return b.focusSide
}

func (b *Browser) NavigateRemote(cwd string) {
	if b != nil && b.remote != nil && cwd != "" {
		b.remote.setCwd(cwd)
	}
}

func (b *Browser) ActiveOperations() int {
	if b == nil || b.Manager == nil {
		return 0
	}
	return b.Manager.ActiveCount()
}

// Close stops new work immediately, then drains network operations off the UI
// goroutine before closing the shared SFTP client. done runs after teardown.
func (b *Browser) Close(done func()) {
	if b == nil {
		if done != nil {
			done()
		}
		return
	}
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		if b.lifetimeCancel != nil {
			b.lifetimeCancel()
		}
		if b.Manager != nil {
			b.Manager.CancelAll()
		}
		if b.ticker != nil {
			b.ticker.ok = false
			anim.Unregister(b.ticker)
		}
		if b.Manager != nil {
			removeLiveMgr(b.Manager)
		}
		go func() {
			b.refreshWG.Wait()
			if b.Manager != nil {
				b.Manager.CancelAll()
				b.Manager.Wait()
			}
			if b.client != nil {
				_ = b.client.Close()
			}
			if done != nil {
				done()
			}
		}()
	})
}

// ShowAsync connects a Files window against alias without blocking the
// UI event goroutine. The network phase (ssh connect, SFTP negotiate,
// Getwd, initial remote reads) runs on a background goroutine; the dialog
// is constructed on the UI goroutine via views.CallSoon once the data
// arrives. Opening a browser to a dead host therefore no longer freezes
// the multiplexer for the connect timeout.
//
// onResolved runs on the UI goroutine with either the live Browser or an
// error. Cancellation is checked again at UI delivery so a successful but
// stale worker result can never populate a window from a different session.
//
// The dialog layout (built in buildBrowser) has five regions: remote
// folder tree + listing (upper-left), local folder tree + listing
// (lower-left), and a shared preview pane on the right; a TaskProgress
// strip and the action-button row run along the bottom.
func ShowAsync(
	ctx context.Context,
	a *fvapp.Application,
	frame *views.Window,
	alias, controlPath string,
	hostOpts []string,
	parallel int,
	opts OpenOptions,
	onResolved func(*Browser, error),
) {
	go func() {
		res, err := connectFn(ctx, alias, controlPath, hostOpts, opts.RemoteCWD)
		views.CallSoon(func() {
			if err != nil {
				if onResolved != nil {
					onResolved(nil, err)
				}
				return
			}
			if err := ctx.Err(); err != nil {
				_ = res.client.Close()
				if onResolved != nil {
					onResolved(nil, err)
				}
				return
			}
			browser := buildBrowserFn(a, frame, alias, controlPath, hostOpts, parallel, opts, res)
			if onResolved != nil {
				onResolved(browser, nil)
			}
		})
	}()
}

// buildBrowser assembles a Files window from an already-connected
// client. Runs on the UI goroutine (called from ShowAsync's CallSoon).
// Tab cycles focus across the four panes; F5 copies the focused
// listing's selection to the other side, F6 moves/renames it (direction
// derived from focus), Del cancels the newest in-flight transfer; Enter
// in a listing dives into a folder (../ goes up), Enter on a file is a
// no-op (its preview is already current).
func buildBrowser(a *fvapp.Application, frame *views.Window, alias, controlPath string, hostOpts []string, parallel int, opts OpenOptions, res *connectResult) *Browser {
	c := res.client
	remoteCwd := res.remoteCwd
	localCwd := defaultLocalRoot()
	if opts.LocalCWD != "" {
		localCwd = opts.LocalCWD
	}

	wasActive := frame != nil && a != nil && a.Desktop.Current() == frame.Self()
	if frame == nil {
		desk := a.Desktop.BaseView()
		r := ui.CenterRect(desk.Size, 110, 32, 2)
		frame = views.NewWindow(r, fmt.Sprintf("[%s] Files — %s", alias, remoteCwd), 0)
		a.Desktop.Insert(frame)
	}
	for _, child := range append([]views.View(nil), frame.Children...) {
		if child != frame.Frame.Self() {
			frame.Delete(child)
		}
	}
	w, h := frame.BaseView().Size.X, frame.BaseView().Size.Y
	// Min 80×18 — tight but everything still draws: ~16-col tree,
	// ~22-col listing, ~20-col preview, plus the transfer strip and
	// button row.
	d := frame
	d.SetTitle(fmt.Sprintf("[%s] Files — %s", alias, remoteCwd))
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

	// The lifetime context is shared by bounded directory reads, recursive
	// scans, and dedicated transfers so closing the Files window interrupts
	// every network operation before teardown waits for it.
	lifetimeCtx, lifetimeCancel := context.WithCancel(context.Background())

	// Remote panel (upper-left). Fixed: stays at the top of the
	// dialog at constant width/height — extra Y goes to the local
	// panel and TaskProgress strip, extra X goes to the preview.
	remote := newPanel(d, true, c.SFTP(), remoteCwd, remoteHeader, listX1-treeX0,
		geom.NewRect(treeX0, 2, treeX1, midY),
		geom.NewRect(listX0, 2, listX1, midY),
		res.tree, res.listing, // initial reads already done off the UI goroutine.
	)
	// Local panel (lower-left). The tree + listing grow vertically
	// so extra Y is consumed by the local side. The local header
	// sits at midY and stays put (fixed Y).
	local := newPanel(d, false, nil, localCwd, localHeader, listX1-treeX0,
		geom.NewRect(treeX0, midY+1, treeX1, areaBottom),
		geom.NewRect(listX0, midY+1, listX1, areaBottom),
		buildLocalTree(localCwd), buildLocalListing(localCwd), // local reads don't block.
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
	browser := &Browser{Alias: alias, Window: d, client: c, remote: remote, local: local, lifetimeCancel: lifetimeCancel, focusSide: opts.FocusSide}
	remote.ctx = lifetimeCtx
	remote.readDirectory = c.ReadDirectory
	remote.treeBudget = &remoteTreeBudget{used: countRemoteTreeNodes(res.tree), limit: maxRemoteTreeNodes}
	remote.onActive = func() { browser.focusSide = "remote" }
	local.onActive = func() { browser.focusSide = "local" }
	remote.closed = &browser.closed
	remote.refreshWG = &browser.refreshWG
	remote.onCwd = opts.OnRemoteCWD

	// Preview spans the right column, full height of the panel area.
	// Extra X and extra Y both flow into the preview.
	pp := newPreviewPane(d, c.SFTP(), geom.NewRect(previewX0, 2, previewX1, areaBottom))
	pp.growMode = consts.GfGrowHiX | consts.GfGrowHiY
	pp.applyGrowMode()
	pp.ctx = lifetimeCtx
	pp.openClient = func(ctx context.Context) (*Client, error) {
		return OpenContext(ctx, alias, controlPath, hostOpts)
	}
	pp.closed = &browser.closed // shared with the panels' refresh guards.
	pp.refreshWG = &browser.refreshWG
	remote.preview = pp
	local.preview = pp

	// Transfer manager + TaskProgress strip. Browser is non-modal, so
	// the manager + ticker are registered for the dialog's whole
	// lifetime and torn down from d.OnClose (set below).
	mgr := NewManager(alias)
	mgr.SetParallel(parallel) // cap concurrent file transfers (config.SFTP.Parallel).
	mgr.SetRemoteDirectoryReader(c.ReadDirectory)
	// Single-file F5/F6 transfers and whole folder operations get an owned ssh
	// session off the same ControlMaster, so a dead link can be hard-aborted.
	mgr.EnableDedicatedTransfersContext(lifetimeCtx, controlPath, hostOpts)
	addLiveMgr(mgr)
	browser.Manager = mgr

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
	browser.ticker = tt

	// Hotkey handler — Enter for cd, F5/Del for transfers, CmCancel
	// (Esc / Close button) routed to d.Close since non-modal dialogs
	// no-op on EndModal.
	keys := newKeyHandler(a, mgr, remote, local)
	keys.window = d
	keys.onTerminal = opts.OnTerminal
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
		geom.NewRect(16, h-3, hintX1, h-2),
		"F5 cp · F6 mv · F7 mkdir · F8 del · Ctrl-R refresh",
	)
	// Hint sticks to the bottom row (Y slides with parent) and
	// stretches horizontally so the full F-key cheat actually
	// becomes visible on wider terminals — without GfGrowHiX it
	// clips at construction width forever.
	hint.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY | consts.GfGrowHiX
	d.Insert(hint)
	terminalBtn := dialogs.NewButton(
		geom.NewRect(2, h-3, 14, h-2), "~T~erminal", cmSftpTerminal, 0,
	)
	terminalBtn.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY
	d.Insert(terminalBtn)

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

	if wasActive {
		if opts.FocusSide == "local" {
			d.Focus(local.listing)
		} else {
			d.Focus(remote.listing)
		}
	}
	return browser
}

// previewPane owns the swappable widget on the right side of the
// browser. show(path) tears down the previous widget and inserts a
// fresh one built from BuildPreview.
type previewPane struct {
	d        *views.Window
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

	// Only one remote preview may read/decode at once. Each activation owns a
	// cancellable SFTP subprocess; a newer preview cancels the previous one,
	// while the slot prevents already-buffered image decodes from accumulating.
	ctx           context.Context
	previewCancel context.CancelFunc
	previewSlots  chan struct{}
	openClient    func(context.Context) (*Client, error)
	render        func(*pkgsftp.Client, string, geom.Rect) views.View
}

func newPreviewPane(d *views.Window, c *pkgsftp.Client, bounds geom.Rect) *previewPane {
	mv := markdown.New(bounds, nil)
	mv.SetMarkdown("# Files\n\n" +
		"- **Tab** — switch between the four panes.\n" +
		"- **Enter** on a folder — change into it.\n" +
		"- **Enter** on a file — preview it here.\n" +
		"- **F5** or the **Copy** button — copy the focused listing's selection (file or folder) to the other side.\n" +
		"- **F6** — move/rename: a bare name renames in place; a path moves it to the other side.\n" +
		"- **Del** or **Cancel xfer** — cancel the newest in-flight transfer.\n" +
		"- **Terminal** — return to the matching SSH terminal.\n" +
		"- **Esc** or **Close** — close this Files window.")
	d.Insert(mv)
	return &previewPane{
		d: d, c: c, bounds: bounds, current: mv,
		previewSlots: make(chan struct{}, 1),
		render:       BuildPreview,
	}
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
	if p.previewCancel != nil {
		p.previewCancel()
	}
	baseCtx := p.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	previewCtx, cancel := context.WithCancel(baseCtx)
	p.previewCancel = cancel
	p.swap(loadingPreview(bounds, path))

	if p.refreshWG != nil {
		p.refreshWG.Add(1) // gates the browser's client.Close on teardown.
	}
	go func() {
		defer cancel()
		if p.refreshWG != nil {
			defer p.refreshWG.Done()
		}
		next := p.buildRemote(previewCtx, path, bounds) // network read, off the UI goroutine.
		views.CallSoon(func() {
			if p.closed != nil && p.closed.Load() {
				return // browser tore down while the read was in flight.
			}
			if myGen != p.gen {
				return // user previewed another file meanwhile.
			}
			p.previewCancel = nil
			p.swap(next)
		})
	}()
}

func (p *previewPane) buildRemote(ctx context.Context, path string, bounds geom.Rect) views.View {
	if p.previewSlots != nil {
		select {
		case p.previewSlots <- struct{}{}:
			defer func() { <-p.previewSlots }()
		case <-ctx.Done():
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	client := p.c
	if p.openClient != nil {
		owned, err := p.openClient(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errorPreview(bounds, err.Error())
		}
		defer func() { _ = owned.Close() }()
		client = owned.SFTP()
	}
	render := p.render
	if render == nil {
		render = BuildPreview
	}
	return render(client, path, bounds)
}

// showLocal is show() for local-FS paths — reads via os.Open instead of
// the SFTP client. Local reads don't block meaningfully, so this stays
// synchronous; it still bumps gen so a slower remote load already in
// flight (e.g. the user previewed a remote file, then a local one)
// drops its stale result instead of clobbering this preview.
func (p *previewPane) showLocal(path string) {
	p.refreshBounds()
	if p.previewCancel != nil {
		p.previewCancel()
		p.previewCancel = nil
	}
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
	m           *Manager
	tp          *taskprogress.TaskProgress
	remote      *panel
	local       *panel
	seen        map[*Transfer]int32 // last-observed status per transfer.
	wasScanning bool
	ok          bool
}

func (t *transferTicker) Tick(now time.Time) bool {
	if !t.ok {
		return false
	}
	snap := t.m.Snapshot()
	changed := false // a transfer appeared, vanished, or changed status.
	active := t.m.HasScanning()
	if active != t.wasScanning {
		changed = true
		t.wasScanning = active
	}
	for _, x := range snap {
		st := x.Status()
		prev, had := t.seen[x]
		t.seen[x] = st
		if IsActiveStatus(st) {
			active = true
		}
		if !had {
			changed = true // newly observed transfer.
			if st == StatusDone {
				t.refreshDestination(x)
			}
			continue
		}
		if prev == st {
			continue
		}
		changed = true
		if !IsActiveStatus(prev) || st != StatusDone {
			continue
		}
		// Active → Done: refresh the destination side. Uploads
		// land on the remote, downloads on the local FS.
		t.refreshDestination(x)
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

func (t *transferTicker) refreshDestination(x *Transfer) {
	if x == nil {
		return
	}
	if x.Direction == Upload && t.remote != nil {
		t.remote.refresh()
	}
	if x.Direction == Download && t.local != nil {
		t.local.refresh()
	}
}

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
