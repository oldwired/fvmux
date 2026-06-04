// Recursive directory operations for the SFTP browser. The single-file
// transfer engine in transfers.go copies one (local,remote) pair; these
// helpers fan a directory tree out into one per-file transfer each, after
// recreating the directory skeleton on the destination side. Per-file
// transfers reuse the existing Manager goroutines, progress widget, and
// destination-side auto-refresh (transferTicker), so a folder copy looks
// like a burst of file copies in the transfer pane.
package sftp

import (
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
		if err := c.MkdirAll(remoteRoot); err != nil {
			return 0, err
		}
		walkErr = filepath.WalkDir(localRoot, func(path string, d fs.DirEntry, err error) error {
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
		walkErr = walkRemote(c, remoteRoot, func(remotePath string, isDir bool) error {
			rel := strings.TrimPrefix(strings.TrimPrefix(remotePath, remoteRoot), "/")
			localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
			if isDir {
				return os.MkdirAll(localPath, 0o755)
			}
			return enqueue(Download, localPath, remotePath)
		})
	}

	// Schedule the source-tree deletion even when walkErr is non-nil or no
	// files were enqueued: a partial/empty copy that we then can't verify
	// must NOT delete the source, and finishMove enforces that by checking
	// every transfer's terminal status (an empty set deletes immediately,
	// which is correct for a fully-copied empty tree).
	if removeSourceTree != nil && walkErr == nil {
		m.wg.Add(1)
		go m.finishMove(transfers, removeSourceTree)
	}
	return len(transfers), walkErr
}

// finishMove waits for every transfer in a tree-move group to reach a
// terminal state and deletes the source tree only if all of them
// succeeded. Tracked by m.wg so the browser's teardown drain waits for
// the deletion before closing the SFTP client.
func (m *Manager) finishMove(transfers []*Transfer, removeSourceTree func() error) {
	defer m.wg.Done()
	allDone := true
	for _, t := range transfers {
		<-t.done
		if t.Status() != StatusDone {
			allDone = false
		}
	}
	if allDone {
		_ = removeSourceTree() // best effort: the copy already succeeded.
	}
}

// walkRemote recurses a remote directory depth-first, invoking fn for
// every entry (directories before their contents) so callers can create
// the destination directory before the files that land inside it. A
// ReadDir error aborts the walk and propagates.
func walkRemote(c *pkgsftp.Client, root string, fn func(path string, isDir bool) error) error {
	entries, err := c.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := root + "/" + e.Name()
		if root == "/" {
			path = "/" + e.Name()
		}
		if e.IsDir() {
			if err := fn(path, true); err != nil {
				return err
			}
			if err := walkRemote(c, path, fn); err != nil {
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
