// Mutating actions for the SFTP browser following the Norton /
// Midnight Commander convention: F6 move/rename (see move.go), F7
// mkdir, F8 delete.
// Each action determines the target side from focus, prompts the user
// (input dialog for names, YesNo for delete), runs the local-FS or
// SFTP call, and refreshes the affected panel's listing.
//
// promptName is local to this package because the sftp package can't
// import internal/app (the dependency runs the other direction).
package sftp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
)

// joinRemote concatenates a POSIX remote directory with a basename,
// handling the root-dir special case ("/") so we don't emit "//name".
func joinRemote(cwd, name string) string {
	if cwd == "/" || cwd == "" {
		return "/" + name
	}
	return cwd + "/" + name
}

// validateBasename returns a non-nil error if name is empty, only
// whitespace, or contains a path separator. Used by mkdir and by F6's
// in-place rename to keep a bare name inside the panel's current
// directory — a path entry in the F6 dialog is a cross-host move instead.
func validateBasename(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("name cannot contain '/' or '\\'")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("name cannot be '.' or '..'")
	}
	return nil
}

// promptName opens a centered modal InputLine seeded with initial.
// Returns the entered text + true on OK, or "", false on Cancel/Esc.
// Mirrors internal/app.promptString — kept in step intentionally; the
// sftp package can't import internal/app since app already imports
// sftp (and adding a third package just for one shared helper is more
// scaffolding than it's worth).
func promptName(a *fvapp.Application, title, label, initial string) (string, bool) {
	desk := a.Desktop.BaseView()
	w, h := 54, 8
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2

	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), title)

	il := dialogs.NewInputLine(geom.NewRect(2, 4, w-3, 5), 256)
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
	return strings.TrimSpace(il.Text()), true
}

// mkdirAction (F7) creates a new directory under p's current folder.
// Empty input is treated as cancel. Uses MkdirAll so the user can
// type "a/b/c" if they ever want nested creation — though the basename
// validator currently forbids slashes, keep MkdirAll for forward
// flexibility (it's harmless on a single segment).
func mkdirAction(a *fvapp.Application, p *panel) {
	if p == nil {
		msgbox.Show(&a.Desktop.Group, msgbox.Info,
			"Focus the remote or local side first.", msgbox.OKOnly)
		return
	}
	name, ok := promptName(a, "Make Directory",
		"New folder name (in "+p.cwd+"):", "")
	if !ok {
		return
	}
	if err := validateBasename(name); err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"%s.", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	var err error
	if p.isRemote {
		err = p.c.MkdirAll(joinRemote(p.cwd, name))
	} else {
		err = os.MkdirAll(filepath.Join(p.cwd, name), 0o755)
	}
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"mkdir failed: %s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	p.refresh()
}

// deleteAction (F8) recursively deletes the listing's current
// selection after a YesNo confirm. Recursive removal is the
// NC/MC-Far convention — a separate "are you sure?" prompt is the
// guard. Directory removals can fail partway on slow links; we
// always refresh after so the user sees whatever did get removed.
func deleteAction(a *fvapp.Application, p *panel, listingFocused bool) {
	if p == nil || !listingFocused {
		msgbox.Show(&a.Desktop.Group, msgbox.Info,
			"Highlight a file or folder in a listing to delete.",
			msgbox.OKOnly)
		return
	}
	e := currentListingEntry(p)
	if e == nil {
		return
	}
	if e.Parent {
		msgbox.Show(&a.Desktop.Group, msgbox.Info,
			"Can't delete the '../' row.", msgbox.OKOnly)
		return
	}

	kind := "file"
	if e.IsDir {
		kind = "folder (recursive)"
	}
	side := "remote"
	if e.Local {
		side = "local"
	}
	answer := msgbox.Showf(&a.Desktop.Group, msgbox.Question,
		"Delete %s %s\n%s ?",
		[]any{side, kind, e.Path}, msgbox.YesNo)
	if answer != consts.CmYes {
		return
	}

	var err error
	if e.Local {
		err = os.RemoveAll(e.Path)
	} else {
		err = p.c.RemoveAll(e.Path)
	}
	// Refresh either way: a partial delete still changes the listing.
	p.refresh()
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"delete failed: %s", []any{err.Error()}, msgbox.OKOnly)
	}
}

// currentListingEntry returns the fileEntry under the listing's
// current cursor, or nil if there isn't one. Common to rename + delete.
func currentListingEntry(p *panel) *fileEntry {
	if p == nil || p.listing == nil {
		return nil
	}
	n := p.listing.CurrentNode()
	if n == nil {
		return nil
	}
	e, _ := n.Data.(*fileEntry)
	return e
}
