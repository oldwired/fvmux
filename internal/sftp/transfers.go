// Transfer queue for the SFTP browser. Uploads + downloads run as
// goroutines updating an atomic byte counter; the UI tick rebuilds a
// fv-go taskprogress widget from a snapshot under lock, so the goroutine
// and the renderer never share mutable widget state.
package sftp

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"

	pkgsftp "github.com/pkg/sftp"
)

// partSuffix names the temporary file a transfer writes into; on success
// it is renamed over the real destination, so a failed/cancelled transfer
// never leaves a truncated file masquerading as a complete one, and an
// existing destination is only replaced once the copy fully succeeds.
const partSuffix = ".part-fvmux"

// Direction is the transfer direction.
type Direction uint8

const (
	Upload   Direction = iota // local → remote.
	Download                  // remote → local.
)

// Transfer is one queued / running / finished operation. Only Bytes
// and Status are written after construction; readers must use the
// accessor methods, which take the manager's lock.
type Transfer struct {
	Direction  Direction
	LocalPath  string
	RemotePath string
	Size       int64
	StartedAt  time.Time

	bytes  atomic.Int64 // copied so far.
	status atomic.Int32 // StatusActive/Done/Failed/Cancelled.
	errMsg atomic.Pointer[string]

	cancel     chan struct{}
	cancelOnce sync.Once
}

// requestCancel closes t.cancel exactly once across all callers. Safe
// to invoke repeatedly (rapid Del presses, lifetime-tied context
// cancellation, etc.).
func (t *Transfer) requestCancel() {
	t.cancelOnce.Do(func() { close(t.cancel) })
}

// cancelled reports whether cancellation has been requested. Used to
// classify a transfer that errored out specifically because the client
// was closed under it (browser teardown) as Cancelled rather than Failed.
func (t *Transfer) cancelled() bool {
	select {
	case <-t.cancel:
		return true
	default:
		return false
	}
}

// Status values, stored as int32 in Transfer.status.
const (
	StatusActive int32 = iota
	StatusDone
	StatusFailed
	StatusCancelled
)

// Bytes copied so far.
func (t *Transfer) Bytes() int64 { return t.bytes.Load() }

// Status returns the live status enum.
func (t *Transfer) Status() int32 { return t.status.Load() }

// Error returns the last reported error, or "".
func (t *Transfer) Error() string {
	if p := t.errMsg.Load(); p != nil {
		return *p
	}
	return ""
}

// Manager owns all in-flight + recent SFTP transfers. Alias is the
// SSH host this browser is bound to — read by session-snapshot save
// so reopening the session restores the right browsers. closeFn,
// when set, closes the browser dialog that owns this manager (used
// by CloseAllBrowsers).
type Manager struct {
	Alias   string
	closeFn func()

	mu   sync.Mutex
	list []*Transfer
	wg   sync.WaitGroup // tracks live run goroutines.
}

// NewManager returns an empty transfer manager bound to alias.
func NewManager(alias string) *Manager { return &Manager{Alias: alias} }

// Wait blocks until every started transfer goroutine has finished. The
// browser's close path calls this (off the UI thread) before closing the
// SFTP client, since pkg/sftp's Client is not safe to use concurrently
// with Close — draining first guarantees no transfer is mid-Read/Write
// when the client tears down.
func (m *Manager) Wait() { m.wg.Wait() }

// SetCloseFn lets the browser register its close action against the
// manager so CloseAllBrowsers can dismiss it.
func (m *Manager) SetCloseFn(fn func()) { m.closeFn = fn }

// Start enqueues an upload or download against c and kicks off the
// goroutine. The returned Transfer is the manager's tracking entry —
// caller may stash it but doesn't need to.
func (m *Manager) Start(c *pkgsftp.Client, dir Direction, localPath, remotePath string) (*Transfer, error) {
	var size int64
	switch dir {
	case Upload:
		fi, err := os.Stat(localPath)
		if err != nil {
			return nil, err
		}
		size = fi.Size()
	case Download:
		fi, err := c.Stat(remotePath)
		if err != nil {
			return nil, err
		}
		size = fi.Size()
	}
	t := &Transfer{
		Direction:  dir,
		LocalPath:  localPath,
		RemotePath: remotePath,
		Size:       size,
		StartedAt:  time.Now(),
		cancel:     make(chan struct{}),
	}
	m.mu.Lock()
	m.list = append(m.list, t)
	m.mu.Unlock()

	m.wg.Add(1) // paired with Done in run; before the goroutine starts.
	go m.run(c, t)
	return t, nil
}

// CancelLast aborts the most recent still-active transfer, if any.
// Used by the browser's Del key. Safe to call repeatedly: the per-
// Transfer sync.Once dedups concurrent cancels. Returns true if a
// cancel signal was sent.
func (m *Manager) CancelLast() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.list) - 1; i >= 0; i-- {
		if m.list[i].Status() == StatusActive {
			m.list[i].requestCancel()
			return true
		}
	}
	return false
}

// CancelAll requests cancellation of every active transfer. Used when
// the browser closes mid-flight; idempotent.
func (m *Manager) CancelAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.list {
		if t.Status() == StatusActive {
			t.requestCancel()
		}
	}
}

// ClearCompleted drops Done/Failed/Cancelled entries. Active transfers
// are preserved.
func (m *Manager) ClearCompleted() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.list[:0]
	dropped := 0
	for _, t := range m.list {
		if t.Status() == StatusActive {
			kept = append(kept, t)
		} else {
			dropped++
		}
	}
	m.list = kept
	return dropped
}

// Snapshot returns a list of all transfers in declaration order. Safe
// for the UI loop to iterate.
func (m *Manager) Snapshot() []*Transfer {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Transfer, len(m.list))
	copy(out, m.list)
	return out
}

// SyncWidget rebuilds tp.Tasks from the current snapshot. Callers run
// this on the UI tick so the goroutine and the renderer never touch
// the same Task struct concurrently.
func (m *Manager) SyncWidget(tp *taskprogress.TaskProgress) {
	if tp == nil {
		return
	}
	snap := m.Snapshot()
	tp.Tasks = tp.Tasks[:0]
	for _, t := range snap {
		caption := captionFor(t)
		// taskprogress fields are int; halve both Max and Value until Max
		// fits int32 so a >2 GiB transfer can't overflow to a negative bar
		// on 32-bit builds. Display-only — the copy loop uses int64.
		maxV, valV := t.Size, t.Bytes()
		for maxV > math.MaxInt32 {
			maxV >>= 1
			valV >>= 1
		}
		if maxV < 1 {
			maxV = 1 // taskprogress needs Max > Min.
		}
		task := &taskprogress.Task{
			Caption:   caption,
			Min:       0,
			Max:       int(maxV),
			Value:     int(valV),
			StartedAt: t.StartedAt,
		}
		switch t.Status() {
		case StatusDone:
			task.Done = true
		case StatusFailed, StatusCancelled:
			task.Failed = true
		}
		tp.Tasks = append(tp.Tasks, task)
	}
}

func captionFor(t *Transfer) string {
	arrow := "⇑"
	if t.Direction == Download {
		arrow = "⇓"
	}
	short := filepath.Base(t.LocalPath)
	if t.Direction == Download {
		short = filepath.Base(t.RemotePath)
	}
	switch t.Status() {
	case StatusFailed:
		return fmt.Sprintf("✗ %s %s — %s", arrow, short, t.Error())
	case StatusCancelled:
		return fmt.Sprintf("⊘ %s %s — cancelled", arrow, short)
	case StatusDone:
		return fmt.Sprintf("✓ %s %s", arrow, short)
	}
	return fmt.Sprintf("%s %s", arrow, short)
}

func (m *Manager) run(c *pkgsftp.Client, t *Transfer) {
	defer m.wg.Done()
	err := m.copy(c, t)
	switch {
	case err == nil:
		t.bytes.Store(t.Size)
		t.status.Store(StatusDone)
	case err == errCancelled || t.cancelled():
		// errCancelled is the cooperative path; t.cancelled() catches a
		// transfer killed by the client closing under it on teardown
		// (which surfaces as a read/write error). Either way: Cancelled,
		// not Failed.
		t.status.Store(StatusCancelled)
	default:
		s := err.Error()
		t.errMsg.Store(&s)
		t.status.Store(StatusFailed)
	}
}

var errCancelled = fmt.Errorf("cancelled")
var errWriteStall = fmt.Errorf("destination write stalled (n=0 with no error)")

// copy streams the source into a temporary destination, then renames it
// over the real destination on success. On any failure (including cancel)
// the temporary is removed, so a partial transfer never leaves a
// truncated file behind and an existing destination survives untouched.
func (m *Manager) copy(c *pkgsftp.Client, t *Transfer) error {
	switch t.Direction {
	case Upload:
		lf, err := os.Open(t.LocalPath)
		if err != nil {
			return err
		}
		defer func() { _ = lf.Close() }()
		tmp := t.RemotePath + partSuffix
		rf, err := c.Create(tmp)
		if err != nil {
			return err
		}
		perr := pump(lf, rf, t)
		// dst.Close flushes buffered SFTP writes; a silent error here
		// would mean reporting success after losing data.
		if cerr := rf.Close(); perr == nil {
			perr = cerr
		}
		if perr != nil {
			_ = c.Remove(tmp)
			return perr
		}
		return remoteReplace(c, tmp, t.RemotePath)
	case Download:
		rf, err := c.Open(t.RemotePath)
		if err != nil {
			return err
		}
		defer func() { _ = rf.Close() }()
		tmp := t.LocalPath + partSuffix
		lf, err := os.Create(tmp)
		if err != nil {
			return err
		}
		perr := pump(rf, lf, t)
		if cerr := lf.Close(); perr == nil {
			perr = cerr
		}
		if perr != nil {
			_ = os.Remove(tmp)
			return perr
		}
		if err := os.Rename(tmp, t.LocalPath); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return nil
	}
	return nil
}

// remoteReplace atomically replaces dest with tmp on the remote side.
// Prefers the posix-rename extension (overwrites in one step); falls back
// to remove-then-rename for servers that lack it.
func remoteReplace(c *pkgsftp.Client, tmp, dest string) error {
	if err := c.PosixRename(tmp, dest); err == nil {
		return nil
	}
	_ = c.Remove(dest)
	if err := c.Rename(tmp, dest); err != nil {
		_ = c.Remove(tmp)
		return err
	}
	return nil
}

// pump is the cancel-aware copy loop. Cancellation is cooperative: the
// channel is checked at each 64 KiB chunk boundary, so a transfer stalled
// inside a single Read/Write only aborts once that syscall returns (or
// the client is closed under it). Updates t.bytes as it goes.
func pump(src io.Reader, dst io.Writer, t *Transfer) error {
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-t.cancel:
			return errCancelled
		default:
		}
		n, err := src.Read(buf)
		if n > 0 {
			wn, werr := dst.Write(buf[:n])
			if werr != nil {
				return werr
			}
			if wn == 0 {
				// Writer accepted no bytes and reported no error — a
				// degenerate state we can't progress past. Fail loudly
				// rather than loop forever.
				return errWriteStall
			}
			t.bytes.Add(int64(wn))
			if wn < n {
				// io.Writer contract permits short writes only with
				// an error; treat this as a defensive guard.
				return io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
