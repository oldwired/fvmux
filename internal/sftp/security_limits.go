package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"
)

// These are security limits, not performance knobs. They keep one
// authenticated but hostile SFTP server from making fvmux retain an
// unbounded directory, traversal, or transfer queue.
const (
	maxRemoteDirectoryBytes      int64 = 4 << 20
	maxRemoteEntriesPerDirectory       = 4096
	maxRemoteTreeNodes                 = 16384
	maxTreeEntries                     = 10000
	maxTreeDepth                       = 64
	maxTreeDirectories                 = 4096
	maxQueuedTransfers                 = 256
	maxRetainedTransfers               = 512
	maxConcurrentDirectoryReads        = 2
	remoteDirectoryTimeout             = 30 * time.Second
	maxTreeScanDuration                = 5 * time.Minute
)

var (
	ErrRemoteDirectoryLimit = errors.New("remote directory exceeds the safety limit")
	ErrRemoteTreeLimit      = errors.New("transfer tree exceeds the safety limit")
	ErrUnsafeRemoteName     = errors.New("unsafe remote entry name")
	ErrUnsafeLocalLink      = errors.New("unsafe local symbolic link or path replacement")
)

// remoteDirectory is the bounded, validated result consumed by every
// production directory view and recursive remote walk.
type remoteDirectory struct {
	entries   []os.FileInfo
	truncated bool
	rejected  int
}

func (d remoteDirectory) transferError(remotePath string) error {
	switch {
	case d.rejected > 0:
		return fmt.Errorf("%w below %s", ErrUnsafeRemoteName, remotePath)
	case d.truncated:
		return fmt.Errorf("%w at %s", ErrRemoteDirectoryLimit, remotePath)
	default:
		return nil
	}
}

// validateRemoteEntryName requires one portable path segment. pkg/sftp uses
// POSIX path.Base, which does not remove Windows backslashes; rejecting both
// separators here prevents a remote name from acquiring local path meaning
// after filepath.FromSlash.
func validateRemoteEntryName(name string) error {
	if name == "" || name == "." || name == ".." || path.Base(name) != name ||
		strings.ContainsAny(name, "/\\:\x00") || strings.HasSuffix(name, ".") ||
		strings.HasSuffix(name, " ") || isWindowsDeviceName(name) {
		return fmt.Errorf("%w: %q", ErrUnsafeRemoteName, name)
	}
	return nil
}

func isWindowsDeviceName(name string) bool {
	base := name
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	base = strings.ToUpper(strings.TrimRight(base, " ."))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		base[3] >= '1' && base[3] <= '9'
}

func sanitizeRemoteDirectory(entries []os.FileInfo, byteLimited bool) remoteDirectory {
	result := remoteDirectory{truncated: byteLimited}
	limit := len(entries)
	if limit > maxRemoteEntriesPerDirectory {
		limit = maxRemoteEntriesPerDirectory
		result.truncated = true
	}
	result.entries = make([]os.FileInfo, 0, limit)
	for _, entry := range entries {
		if len(result.entries) >= limit {
			break
		}
		if entry == nil || validateRemoteEntryName(entry.Name()) != nil {
			result.rejected++
			continue
		}
		result.entries = append(result.entries, entry)
	}
	return result
}

// byteBudgetReader puts a hard ceiling on raw SFTP response bytes. The
// dependency's ReadDirContext otherwise accumulates NAME packets until EOF.
// A single packet is independently capped by pkg/sftp at 256 KiB.
type byteBudgetReader struct {
	r         io.Reader
	remaining int64
	exhausted atomic.Bool
}

func (r *byteBudgetReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		r.exhausted.Store(true)
		return 0, ErrRemoteDirectoryLimit
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}

// ReadDirectory opens a short-lived, byte-budgeted SFTP subsystem over the
// alias's existing ControlMaster. Isolating listing traffic from file-transfer
// traffic lets the raw response budget be enforced without interrupting an
// unrelated copy using the primary client.
func (c *Client) ReadDirectory(ctx context.Context, remotePath string) (remoteDirectory, error) {
	if c == nil {
		return remoteDirectory{}, errors.New("nil SFTP client")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, remoteDirectoryTimeout)
	defer cancel()
	if c.dirSem == nil || c.alias == "" {
		// Test/in-process clients do not have spawn metadata. Their fixture is
		// trusted, but still apply entry validation and the retained-entry cap.
		entries, err := c.sftp.ReadDirContext(ctx, remotePath)
		if err != nil {
			return remoteDirectory{}, err
		}
		return sanitizeRemoteDirectory(entries, false), nil
	}
	select {
	case c.dirSem <- struct{}{}:
		defer func() { <-c.dirSem }()
	case <-ctx.Done():
		return remoteDirectory{}, ctx.Err()
	}

	dirClient, budget, err := openContext(ctx, c.alias, c.controlPath, c.hostOpts, maxRemoteDirectoryBytes)
	if err != nil {
		if budget != nil && budget.exhausted.Load() {
			return remoteDirectory{truncated: true}, nil
		}
		return remoteDirectory{}, err
	}
	defer func() { _ = dirClient.Close() }()
	entries, readErr := dirClient.sftp.ReadDirContext(ctx, remotePath)
	limited := budget != nil && budget.exhausted.Load()
	if readErr != nil && !limited {
		return remoteDirectory{}, readErr
	}
	return sanitizeRemoteDirectory(entries, limited), nil
}
