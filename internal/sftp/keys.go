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
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"
)

// keyHandler is an invisible OfPreProcess view installed inside the
// SFTP browser dialog. It captures F5 (download focused remote file),
// F6 (upload local file), Del (cancel last transfer). Other keys
// pass through untouched to the tree / preview.
type keyHandler struct {
	views.Base

	app      *fvapp.Application
	c        *pkgsftp.Client
	mgr      *Manager
	tree     *treeview.TreeView
	remoteCw string
}

func newKeyHandler(a *fvapp.Application, c *pkgsftp.Client, mgr *Manager, tree *treeview.TreeView, remoteCw string) *keyHandler {
	h := &keyHandler{
		Base:     views.NewBase(geom.NewRect(0, 0, 0, 0)),
		app:      a,
		c:        c,
		mgr:      mgr,
		tree:     tree,
		remoteCw: remoteCw,
	}
	h.SetSelf(h)
	h.Options |= consts.OfPreProcess
	return h
}

// GetTypeID for serial registry.
func (h *keyHandler) GetTypeID() string { return "sftpkeys" }

// Draw is a no-op — the view is invisible.
func (h *keyHandler) Draw() {}

// HandleEvent intercepts F5/F6/Del; other events pass through.
func (h *keyHandler) HandleEvent(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbF5:
		h.downloadFocused()
		ev.What = consts.EvNothing
	case consts.KbF6:
		h.uploadPrompt()
		ev.What = consts.EvNothing
	case consts.KbDel:
		if h.mgr.CancelLast() {
			ev.What = consts.EvNothing
		}
	}
}

func (h *keyHandler) downloadFocused() {
	n := h.tree.CurrentNode()
	if n == nil {
		return
	}
	e, ok := n.Data.(*fileEntry)
	if !ok || e.IsDir {
		msgbox.Show(&h.app.Desktop.Group, msgbox.Info,
			"Select a file (not a directory) to download.", msgbox.OKOnly)
		return
	}
	def := filepath.Join(defaultLocalDir(), filepath.Base(e.Path))
	target, ok := promptPath(h.app, "Download", "Save to:", def)
	if !ok {
		return
	}
	if _, err := h.mgr.Start(h.c, Download, target, e.Path); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Download failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}

func (h *keyHandler) uploadPrompt() {
	src, ok := promptPath(h.app, "Upload", "Local file to upload:", "")
	if !ok || src == "" {
		return
	}
	if _, err := os.Stat(src); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Can't read %s: %s", []any{src, err.Error()}, msgbox.OKOnly)
		return
	}
	remote := h.remoteCw + "/" + filepath.Base(src)
	if h.remoteCw == "/" {
		remote = "/" + filepath.Base(src)
	}
	if _, err := h.mgr.Start(h.c, Upload, src, remote); err != nil {
		msgbox.Showf(&h.app.Desktop.Group, msgbox.Error,
			"Upload failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}

// promptPath opens a centred modal InputLine and returns the entered
// path. Mirrors fvmux/internal/app.promptString — duplicated rather
// than imported to avoid a package cycle.
func promptPath(a *fvapp.Application, title, label, initial string) (string, bool) {
	desk := a.Desktop.BaseView()
	w, h := 60, 8
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2

	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), title)

	il := dialogs.NewInputLine(geom.NewRect(2, 4, w-3, 5), 1024)
	il.SetText(initial)
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 2, w-3, 3), label, il))
	d.Insert(il)

	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-12, h-3, w/2-2, h-2),
		"O~K~", consts.CmOK, dialogs.BfDefault,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2+2, h-3, w/2+12, h-2),
		"~C~ancel", consts.CmCancel, 0,
	))

	if a.Desktop.ExecView(d) != consts.CmOK {
		return "", false
	}
	return il.Text(), true
}

// defaultLocalDir returns the user's home directory, or "." as a
// safe fallback.
func defaultLocalDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
