// Transfer queue for the Files window. Uploads + downloads run as
// goroutines updating an atomic byte counter; the UI tick rebuilds a
// fv-go taskprogress widget from a snapshot under lock, so the goroutine
// and the renderer never share mutable widget state.
package sftp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"

	pkgsftp "github.com/pkg/sftp"
)

// partSuffix prefixes the unique temporary file each transfer writes into;
// on success it is renamed over the real destination. Per-transfer names
// keep separate fvmux instances and browser windows from sharing a partial
// file when they happen to target the same destination concurrently.
const partSuffix = ".part-fvmux"

// ErrDestinationBusy reports that this Manager already has a transfer
// writing the same local or remote destination. Callers may treat it as
// harmless repeated input rather than presenting it as a transport failure.
var ErrDestinationBusy = errors.New("transfer destination is already in progress")

var fallbackPartID atomic.Uint64

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
	status atomic.Int32 // StatusQueued/Running/Done/Failed/Cancelled.
	errMsg atomic.Pointer[string]

	cancel     chan struct{}
	cancelOnce sync.Once

	// done is closed by run after status reaches a terminal value. A
	// move's group waiter (finishTree) blocks on it to learn when the
	// copy finished, without polling.
	done chan struct{}

	// removeSource, when non-nil, marks this transfer as a move: run
	// deletes the source after a successful copy. A delete failure fails
	// the whole transfer (the copy already landed, so the data is safe;
	// the source just survives and the error surfaces).
	removeSource func() error

	// destinationKey deduplicates active writers inside one manager;
	// partPath isolates the actual temporary file across managers/processes.
	destinationKey string
	partPath       string
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
	StatusRunning int32 = iota
	StatusQueued
	StatusDone
	StatusFailed
	StatusCancelled
	// StatusActive is retained for callers that only need the historical
	// running state. New code should use IsActiveStatus when queued work
	// counts too.
	StatusActive = StatusRunning
)

func IsActiveStatus(status int32) bool {
	return status == StatusQueued || status == StatusRunning
}

func StatusName(status int32) string {
	switch status {
	case StatusQueued:
		return "queued"
	case StatusRunning:
		return "running"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

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

// Manager owns all in-flight + recent SFTP transfers. Alias is the SSH host
// the owning Files window is bound to and is included in every visible task.
type Manager struct {
	Alias    string
	lifetime context.Context

	// openDedicated, when set, opens a fresh SFTP session (its own ssh
	// subprocess over the alias's ControlMaster) for a single-file
	// transfer, so a hard cancel can close that session and abort an
	// in-flight read/write immediately. nil ⇒ StartDedicated falls back
	// to the shared client (cooperative cancel only).
	openDedicated func() (dedicatedConn, error)

	// sem bounds how many transfers move their payload at once (see
	// SetParallel). A folder copy enqueues one transfer per file, so
	// without this cap a large tree would fan every file onto the single
	// shared SFTP session simultaneously. nil ⇒ unbounded (no SetParallel).
	sem chan struct{}

	mu   sync.Mutex
	list []*Transfer

	// treeClaims keeps an entire folder destination reserved until every
	// transfer spawned by its walk has finished. Per-file deduplication alone
	// is insufficient: a second F5 could otherwise re-enqueue files that the
	// first folder copy had already completed while later files were pending.
	treeClaims map[string]struct{}
	scans      map[string]time.Time
	wg         sync.WaitGroup // tracks live run + abort-watcher goroutines.
}

// dedicatedConn is the slice of *Client that StartDedicated needs: the
// underlying pkg/sftp client to transfer over, plus a Close that tears
// the session down (unblocking any read/write wedged on it). *Client
// satisfies it; tests substitute an in-process fake.
type dedicatedConn interface {
	SFTP() *pkgsftp.Client
	Close() error
}

// NewManager returns an empty transfer manager bound to alias.
func NewManager(alias string) *Manager { return &Manager{Alias: alias, lifetime: context.Background()} }

// SetParallel bounds how many file payloads move concurrently on this
// manager. config.SFTP.Parallel feeds it (default 1); the browser calls
// it once, right after NewManager, before any transfer is enqueued. n < 1
// is treated as 1. Leaving it unset (nil sem) means unbounded, which the
// tests that don't care about queuing rely on.
func (m *Manager) SetParallel(n int) {
	if n < 1 {
		n = 1
	}
	m.sem = make(chan struct{}, n)
}

// EnableDedicatedTransfers wires StartDedicated to open a real per-transfer
// ssh subprocess for the manager's alias, reusing controlPath's master.
// hostOpts carries hosts.toml connection overrides (see Open). The
// browser calls this after constructing the manager; tests inject
// their own opener instead.
func (m *Manager) EnableDedicatedTransfers(controlPath string, hostOpts []string) {
	m.EnableDedicatedTransfersContext(context.Background(), controlPath, hostOpts)
}

func (m *Manager) EnableDedicatedTransfersContext(ctx context.Context, controlPath string, hostOpts []string) {
	m.lifetime = ctx
	m.openDedicated = func() (dedicatedConn, error) {
		c, err := OpenContext(ctx, m.Alias, controlPath, hostOpts)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

func (m *Manager) contextErr() error {
	if m.lifetime == nil {
		return nil
	}
	return m.lifetime.Err()
}

// Wait blocks until every started transfer goroutine has finished. The
// browser's close path calls this (off the UI thread) before closing the
// SFTP client, since pkg/sftp's Client is not safe to use concurrently
// with Close — draining first guarantees no transfer is mid-Read/Write
// when the client tears down.
func (m *Manager) Wait() { m.wg.Wait() }

// Start enqueues an upload or download against c and kicks off the
// goroutine. The returned Transfer is the manager's tracking entry —
// caller may stash it but doesn't need to.
func (m *Manager) Start(c *pkgsftp.Client, dir Direction, localPath, remotePath string) (*Transfer, error) {
	return m.enqueue(c, dir, localPath, remotePath, nil)
}

// StartMove is Start plus a move semantic: after the copy succeeds, run
// invokes removeSource to delete the original. A removeSource error fails
// the transfer (the copy already landed; the source is left in place).
func (m *Manager) StartMove(c *pkgsftp.Client, dir Direction, localPath, remotePath string, removeSource func() error) (*Transfer, error) {
	return m.enqueue(c, dir, localPath, remotePath, removeSource)
}

// enqueue stats the source for its size, registers the transfer, and
// launches its run goroutine. removeSource is nil for a plain copy.
func (m *Manager) enqueue(c *pkgsftp.Client, dir Direction, localPath, remotePath string, removeSource func() error) (*Transfer, error) {
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
	destKey, destPath := transferDestination(dir, localPath, remotePath)
	t := &Transfer{
		Direction:      dir,
		LocalPath:      localPath,
		RemotePath:     remotePath,
		Size:           size,
		StartedAt:      time.Now(),
		cancel:         make(chan struct{}),
		done:           make(chan struct{}),
		removeSource:   removeSource,
		destinationKey: destKey,
		partPath:       uniquePartPath(destPath),
	}
	t.status.Store(StatusQueued)
	m.mu.Lock()
	for _, existing := range m.list {
		if IsActiveStatus(existing.Status()) && existing.destinationKey == destKey {
			m.mu.Unlock()
			return nil, fmt.Errorf("%w: %s", ErrDestinationBusy, destPath)
		}
	}
	m.list = append(m.list, t)
	m.mu.Unlock()

	m.wg.Add(1) // paired with Done in run; before the goroutine starts.
	go m.run(c, t)
	return t, nil
}

// transferDestination returns a namespace-qualified key plus the concrete
// destination path. Uploads and downloads write different filesystems, so
// their keys cannot collide merely because the path strings look alike.
func transferDestination(dir Direction, localPath, remotePath string) (string, string) {
	if dir == Upload {
		clean := path.Clean(remotePath)
		return "remote\x00" + clean, clean
	}
	clean := filepath.Clean(localPath)
	return "local\x00" + clean, clean
}

func uniquePartPath(destination string) string {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err == nil {
		return destination + partSuffix + "." + hex.EncodeToString(nonce[:])
	}
	// crypto/rand failure must not make transfers unusable. PID plus a
	// process-local monotonic counter retains practical uniqueness.
	return fmt.Sprintf("%s%s.%d.%d", destination, partSuffix,
		os.Getpid(), fallbackPartID.Add(1))
}

func (m *Manager) claimTreeDestination(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.treeClaims == nil {
		m.treeClaims = make(map[string]struct{})
	}
	if _, exists := m.treeClaims[key]; exists {
		return false
	}
	m.treeClaims[key] = struct{}{}
	return true
}

func (m *Manager) releaseTreeDestination(key string) {
	m.mu.Lock()
	delete(m.treeClaims, key)
	m.mu.Unlock()
}

// StartDedicated runs a single-file transfer (optionally a move, via
// removeSource) on its own SFTP session so a cancel can hard-abort it:
// closing that session unblocks a read/write wedged on a dead link
// immediately, instead of waiting for the next cooperative chunk check.
// Folder transfers deliberately stay on the shared client (StartTree) to
// avoid one ssh subprocess per file. If no dedicated opener is configured
// (or it fails), the transfer falls back to the shared client and remains
// cooperative-cancel only.
func (m *Manager) StartDedicated(shared *pkgsftp.Client, dir Direction, localPath, remotePath string, removeSource func() error) (*Transfer, error) {
	if m.openDedicated == nil {
		return m.enqueue(shared, dir, localPath, remotePath, removeSource)
	}
	dc, err := m.openDedicated()
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err != nil || dc == nil {
		return m.enqueue(shared, dir, localPath, remotePath, removeSource)
	}
	t, err := m.enqueue(dc.SFTP(), dir, localPath, remotePath, removeSource)
	if err != nil {
		_ = dc.Close()
		return nil, err
	}
	m.wg.Add(1)
	go m.abortWatcher(t, dc)
	return t, nil
}

// abortWatcher closes the dedicated session as soon as the transfer is
// cancelled (to unblock a wedged read/write), and in all cases reaps the
// session once the transfer finishes. Close is idempotent, so the
// cancel-then-finish path closing twice is harmless.
func (m *Manager) abortWatcher(t *Transfer, dc dedicatedConn) {
	defer m.wg.Done()
	select {
	case <-t.cancel:
		_ = dc.Close() // hard-abort: unblock the in-flight op now.
		<-t.done       // then wait for run to unwind.
	case <-t.done:
	}
	_ = dc.Close() // reap the subprocess on normal completion.
}

// CancelLast aborts the most recent still-active transfer, if any.
// Used by the browser's Del key. Safe to call repeatedly: the per-
// Transfer sync.Once dedups concurrent cancels. Returns true if a
// cancel signal was sent.
func (m *Manager) CancelLast() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.list) - 1; i >= 0; i-- {
		if IsActiveStatus(m.list[i].Status()) {
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
		if IsActiveStatus(t.Status()) {
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
		if IsActiveStatus(t.Status()) {
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

// ActiveCount includes queued/running copies and recursive directory scans.
// It is used by close-request confirmation, where either kind of work would
// otherwise be cancelled without warning.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.scans)
	for _, t := range m.list {
		if IsActiveStatus(t.Status()) {
			n++
		}
	}
	return n
}

func (m *Manager) HasScanning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.scans) > 0
}

func (m *Manager) beginScan(label string) {
	m.mu.Lock()
	if m.scans == nil {
		m.scans = make(map[string]time.Time)
	}
	m.scans[label] = time.Now()
	m.mu.Unlock()
}

func (m *Manager) endScan(label string) {
	m.mu.Lock()
	delete(m.scans, label)
	m.mu.Unlock()
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
	m.mu.Lock()
	for label, started := range m.scans {
		tp.Tasks = append(tp.Tasks, &taskprogress.Task{
			Caption: "Scanning [" + m.Alias + "] " + label,
			Min:     0, Max: 1, Value: 0, StartedAt: started,
		})
	}
	m.mu.Unlock()
	for _, t := range snap {
		caption := "[" + m.Alias + "] " + captionFor(t)
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
	route := t.LocalPath + " → " + t.RemotePath
	if t.Direction == Download {
		route = t.RemotePath + " → " + t.LocalPath
	}
	switch t.Status() {
	case StatusFailed:
		return fmt.Sprintf("✗ [failed] %s %s — %s", arrow, route, t.Error())
	case StatusCancelled:
		return fmt.Sprintf("⊘ [cancelled] %s %s", arrow, route)
	case StatusDone:
		return fmt.Sprintf("✓ [done] %s %s", arrow, route)
	case StatusQueued:
		return fmt.Sprintf("… [queued] %s %s", arrow, route)
	}
	return fmt.Sprintf("↻ [running] %s %s", arrow, route)
}

func (m *Manager) run(c *pkgsftp.Client, t *Transfer) {
	defer m.wg.Done()
	if t.done != nil {
		defer close(t.done)
	}
	// Concurrency gate: at most Parallel payloads move at once. Queued
	// transfers block here, before the first byte, rather than flooding
	// one SFTP session. A cancel while queued (browser teardown, Del)
	// short-circuits the wait so we don't hold up the drain waiting for a
	// slot the transfer no longer needs. nil sem ⇒ unbounded.
	if m.sem != nil {
		select {
		case <-t.cancel:
			t.status.Store(StatusCancelled)
			return
		case m.sem <- struct{}{}:
			defer func() { <-m.sem }()
		}
	}
	t.status.Store(StatusRunning)
	err := m.copy(c, t)
	// A move deletes the source only once the copy fully succeeded. A
	// delete failure fails the move (copy already landed → data is safe).
	if err == nil && t.removeSource != nil {
		err = t.removeSource()
	}
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
		tmp := t.partPath
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
		tmp := t.partPath
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

// renameClient is the slice of *sftp.Client remoteReplace needs,
// narrowed so tests can script rename failures.
type renameClient interface {
	PosixRename(oldname, newname string) error
	Rename(oldname, newname string) error
	Remove(path string) error
}

// remoteReplace atomically replaces dest with tmp on the remote side.
// Prefers the posix-rename extension (overwrites in one step). Servers
// without it get rename-first: a plain rename succeeds when dest
// doesn't exist. If it fails — which may mean "dest exists" but can
// equally be a transient or permission error the SFTP status doesn't
// let us distinguish — dest is moved ASIDE, never deleted: only after
// tmp has landed at dest is our backup removed, and if the final
// rename fails the backup is moved back. The original therefore
// survives every failure mode, and the temp copy is never removed
// here either — no interleaving can destroy both files.
//
// The backup name is claimed with plain Rename, whose SFTP semantics
// fail when the target exists (servers with the overwriting rename
// support posix-rename and never reach this path). A name that's taken
// — a preserved original left by a previously failed restore, or an
// unrelated user file — is therefore never clobbered; we step to the
// next candidate instead. Nothing here ever Removes a path we didn't
// create in this call.
func remoteReplace(c renameClient, tmp, dest string) error {
	if err := c.PosixRename(tmp, dest); err == nil {
		return nil
	}
	if err := c.Rename(tmp, dest); err == nil {
		return nil
	}
	backup, err := moveAside(c, dest)
	if err != nil {
		// Couldn't move the original aside — nothing has been touched.
		return fmt.Errorf("replacing %s failed: %w (the transferred data is preserved at %s)", dest, err, tmp)
	}
	if err := c.Rename(tmp, dest); err != nil {
		if rerr := c.Rename(backup, dest); rerr != nil {
			return fmt.Errorf("replacing %s failed: %w (the original was moved to %s; the transferred data is preserved at %s)", dest, err, backup, tmp)
		}
		return fmt.Errorf("replacing %s failed: %w (the original was restored; the transferred data is preserved at %s)", dest, err, tmp)
	}
	_ = c.Remove(backup) // reap the backup THIS call created just above
	return nil
}

// moveAside renames dest to a free ".replaced-fvmux" backup path and
// returns the name that won. Collision-safe by construction: each
// candidate is claimed via Rename (which fails on an existing target),
// so occupied names survive untouched. Returns the last rename error
// when every candidate fails — including the degenerate case where the
// failures aren't collisions at all (dead link, permissions), which a
// bounded probe keeps cheap.
func moveAside(c renameClient, dest string) (string, error) {
	var lastErr error
	for i := 0; i < 8; i++ {
		backup := dest + ".replaced-fvmux"
		if i > 0 {
			backup = fmt.Sprintf("%s.replaced-fvmux.%d", dest, i)
		}
		err := c.Rename(dest, backup)
		if err == nil {
			return backup, nil
		}
		lastErr = err
	}
	return "", lastErr
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
