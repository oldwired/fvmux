// Mutating actions for the SFTP browser following the Norton /
// Midnight Commander convention: F6 move/rename (see move.go), F7
// mkdir, F8 delete.
// Each action determines the target side from focus, prompts the user
// (input dialog for names, YesNo for delete), runs the local-FS or
// SFTP call, and refreshes the affected panel's listing.
package sftp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/ui"
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

// promptName wraps the shared ui.PromptString modal and trims the
// result: SFTP mkdir and move/rename want a clean basename, never one
// carrying leading or trailing spaces. Returns "", false on Cancel/Esc.
func promptName(a *fvapp.Application, title, label, initial string) (string, bool) {
	s, ok := ui.PromptString(a, title, label, initial)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
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
	if p.isRemote {
		// Off the UI goroutine — a slow link must not freeze the
		// multiplexer for the length of a remote round-trip.
		path := joinRemote(p.cwd, name)
		p.asyncRemoteOp(
			func() error { return p.c.MkdirAll(path) },
			func(err error) {
				if err != nil {
					msgbox.Showf(&a.Desktop.Group, msgbox.Error,
						"mkdir failed: %s", []any{err.Error()}, msgbox.OKOnly)
					return
				}
				p.refresh()
			})
		return
	}
	if err := os.MkdirAll(filepath.Join(p.cwd, name), 0o755); err != nil {
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

	if e.Local {
		err := os.RemoveAll(e.Path)
		// Refresh either way: a partial delete still changes the listing.
		p.refresh()
		if err != nil {
			msgbox.Showf(&a.Desktop.Group, msgbox.Error,
				"delete failed: %s", []any{err.Error()}, msgbox.OKOnly)
		}
		return
	}
	// Remote recursive delete walks the tree server-round-trip by
	// round-trip — run it off the UI goroutine so a slow or dropped
	// link can't freeze every window until the TCP timeout.
	p.asyncRemoteOp(
		func() error { return p.c.RemoveAll(e.Path) },
		func(err error) {
			p.refresh()
			if err != nil {
				msgbox.Showf(&a.Desktop.Group, msgbox.Error,
					"delete failed: %s", []any{err.Error()}, msgbox.OKOnly)
			}
		})
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
