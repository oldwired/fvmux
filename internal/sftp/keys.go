package sftp

import (
	"os"
	"path/filepath"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// keyHandler is an invisible OfPreProcess view installed inside the
// browser dialog. It intercepts:
//
//   - Enter        — when focus is in a listing, dive into a folder /
//     parent row. The listing's TreeView would otherwise
//     try to toggle children (no-op for leaf-only
//     listings, but consuming the event keeps it tidy).
//   - F5 / F6      — copy the focused listing's highlighted file to
//     the other side's cwd. Direction is derived from
//     which side has focus.
//   - Del          — cancel the most recent in-flight transfer.
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

// HandleEvent intercepts the listing-relevant keys; everything else
// (including Tab) passes through untouched.
func (h *keyHandler) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbEnter:
		// Enter is only meaningful for listings (cd). Trees use Enter
		// for expand/collapse via TreeView's own handler — leave alone.
		if p := h.focusedListingPanel(); p != nil {
			p.listingEnter()
			ev.What = consts.EvNothing
		}
	case consts.KbF5, consts.KbF6:
		h.copyAcross()
		ev.What = consts.EvNothing
	case consts.KbDel:
		if h.mgr.CancelLast() {
			ev.What = consts.EvNothing
		}
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

// copyAcross transfers the focused listing's selected file to the
// other panel's cwd. Errors:
//
//   - focus is on a tree, not a listing → ask the user to pick a file.
//   - selected row is a folder / parent → same message.
//   - source file is missing locally → surface the os.Stat error.
func (h *keyHandler) copyAcross() {
	active, other, listingFocused := h.focusedSide()
	if active == nil || other == nil {
		return
	}
	if !listingFocused {
		msgbox.Show(&h.app.Desktop.Group, msgbox.Info,
			"Highlight a file in either listing, then F5/F6 to copy.",
			msgbox.OKOnly)
		return
	}
	e, _ := active.activeSelection()
	if e == nil || e.IsDir || e.Parent {
		msgbox.Show(&h.app.Desktop.Group, msgbox.Info,
			"Highlight a file (not a directory) to copy.",
			msgbox.OKOnly)
		return
	}

	if active.isRemote {
		// Remote → local download.
		target := filepath.Join(other.cwd, filepath.Base(e.Path))
		if _, err := h.mgr.Start(active.c, Download, target, e.Path); err != nil {
			msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
				"Download failed: %s", []any{err.Error()}, msgbox.OKOnly)
		}
		return
	}
	// Local → remote upload.
	if _, err := os.Stat(e.Path); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Can't read %s: %s", []any{e.Path, err.Error()}, msgbox.OKOnly)
		return
	}
	remote := other.cwd + "/" + filepath.Base(e.Path)
	if other.cwd == "/" {
		remote = "/" + filepath.Base(e.Path)
	}
	if _, err := h.mgr.Start(other.c, Upload, e.Path, remote); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Upload failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}
