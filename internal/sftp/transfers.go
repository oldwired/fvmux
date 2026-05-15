// Transfer queue for the SFTP browser. Uploads + downloads run as
// goroutines updating an atomic byte counter; the UI tick rebuilds a
// fv-go taskprogress widget from a snapshot under lock, so the goroutine
// and the renderer never share mutable widget state.
package sftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"

	pkgsftp "github.com/pkg/sftp"
)

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

// Manager owns all in-flight + recent SFTP transfers.
type Manager struct {
	mu   sync.Mutex
	list []*Transfer
}

// NewManager returns an empty transfer manager.
func NewManager() *Manager { return &Manager{} }

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
		size := t.Size
		if size < 1 {
			size = 1 // taskprogress needs Max > Min.
		}
		task := &taskprogress.Task{
			Caption:   caption,
			Min:       0,
			Max:       int(size),
			Value:     int(t.Bytes()),
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
	err := m.copy(c, t)
	switch {
	case err == nil:
		t.bytes.Store(t.Size)
		t.status.Store(StatusDone)
	case err == errCancelled:
		t.status.Store(StatusCancelled)
	default:
		s := err.Error()
		t.errMsg.Store(&s)
		t.status.Store(StatusFailed)
	}
}

var errCancelled = fmt.Errorf("cancelled")
var errWriteStall = fmt.Errorf("destination write stalled (n=0 with no error)")

func (m *Manager) copy(c *pkgsftp.Client, t *Transfer) error {
	var src io.ReadCloser
	var dst io.WriteCloser

	switch t.Direction {
	case Upload:
		lf, err := os.Open(t.LocalPath)
		if err != nil {
			return err
		}
		rf, err := c.Create(t.RemotePath)
		if err != nil {
			lf.Close()
			return err
		}
		src, dst = lf, rf
	case Download:
		rf, err := c.Open(t.RemotePath)
		if err != nil {
			return err
		}
		lf, err := os.Create(t.LocalPath)
		if err != nil {
			rf.Close()
			return err
		}
		src, dst = rf, lf
	}
	defer src.Close()
	defer dst.Close()

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
