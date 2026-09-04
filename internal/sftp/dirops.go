// Recursive directory operations for the Files window. The single-file
// transfer engine in transfers.go copies one (local,remote) pair; these
// helpers fan a directory tree out into one per-file transfer each, after
// recreating the directory skeleton on the destination side. Per-file
// transfers reuse the existing Manager goroutines, progress widget, and
// destination-side auto-refresh (transferTicker), so a folder copy looks
// like a burst of file copies in the transfer pane.
package sftp

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	pkgsftp "github.com/pkg/sftp"
)

// joinRemoteRel joins a POSIX remote root with a (possibly multi-segment)
// relative path, normalising OS separators to '/'. Unlike joinRemote it
// accepts nested rels like "a/b/c", which is what a recursive walk emits.
func joinRemoteRel(root, rel string) string {
	rel = filepath.ToSlash(rel)
	if root == "/" || root == "" {
		return "/" + rel
	}
	return root + "/" + rel
}

// StartTree mirrors a source directory onto the destination side and
// enqueues one transfer per regular file. It returns the number of files
// enqueued (directories and empty trees enqueue zero). The destination
// directory skeleton is created synchronously before any file is enqueued,
// so each file transfer always finds its parent directory in place.
//
// For Upload, localRoot is the source tree and remoteRoot the destination;
// for Download the roles swap. Symlinks are treated as their targets
// (followed), matching the file-at-a-time copy's os.Open semantics.
func (m *Manager) StartTree(c *pkgsftp.Client, dir Direction, localRoot, remoteRoot string) (int, error) {
	return m.startTree(c, dir, localRoot, remoteRoot, nil)
}

// StartTreeMove is StartTree plus a move semantic: once every enqueued
// file transfer has finished successfully, removeSourceTree is invoked
// (off a tracked goroutine) to delete the now-copied source directory. If
// any file fails or is cancelled, the source tree is left intact so the
// data is never lost mid-move.
func (m *Manager) StartTreeMove(c *pkgsftp.Client, dir Direction, localRoot, remoteRoot string, removeSourceTree func() error) (int, error) {
	return m.startTree(c, dir, localRoot, remoteRoot, removeSourceTree)
}

func (m *Manager) startTree(c *pkgsftp.Client, dir Direction, localRoot, remoteRoot string, removeSourceTree func() error) (int, error) {
	if err := m.contextErr(); err != nil {
		return 0, err
	}
	destinationKey, destinationPath := transferDestination(dir, localRoot, remoteRoot)
	treeKey := "tree\x00" + destinationKey
	if !m.claimTreeDestination(treeKey) {
		return 0, fmt.Errorf("%w: %s", ErrDestinationBusy, destinationPath)
	}
	scanLabel := localRoot + " → " + remoteRoot
	if dir == Download {
		scanLabel = remoteRoot + " → " + localRoot
	}
	m.beginScan(scanLabel)
	defer m.endScan(scanLabel)
	claimHandedOff := false
	defer func() {
		if !claimHandedOff {
			m.releaseTreeDestination(treeKey)
		}
	}()

	var transfers []*Transfer
	enqueue := func(d Direction, local, remote string) error {
		t, err := m.enqueue(c, d, local, remote, nil)
		if err != nil {
			return err
		}
		transfers = append(transfers, t)
		return nil
	}

	var walkErr error
	switch dir {
	case Upload:
		if err := m.contextErr(); err != nil {
			return 0, err
		}
		if err := c.MkdirAll(remoteRoot); err != nil {
			return 0, err
		}
		walkErr = filepath.WalkDir(localRoot, func(path string, d fs.DirEntry, err error) error {
			if cancelErr := m.contextErr(); cancelErr != nil {
				return cancelErr
			}
			if err != nil {
				return err
			}
			rel, rerr := filepath.Rel(localRoot, path)
			if rerr != nil {
				return rerr
			}
			if rel == "." {
				return nil // root already created above.
			}
			remotePath := joinRemoteRel(remoteRoot, rel)
			if d.IsDir() {
				return c.MkdirAll(remotePath)
			}
			return enqueue(Upload, path, remotePath)
		})
	case Download:
		if err := os.MkdirAll(localRoot, 0o755); err != nil {
			return 0, err
		}
		walkErr = walkRemote(m.lifetime, c, remoteRoot, func(remotePath string, isDir bool) error {
			rel := strings.TrimPrefix(strings.TrimPrefix(remotePath, remoteRoot), "/")
			localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
			if isDir {
				return os.MkdirAll(localPath, 0o755)
			}
			return enqueue(Download, localPath, remotePath)
		})
	}

	// Keep the destination claimed until every file spawned by the tree walk
	// has settled. A failed/partial walk must not delete a move source, while
	// an empty successful tree move may delete immediately.
	if walkErr != nil {
		removeSourceTree = nil
	}
	m.wg.Add(1)
	go m.finishTree(transfers, removeSourceTree, treeKey)
	claimHandedOff = true
	return len(transfers), walkErr
}

// finishTree waits for every transfer in a tree group to reach a terminal
// state, releases its root destination claim, and for a move deletes the
// source only if every copy succeeded. It is tracked by m.wg so browser
// teardown waits for both transfer completion and the optional deletion.
func (m *Manager) finishTree(transfers []*Transfer, removeSourceTree func() error, treeKey string) {
	defer m.wg.Done()
	defer m.releaseTreeDestination(treeKey)
	allDone := true
	for _, t := range transfers {
		<-t.done
		if t.Status() != StatusDone {
			allDone = false
		}
	}
	if allDone && removeSourceTree != nil {
		_ = removeSourceTree() // best effort: the copy already succeeded.
	}
}

// walkRemote recurses a remote directory depth-first, invoking fn for
// every entry (directories before their contents) so callers can create
// the destination directory before the files that land inside it. A
// ReadDir error aborts the walk and propagates.
func walkRemote(ctx context.Context, c *pkgsftp.Client, root string, fn func(path string, isDir bool) error) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	entries, err := c.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		path := joinRemote(root, e.Name())
		if e.IsDir() {
			if err := fn(path, true); err != nil {
				return err
			}
			if err := walkRemote(ctx, c, path, fn); err != nil {
				return err
			}
			continue
		}
		if err := fn(path, false); err != nil {
			return err
		}
	}
	return nil
}
