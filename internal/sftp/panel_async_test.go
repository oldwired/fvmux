package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Regression tests for finding #13: every remote SFTP round-trip in the
// browser now runs off the UI event goroutine via panel.asyncRemoteOp,
// tracked by the browser's teardown drain (refreshWG) and gated by the
// shared closed flag.

// goID returns the calling goroutine's numeric id, parsed from the
// runtime stack header ("goroutine N [running]:"). Used to prove op runs
// on a goroutine other than the caller's.
func goID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	id, _ := strconv.ParseUint(s, 10, 64)
	return id
}

// TestAsyncRemoteOp_RunsOffUIAndDeliversError proves op executes on a
// different goroutine and its error reaches done on the "UI" path. A
// queuing CallSoon scheduler stands in for the program loop: the test
// delivers the callback itself, mirroring how the real loop marshals it
// back onto the UI goroutine.
func TestAsyncRemoteOp_RunsOffUIAndDeliversError(t *testing.T) {
	scheduled := make(chan func(), 1)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)

	var closed atomic.Bool
	var wg sync.WaitGroup
	p := &panel{isRemote: true, closed: &closed, refreshWG: &wg}

	callerID := goID()
	var opID uint64
	boom := errors.New("boom")

	var doneErr error
	doneRan := false
	p.asyncRemoteOp(
		func() error {
			opID = goID()
			return boom
		},
		func(err error) {
			doneErr = err
			doneRan = true
		},
	)

	// The done callback arrives via CallSoon; grab it and run it here.
	var deliver func()
	select {
	case deliver = <-scheduled:
	case <-time.After(2 * time.Second):
		t.Fatal("asyncRemoteOp never scheduled its done callback")
	}
	// The channel receive synchronises with the worker goroutine, so opID
	// is now safe to read without a race.
	if opID == 0 {
		t.Fatal("op did not run")
	}
	if opID == callerID {
		t.Errorf("op ran on the caller goroutine (%d) — must be off the UI thread", callerID)
	}
	if doneRan {
		t.Fatal("done ran before the UI delivered the CallSoon callback")
	}
	deliver()
	if !doneRan {
		t.Fatal("done was not invoked after delivering the callback")
	}
	if !errors.Is(doneErr, boom) {
		t.Errorf("done received err = %v, want boom", doneErr)
	}
}

// TestAsyncRemoteOp_SkippedWhenClosed: once the browser is tearing down
// (closed = true) asyncRemoteOp must not run op or done, and must not
// touch refreshWG (no worker is started).
func TestAsyncRemoteOp_SkippedWhenClosed(t *testing.T) {
	views.SetCallSoon(nil)
	var closed atomic.Bool
	closed.Store(true)
	var wg sync.WaitGroup
	p := &panel{isRemote: true, closed: &closed, refreshWG: &wg}

	ran := make(chan struct{}, 2)
	p.asyncRemoteOp(
		func() error { ran <- struct{}{}; return nil },
		func(error) { ran <- struct{}{} },
	)
	select {
	case <-ran:
		t.Fatal("asyncRemoteOp ran op/done despite the browser being closed")
	case <-time.After(150 * time.Millisecond):
		// good — neither op nor done fired.
	}
	// A stray refreshWG.Add would leave this hanging; it returns at once.
	wg.Wait()
}

// TestAsyncRemoteOp_RefreshWGBalanced: the WaitGroup the browser hands
// the panel is Add/Done balanced, so teardown's refreshWG.Wait unblocks
// once the op has drained. Under the inline CallSoon fallback the worker
// runs done before its deferred Done, so Wait returning implies done
// already fired.
func TestAsyncRemoteOp_RefreshWGBalanced(t *testing.T) {
	views.SetCallSoon(nil)
	defer views.SetCallSoon(nil)
	var closed atomic.Bool
	var wg sync.WaitGroup
	p := &panel{isRemote: true, closed: &closed, refreshWG: &wg}

	doneFired := make(chan struct{})
	p.asyncRemoteOp(
		func() error { return nil },
		func(error) { close(doneFired) },
	)
	wg.Wait()
	select {
	case <-doneFired:
		// good — done fired before the WaitGroup drained.
	default:
		t.Fatal("refreshWG.Wait returned before done fired")
	}
}

// TestAsyncRemoteOp_EndToEndMkdirDelete drives a real mkdir + recursive
// delete through the async helper against the in-process SFTP harness —
// the same client calls the F7/F8 actions make. The inline CallSoon
// fallback keeps each round-trip synchronous to refreshWG.Wait. (The
// action funcs themselves, mkdirAction/deleteAction, prompt via modal
// dialogs, so they are exercised here through asyncRemoteOp directly
// rather than through their dialog scaffolding.)
func TestAsyncRemoteOp_EndToEndMkdirDelete(t *testing.T) {
	views.SetCallSoon(nil)
	defer views.SetCallSoon(nil)
	c := newTestClient(t)
	root := t.TempDir()

	var closed atomic.Bool
	var wg sync.WaitGroup
	p := &panel{isRemote: true, c: c, cwd: root, closed: &closed, refreshWG: &wg}

	dir := filepath.Join(root, "made-via-async")

	// mkdir through the async helper.
	mkErr := make(chan error, 1)
	p.asyncRemoteOp(
		func() error { return p.c.MkdirAll(dir) },
		func(err error) { mkErr <- err },
	)
	wg.Wait()
	if err := <-mkErr; err != nil {
		t.Fatalf("async mkdir: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("directory not created: stat err=%v", err)
	}

	// recursive delete through the async helper.
	rmErr := make(chan error, 1)
	p.asyncRemoteOp(
		func() error { return p.c.RemoveAll(dir) },
		func(err error) { rmErr <- err },
	)
	wg.Wait()
	if err := <-rmErr; err != nil {
		t.Fatalf("async delete: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory not removed: stat err=%v", err)
	}
}
