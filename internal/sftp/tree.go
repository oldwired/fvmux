// Folders-only tree builders. The dual-pane browser uses these for
// the upper-left + lower-left navigation trees; files are deliberately
// omitted because each tree has a paired listing pane that handles
// per-folder content.
package sftp

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"
)

// buildRemoteTree returns folders only at the SFTP path. Files are
// skipped — the listing pane on the right of the tree owns those.
func buildRemoteTree(s *pkgsftp.Client, cwd string) []*treeview.Node {
	entries, err := s.ReadDir(cwd)
	if err != nil {
		return []*treeview.Node{
			{Label: "<error: " + err.Error() + ">"},
		}
	}
	return buildRemoteTreeEntries(cwd, sanitizeRemoteDirectory(entries, false))
}

func buildRemoteTreeEntries(cwd string, directory remoteDirectory) []*treeview.Node {
	dirs := make([]os.FileInfo, 0, len(directory.entries))
	for _, e := range directory.entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })

	out := make([]*treeview.Node, 0, len(dirs))
	for _, e := range dirs {
		out = append(out, &treeview.Node{
			Label:       e.Name() + "/",
			Data:        &fileEntry{Path: joinRemote(cwd, e.Name()), IsDir: true},
			HasChildren: true,
		})
	}
	if directory.rejected > 0 {
		out = append(out, &treeview.Node{Label: "<unsafe remote names omitted>"})
	}
	if directory.truncated {
		out = append(out, &treeview.Node{Label: "<directory limit reached>"})
	}
	return out
}

// Remote tree expansion is asynchronous — see panel.expandRemoteAsync.
// A synchronous ReadDir here would block the UI event goroutine for the
// length of a slow-link round-trip or a dead-host TCP timeout, freezing
// every window the instant the user expands a collapsed remote folder.
// The local mirror below stays synchronous: the local FS doesn't block.

// buildLocalTree is the local-FS mirror of buildRemoteTree.
func buildLocalTree(dir string) []*treeview.Node {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []*treeview.Node{
			{Label: "<error: " + err.Error() + ">"},
		}
	}
	dirs := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })

	out := make([]*treeview.Node, 0, len(dirs))
	for _, e := range dirs {
		path := filepath.Join(dir, e.Name())
		out = append(out, &treeview.Node{
			Label:       e.Name() + string(filepath.Separator),
			Data:        &fileEntry{Path: path, IsDir: true, Local: true},
			HasChildren: true,
		})
	}
	return out
}

// expandLocalTree lazily populates a local folder node's child folders.
// Stays synchronous (unlike the remote side's expandRemoteAsync): the
// local FS doesn't block the UI goroutine. No-op for already-expanded or
// non-directory nodes.
func expandLocalTree(n *treeview.Node) {
	if n == nil || len(n.Children) > 0 {
		return
	}
	e, ok := n.Data.(*fileEntry)
	if !ok || !e.IsDir {
		return
	}
	for _, child := range buildLocalTree(e.Path) {
		child.Parent = n
		n.Children = append(n.Children, child)
	}
}
