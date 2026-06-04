package sftp

import (
	"os"
	"path/filepath"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"

	pkgsftp "github.com/pkg/sftp"
)

// keyHandler is an invisible OfPreProcess view installed inside the
// browser dialog. It intercepts:
//
//   - Enter   — when focus is in a listing, dive into a folder /
//     parent row. The listing's TreeView would otherwise
//     try to toggle children (no-op for leaf-only
//     listings, but consuming the event keeps it tidy).
//   - F5      — copy the focused listing's highlighted file or folder
//     to the other side's cwd. Direction is derived from focus.
//   - F6      — move/rename the focused listing's selection.
//   - F7      — make a new directory inside the focused side's cwd.
//   - F8      — delete (recursively) the focused listing's selection.
//   - Ctrl-R  — refresh both panels' listings.
//   - Del     — cancel the most recent in-flight transfer.
//
// Tab cycling between the four selectable views (remote tree, remote
// listing, local tree, local listing) is handled by fv-go's standard
// dialog focus rotation; this handler doesn't touch Tab.
type keyHandler struct {
	views.Base

	app    *fvapp.Application
	mgr    *Manager
	remote *panel
	local  *panel
	dlg    *dialogs.Dialog // set by Show after construction.
}

func newKeyHandler(a *fvapp.Application, mgr *Manager, remote, local *panel) *keyHandler {
	h := &keyHandler{
		Base:   views.NewBase(geom.NewRect(0, 0, 0, 0)),
		app:    a,
		mgr:    mgr,
		remote: remote,
		local:  local,
	}
	h.SetSelf(h)
	h.Options |= consts.OfPreProcess
	return h
}

// GetTypeID for serial registry.
func (h *keyHandler) GetTypeID() string { return "sftpkeys" }

// Draw is a no-op — the view is invisible.
func (h *keyHandler) Draw() {}

// HandleEvent intercepts the listing-relevant keys and the dialog's
// action-button Cm codes. Everything else (including Tab) passes
// through untouched.
func (h *keyHandler) HandleEvent(ev *drivers.Event) {
	if ev.What == consts.EvCommand {
		switch ev.Command {
		case cmSftpCopy:
			h.copyAcross()
			ev.What = consts.EvNothing
		case cmSftpCancel:
			h.mgr.CancelLast()
			ev.What = consts.EvNothing
		case cmSftpRefresh:
			h.refreshBoth()
			ev.What = consts.EvNothing
		case consts.CmCancel:
			// Dialog's own EndModal is a no-op for non-modal — close
			// the dialog ourselves. OnClose then runs the teardown.
			if h.dlg != nil {
				h.dlg.Close()
				ev.What = consts.EvNothing
			}
		}
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbEnter:
		// Enter on a listing row: cd into folder, preview a file.
		// Trees use Enter for expand/collapse via TreeView's own
		// handler — leave their Enter alone.
		if p := h.focusedListingPanel(); p != nil {
			p.listingEnter()
			ev.What = consts.EvNothing
		}
	case consts.KbF5:
		h.copyAcross()
		ev.What = consts.EvNothing
	case consts.KbF6:
		h.moveSelected()
		ev.What = consts.EvNothing
	case consts.KbF7:
		h.makeDir()
		ev.What = consts.EvNothing
	case consts.KbF8:
		h.deleteSelected()
		ev.What = consts.EvNothing
	case consts.KbCtrlR:
		h.refreshBoth()
		ev.What = consts.EvNothing
	case consts.KbDel:
		if h.mgr.CancelLast() {
			ev.What = consts.EvNothing
		}
	}
}

// makeDir, moveSelected, deleteSelected are the thin keyHandler
// wrappers around the action funcs. They use a strict focus check —
// F7 only requires *some* panel focus (mkdir doesn't care which row is
// highlighted), F6/F8 also require listing focus (move/delete need a row).
func (h *keyHandler) makeDir() {
	p, _ := h.strictFocusedPanel()
	mkdirAction(h.app, p)
}

func (h *keyHandler) deleteSelected() {
	p, listingFocused := h.strictFocusedPanel()
	deleteAction(h.app, p, listingFocused)
}

// opposite returns the panel that isn't p (the move destination side).
func (h *keyHandler) opposite(p *panel) *panel {
	if p == h.remote {
		return h.local
	}
	return h.remote
}

// strictFocusedPanel returns the panel whose tree OR listing currently
// holds focus, plus a flag for whether the listing was focused.
// Unlike focusedSide (used by copy), this returns (nil, false) when
// focus is on a button / hint instead of falling back to the remote
// side — mutating actions should never silently default.
func (h *keyHandler) strictFocusedPanel() (*panel, bool) {
	switch {
	case h.remote != nil && (h.remote.listingFocused() || h.remote.treeFocused()):
		return h.remote, h.remote.listingFocused()
	case h.local != nil && (h.local.listingFocused() || h.local.treeFocused()):
		return h.local, h.local.listingFocused()
	}
	return nil, false
}

// refreshBoth re-reads both panels' current folders. Bound to the
// Refresh button and Ctrl-R; the post-transfer auto-refresh only
// refreshes the destination side (driven from transferTicker).
func (h *keyHandler) refreshBoth() {
	if h.remote != nil {
		h.remote.refresh()
	}
	if h.local != nil {
		h.local.refresh()
	}
}

// focusedListingPanel returns the panel whose listing currently holds
// fv-go focus, or nil if no listing is focused.
func (h *keyHandler) focusedListingPanel() *panel {
	switch {
	case h.remote != nil && h.remote.listingFocused():
		return h.remote
	case h.local != nil && h.local.listingFocused():
		return h.local
	}
	return nil
}

// focusedSide returns the panel whose tree OR listing currently holds
// focus, plus a flag for whether the listing was the focused element.
// Falls back to the remote panel when nothing's focused (lets F5/F6
// give a useful error message instead of silently doing nothing).
func (h *keyHandler) focusedSide() (active, other *panel, listingFocused bool) {
	switch {
	case h.remote != nil && (h.remote.listingFocused() || h.remote.treeFocused()):
		return h.remote, h.local, h.remote.listingFocused()
	case h.local != nil && (h.local.listingFocused() || h.local.treeFocused()):
		return h.local, h.remote, h.local.listingFocused()
	}
	return h.remote, h.local, false
}

// copyAcross transfers the focused listing's selection to the other
// panel's cwd. Files copy via a single transfer; directories fan out
// into one transfer per file (see copyDir / Manager.StartTree). Errors:
//
//   - focus is on a tree, not a listing → ask the user to pick a row.
//   - selected row is the "../" parent → same message.
//   - source file is missing locally → surface the os.Stat error.
func (h *keyHandler) copyAcross() {
	active, other, listingFocused := h.focusedSide()
	if active == nil || other == nil {
		return
	}
	if !listingFocused {
		msgbox.Show(&h.app.Desktop.Group, msgbox.Info,
			"Highlight a file or folder in either listing, then F5 to copy.",
			msgbox.OKOnly)
		return
	}
	e, _ := active.activeSelection()
	if e == nil || e.Parent {
		msgbox.Show(&h.app.Desktop.Group, msgbox.Info,
			"Highlight a file or folder to copy.",
			msgbox.OKOnly)
		return
	}

	if active.isRemote {
		// Remote → local download.
		dst := filepath.Join(other.cwd, filepath.Base(e.Path))
		if e.IsDir {
			h.copyDir(active.c, Download, e.Path, dst, false, other)
			return
		}
		if _, err := os.Stat(dst); err == nil && !h.confirmOverwrite(dst) {
			return
		}
		if _, err := h.mgr.StartDedicated(active.c, Download, dst, e.Path, nil); err != nil {
			msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
				"Download failed: %s", []any{err.Error()}, msgbox.OKOnly)
		}
		return
	}
	// Local → remote upload.
	dst := joinRemote(other.cwd, filepath.Base(e.Path))
	if e.IsDir {
		h.copyDir(other.c, Upload, e.Path, dst, true, other)
		return
	}
	if _, err := os.Stat(e.Path); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Can't read %s: %s", []any{e.Path, err.Error()}, msgbox.OKOnly)
		return
	}
	if _, err := other.c.Stat(dst); err == nil && !h.confirmOverwrite(dst) {
		return
	}
	if _, err := h.mgr.StartDedicated(other.c, Upload, e.Path, dst, nil); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Upload failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}

// copyDir recursively copies a directory across panes. srcRoot is the
// source directory (local for Upload, remote for Download); dst is the
// destination directory on the other side. destRemote selects which
// side to stat for the pre-copy existence check; other is the
// destination panel, refreshed directly when the tree holds no files
// (the transferTicker only refreshes after a file transfer completes).
func (h *keyHandler) copyDir(c *pkgsftp.Client, dir Direction, srcRoot, dst string, destRemote bool, other *panel) {
	exists := false
	if destRemote {
		_, err := c.Stat(dst)
		exists = err == nil
	} else {
		_, err := os.Stat(dst)
		exists = err == nil
	}
	if exists && !h.confirmMerge(dst) {
		return
	}

	var localRoot, remoteRoot string
	switch dir {
	case Upload: // local srcRoot → remote dst
		localRoot, remoteRoot = srcRoot, dst
	case Download: // remote srcRoot → local dst
		remoteRoot, localRoot = srcRoot, dst
	}
	n, err := h.mgr.StartTree(c, dir, localRoot, remoteRoot)
	if err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Copy folder failed: %s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	if n == 0 && other != nil {
		other.refresh()
	}
}

// confirmOverwrite asks before clobbering an existing destination,
// mirroring the F8 delete confirmation. Returns true to proceed.
func (h *keyHandler) confirmOverwrite(dest string) bool {
	return msgbox.Showf(&h.app.Desktop.Group, msgbox.Question,
		"%s\nalready exists. Overwrite?", []any{dest}, msgbox.YesNo) == consts.CmYes
}

// confirmMerge asks before copying into an existing destination folder,
// whose contents will be merged (same-named files overwritten).
func (h *keyHandler) confirmMerge(dest string) bool {
	return msgbox.Showf(&h.app.Desktop.Group, msgbox.Question,
		"%s\nalready exists. Merge into it (overwriting same-named files)?",
		[]any{dest}, msgbox.YesNo) == consts.CmYes
}
