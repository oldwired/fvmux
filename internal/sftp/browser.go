package sftp

import (
	"fmt"
	"io"
	"sort"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hexedit"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"
)

const (
	maxPreviewBytes = 64 * 1024
)

// Show opens an SFTP browser modal against alias. Step 10 ships a
// single-pane remote tree with a read-only preview area: text /
// markdown / hex depending on the sniffed media kind. Transfer queue,
// local panel, and image preview are sub-step 12 polish.
func Show(a *fvapp.Application, alias string) {
	c, err := Open(alias)
	if err != nil {
		msgbox.Showf(&a.Desktop.Group, msgbox.Error,
			"Couldn't open SFTP to %s:\n%s",
			[]any{alias, err.Error()}, msgbox.OKOnly)
		return
	}
	defer c.Close()

	cwd, err := c.SFTP().Getwd()
	if err != nil {
		cwd = "/"
	}

	desk := a.Desktop.BaseView()
	w, h := 100, 28
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "SFTP — "+alias+" — "+cwd)

	treeW := w / 2
	tree := treeview.New(geom.NewRect(2, 2, treeW-1, h-3), buildRoots(c.SFTP(), cwd))
	tree.OnExpand = func(n *treeview.Node) {
		expandNode(c.SFTP(), n)
	}
	d.Insert(tree)

	previewBounds := geom.NewRect(treeW+1, 2, w-2, h-3)
	mv := markdown.New(previewBounds, nil)
	mv.SetMarkdown("# SFTP\n\nSelect a file in the tree to preview.")
	d.Insert(mv)

	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-5, h-3, w/2+5, h-2),
		"Cl~o~se", consts.CmCancel, dialogs.BfDefault,
	))

	a.Desktop.ExecView(d)
}

// fileEntry is what we stash on each tree Node so OnExpand / preview
// can find the path back.
type fileEntry struct {
	Path  string
	IsDir bool
}

func buildRoots(s *pkgsftp.Client, cwd string) []*treeview.Node {
	entries, err := s.ReadDir(cwd)
	if err != nil {
		return []*treeview.Node{
			{Label: "<error: " + err.Error() + ">"},
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})
	out := make([]*treeview.Node, 0, len(entries))
	for _, e := range entries {
		path := cwd + "/" + e.Name()
		if cwd == "/" {
			path = "/" + e.Name()
		}
		label := e.Name()
		if e.IsDir() {
			label += "/"
		} else {
			label += fmt.Sprintf("  (%d bytes)", e.Size())
		}
		out = append(out, &treeview.Node{
			Label:       label,
			Data:        &fileEntry{Path: path, IsDir: e.IsDir()},
			HasChildren: e.IsDir(),
		})
	}
	return out
}

func expandNode(s *pkgsftp.Client, n *treeview.Node) {
	if n == nil {
		return
	}
	if len(n.Children) > 0 {
		return // already populated.
	}
	entry, ok := n.Data.(*fileEntry)
	if !ok || !entry.IsDir {
		return
	}
	for _, child := range buildRoots(s, entry.Path) {
		child.Parent = n
		n.Children = append(n.Children, child)
	}
}

// PreviewMarkdown reads up to maxPreviewBytes from s at path and
// returns a markdown string suitable for MarkdownView (text/markdown
// preview) plus a hexedit.DataSource (when the file is binary/image).
// Exactly one of the returned values is non-zero.
//
// Sub-step 12 wires this into the TreeView's selection so the preview
// pane updates on every focus change; step 10 leaves the helper public
// so callers and tests can exercise it.
func PreviewMarkdown(s *pkgsftp.Client, path string) (string, hexedit.DataSource) {
	f, err := s.Open(path)
	if err != nil {
		return fmt.Sprintf("# Error\n\n%s", err.Error()), nil
	}
	defer f.Close()
	buf := make([]byte, maxPreviewBytes)
	n, err := io.ReadFull(f, buf)
	if n == 0 && err != nil {
		return fmt.Sprintf("# Error\n\n%s", err.Error()), nil
	}
	buf = buf[:n]
	switch Sniff(path, buf) {
	case KindMarkdown:
		return string(buf), nil
	case KindBinary, KindImage:
		src := hexedit.NewMemorySource(buf)
		src.SetReadOnly(true)
		return "", src
	default:
		return "```\n" + string(buf) + "\n```", nil
	}
}
