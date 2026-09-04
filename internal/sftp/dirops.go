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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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
// for Download the roles swap. Directory uploads reject symlinks and all
// local tree access is rooted so concurrent link swaps cannot escape the
// selected directory.
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
	baseCtx := m.lifetime
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	scanCtx, cancelScan := context.WithTimeout(baseCtx, maxTreeScanDuration)
	defer cancelScan()
	treeClient := c
	var treeConn dedicatedConn
	treeConnHandedOff := false
	defer func() {
		if treeConn != nil && !treeConnHandedOff {
			_ = treeConn.Close()
		}
	}()
	if m.openDedicated != nil {
		var err error
		treeConn, err = m.openDedicated()
		if err != nil {
			return 0, err
		}
		if treeConn == nil {
			return 0, errors.New("opening operation-owned SFTP session returned nil")
		}
		treeClient = treeConn.SFTP()
	}
	// Production tree operations own one SFTP transport. If the scan deadline
	// expires while a server withholds a Mkdir response, closing that transport
	// interrupts the call; after enumeration completes, the timer is disarmed
	// and the same transport remains alive until its file transfers settle.
	var stopScanClose func() bool
	if treeConn != nil {
		stopScanClose = context.AfterFunc(scanCtx, func() { _ = treeConn.Close() })
		defer stopScanClose()
	}
	var rootedLocal *os.Root
	rootHandedOff := false
	defer func() {
		if rootedLocal != nil && !rootHandedOff {
			_ = rootedLocal.Close()
		}
	}()
	enqueue := func(d Direction, local, remote, rel string) error {
		t, err := m.enqueueRooted(treeClient, d, local, remote, nil, rootedLocal, rel, false)
		if err != nil {
			return err
		}
		transfers = append(transfers, t)
		return nil
	}

	var walkErr error
	switch dir {
	case Upload:
		var err error
		rootedLocal, err = openSecureLocalRoot(localRoot)
		if err != nil {
			return 0, err
		}
		if err := treeClient.MkdirAll(remoteRoot); err != nil {
			return 0, err
		}
		visited := 0
		walkErr = fs.WalkDir(rootedLocal.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
			if cancelErr := scanCtx.Err(); cancelErr != nil {
				return cancelErr
			}
			if err != nil {
				return err
			}
			if rel == "." {
				return nil // root already created above.
			}
			visited++
			if visited > maxTreeEntries {
				return fmt.Errorf("%w: more than %d entries", ErrRemoteTreeLimit, maxTreeEntries)
			}
			if depth := strings.Count(rel, "/") + 1; depth > maxTreeDepth {
				return fmt.Errorf("%w: depth exceeds %d", ErrRemoteTreeLimit, maxTreeDepth)
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: %s", ErrUnsafeLocalLink, rel)
			}
			remotePath := joinRemoteRel(remoteRoot, rel)
			if d.IsDir() {
				return treeClient.MkdirAll(remotePath)
			}
			info, statErr := rootedLocal.Lstat(filepath.FromSlash(rel))
			if statErr != nil {
				return statErr
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: %s", ErrUnsafeLocalLink, rel)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported non-regular upload entry: %s", rel)
			}
			localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
			return enqueue(Upload, localPath, remotePath, filepath.FromSlash(rel))
		})
	case Download:
		var err error
		destinationParent := filepath.Dir(localRoot)
		destinationBase := filepath.Base(localRoot)
		parentRoot, err := openSecureLocalRoot(destinationParent)
		if err != nil {
			return 0, err
		}
		if err := parentRoot.MkdirAll(destinationBase, 0o755); err != nil {
			_ = parentRoot.Close()
			return 0, err
		}
		if info, err := parentRoot.Lstat(destinationBase); err != nil {
			_ = parentRoot.Close()
			return 0, err
		} else if info.Mode()&os.ModeSymlink != 0 {
			_ = parentRoot.Close()
			return 0, fmt.Errorf("%w: %s", ErrUnsafeLocalLink, localRoot)
		}
		if err := parentRoot.Close(); err != nil {
			return 0, err
		}
		// Anchor subsequent work at the selected destination itself, not its
		// parent. A relative link such as destination/.ssh -> ../.ssh would stay
		// inside the parent root while still escaping the selected tree.
		rootedLocal, err = openSecureLocalRoot(localRoot)
		if err != nil {
			return 0, err
		}
		walkErr = walkRemote(scanCtx, m.remoteDirectoryReader(c), remoteRoot, func(rel, remotePath string, isDir bool) error {
			localRel := filepath.FromSlash(rel)
			localPath := filepath.Join(localRoot, localRel)
			if isDir {
				return rootedLocal.MkdirAll(localRel, 0o755)
			}
			return enqueue(Download, localPath, remotePath, localRel)
		})
	}

	// Keep the destination claimed until every file spawned by the tree walk
	// has settled. A failed/partial walk must not delete a move source, while
	// an empty successful tree move may delete immediately.
	if walkErr != nil {
		removeSourceTree = nil
	}
	if stopScanClose != nil {
		stopScanClose()
	}
	m.wg.Add(1)
	go m.finishTree(transfers, removeSourceTree, treeKey, rootedLocal, treeConn)
	rootHandedOff = true
	treeConnHandedOff = true
	claimHandedOff = true
	return len(transfers), walkErr
}

// finishTree waits for every transfer in a tree group to reach a terminal
// state, releases its root destination claim, and for a move deletes the
// source only if every copy succeeded. It is tracked by m.wg so browser
// teardown waits for both transfer completion and the optional deletion.
func (m *Manager) finishTree(transfers []*Transfer, removeSourceTree func() error, treeKey string, rootedLocal *os.Root, treeConn dedicatedConn) {
	defer m.wg.Done()
	defer m.releaseTreeDestination(treeKey)
	allDone := true
	for _, t := range transfers {
		<-t.done
		if t.Status() != StatusDone {
			allDone = false
		}
	}
	if rootedLocal != nil {
		_ = rootedLocal.Close()
	}
	if treeConn != nil {
		_ = treeConn.Close()
	}
	if allDone && removeSourceTree != nil {
		_ = removeSourceTree() // best effort: the copy already succeeded.
	}
}

type remoteDirectoryReader func(context.Context, string) (remoteDirectory, error)

func (m *Manager) remoteDirectoryReader(c *pkgsftp.Client) remoteDirectoryReader {
	if m.readRemoteDirectory != nil {
		return m.readRemoteDirectory
	}
	return func(ctx context.Context, remotePath string) (remoteDirectory, error) {
		entries, err := c.ReadDirContext(ctx, remotePath)
		if err != nil {
			return remoteDirectory{}, err
		}
		return sanitizeRemoteDirectory(entries, false), nil
	}
}

// walkRemote uses an explicit stack and finite entry, directory, depth, and
// time budgets. That keeps attacker-controlled hierarchy depth off the Go
// call stack and makes every production ReadDir cancellable and byte-bounded.
func walkRemote(ctx context.Context, readDir remoteDirectoryReader, root string, fn func(rel, remotePath string, isDir bool) error) error {
	type pendingDir struct {
		remotePath string
		rel        string
		depth      int
	}
	stack := []pendingDir{{remotePath: root}}
	entriesSeen, directoriesSeen := 0, 0
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		directoriesSeen++
		if directoriesSeen > maxTreeDirectories {
			return fmt.Errorf("%w: more than %d directories", ErrRemoteTreeLimit, maxTreeDirectories)
		}
		directory, err := readDir(ctx, current.remotePath)
		if err != nil {
			return err
		}
		if err := directory.transferError(current.remotePath); err != nil {
			return err
		}
		for _, entry := range directory.entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			entriesSeen++
			if entriesSeen > maxTreeEntries {
				return fmt.Errorf("%w: more than %d entries", ErrRemoteTreeLimit, maxTreeEntries)
			}
			name := entry.Name()
			if err := validateRemoteEntryName(name); err != nil {
				return err
			}
			childRel := name
			if current.rel != "" {
				childRel = path.Join(current.rel, name)
			}
			childRemote := joinRemote(current.remotePath, name)
			if entry.IsDir() {
				depth := current.depth + 1
				if depth > maxTreeDepth {
					return fmt.Errorf("%w: depth exceeds %d", ErrRemoteTreeLimit, maxTreeDepth)
				}
				if err := fn(childRel, childRemote, true); err != nil {
					return err
				}
				stack = append(stack, pendingDir{remotePath: childRemote, rel: childRel, depth: depth})
				continue
			}
			if err := fn(childRel, childRemote, false); err != nil {
				return err
			}
		}
	}
	return nil
}
