// F6 Move / Rename, following the Midnight Commander convention: the
// prompt is pre-filled with the *other* panel's directory, so the
// zero-edit default moves the selection across panes. Two behaviours
// fall out of what the user types:
//
//   - a bare name (no path separator) renames the selection in place,
//     within the source side's own directory — the rename special case;
//   - anything with a separator is a path on the *other* panel's host,
//     so the move crosses hosts (local↔remote) as a copy followed by a
//     delete of the source once the copy has fully succeeded.
//
// Because an fvmux browser always pairs one remote panel with one local
// panel, "the other panel's host" is always the opposite transport, so a
// path move is always a cross-host copy+delete. Same-host moves across
// directories aren't expressible here (a bare name stays in the source
// dir); that's a deliberate simplification, not an oversight.
package sftp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"

	pkgsftp "github.com/pkg/sftp"
)

// moveSelected (F6) moves or renames the focused listing's selection.
func (h *keyHandler) moveSelected() {
	active, listingFocused := h.strictFocusedPanel()
	if active == nil || !listingFocused {
		msgbox.Show(&h.app.Desktop.Group, msgbox.Info,
			"Highlight a file or folder in a listing to move or rename.",
			msgbox.OKOnly)
		return
	}
	e := currentListingEntry(active)
	if e == nil {
		return
	}
	if e.Parent {
		msgbox.Show(&h.app.Desktop.Group, msgbox.Info,
			"Can't move the '../' row.", msgbox.OKOnly)
		return
	}
	other := h.opposite(active)
	name := filepath.Base(e.Path)

	prefill := name
	if other != nil {
		if other.isRemote {
			prefill = joinRemote(other.cwd, name)
		} else {
			prefill = filepath.Join(other.cwd, name)
		}
	}
	dest, ok := promptName(h.app, "Move / Rename",
		fmt.Sprintf("Move %q to (a bare name renames it in place):", name), prefill)
	if !ok || dest == "" {
		return
	}

	// A bare name (no separator) renames within the source's own folder.
	if !strings.ContainsAny(dest, "/\\") {
		h.renameInPlace(active, e, dest)
		return
	}
	h.moveToOther(active, other, e, dest)
}

// renameInPlace renames e to a bare newName inside p's current directory,
// on whichever side p lives. Files and directories rename identically.
func (h *keyHandler) renameInPlace(p *panel, e *fileEntry, newName string) {
	if err := validateBasename(newName); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"%s.", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	if newName == filepath.Base(e.Path) {
		return // no-op rename.
	}
	if e.Local {
		if err := os.Rename(e.Path, filepath.Join(p.cwd, newName)); err != nil {
			msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
				"rename failed: %s", []any{err.Error()}, msgbox.OKOnly)
			return
		}
		p.refresh()
		return
	}
	dest := joinRemote(p.cwd, newName)
	if !p.asyncRemoteOpKey("rename:"+e.Path,
		func() error { return p.c.Rename(e.Path, dest) },
		func(err error) {
			if err != nil {
				msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
					"rename failed: %s", []any{err.Error()}, msgbox.OKOnly)
				return
			}
			p.refresh()
		}) {
		showOperationInProgress(h.app, e.Path)
	}
}

// moveToOther moves e onto the other panel's host at dest (a cross-host
// copy + source delete). Direction follows which side is remote.
func (h *keyHandler) moveToOther(active, other *panel, e *fileEntry, dest string) {
	if other == nil {
		return
	}
	// refreshBoth re-reads both panels once the source has been removed,
	// so the moved entry vanishes from the source listing and appears in
	// the destination listing (the transferTicker also refreshes the
	// destination per file — a harmless overlap).
	refreshBoth := func() {
		views.CallSoon(func() {
			active.refresh()
			other.refresh()
		})
	}

	// Every remote round-trip below (Stat existence checks, the
	// StartTree* walks, StartDedicated's session open) runs through
	// h.async — never on the UI event goroutine (#13). Confirmation
	// dialogs stay on the UI goroutine, between the async legs.
	if active.isRemote {
		// Remote → local: download, then delete the remote source.
		local := filepath.Clean(dest)
		rm := func() error {
			var err error
			if e.IsDir {
				err = active.c.RemoveAll(e.Path)
			} else {
				err = active.c.Remove(e.Path)
			}
			refreshBoth()
			return err
		}
		if e.IsDir {
			if h.localExists(local) && !h.confirmMerge(local) {
				return
			}
			h.async(func() error {
				_, err := h.mgr.StartTreeMove(active.c, Download, local, e.Path, rm)
				return err
			}, h.moveDone)
			return
		}
		if h.localExists(local) && !h.confirmOverwrite(local) {
			return
		}
		h.async(func() error {
			_, err := h.mgr.StartDedicated(active.c, Download, local, e.Path, rm)
			return err
		}, h.moveDone)
		return
	}

	// Local → remote: upload, then delete the local source.
	remote := dest
	rm := func() error {
		err := os.RemoveAll(e.Path) // RemoveAll handles both file and dir.
		refreshBoth()
		return err
	}
	var exists bool
	h.async(func() error {
		exists = h.remoteExists(other.c, remote)
		return nil
	}, func(error) {
		if e.IsDir {
			if exists && !h.confirmMerge(remote) {
				return
			}
			h.async(func() error {
				_, err := h.mgr.StartTreeMove(other.c, Upload, e.Path, remote, rm)
				return err
			}, h.moveDone)
			return
		}
		if exists && !h.confirmOverwrite(remote) {
			return
		}
		h.async(func() error {
			_, err := h.mgr.StartDedicated(other.c, Upload, e.Path, remote, rm)
			return err
		}, h.moveDone)
	})
}

func (h *keyHandler) localExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (h *keyHandler) remoteExists(c *pkgsftp.Client, p string) bool {
	_, err := c.Stat(p)
	return err == nil
}

func (h *keyHandler) moveErr(err error) {
	h.reportTransferError("Move", err)
}

// moveDone is the async completion for the move legs: surfaces the
// error, if any (successful moves refresh via refreshBoth/transferTicker).
func (h *keyHandler) moveDone(err error) {
	if err != nil {
		h.moveErr(err)
	}
}
