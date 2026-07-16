package sftp

import (
	"errors"
	"sync/atomic"
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// Residual #2 regression tests: opening a browser must not run the ssh
// connect / SFTP negotiate / initial reads on the UI event goroutine.
// ShowAsync runs the network phase (connectFn) on a worker and marshals
// the outcome back via views.CallSoon, where the caller's callbacks fire.

// TestShowAsync_ConnectErrorReleasesThenResolves: on a connect failure
// onClose fires exactly once (immediately, releasing the pool ref) and
// then onResolved carries the error — both on the delivered UI callback,
// not inline on the worker.
func TestShowAsync_ConnectErrorReleasesThenResolves(t *testing.T) {
	scheduled := make(chan func(), 1)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)

	origConnect := connectFn
	defer func() { connectFn = origConnect }()
	boom := errors.New("dead host")
	var connectRan int32
	connectFn = func(alias, controlPath string, hostOpts []string) (*connectResult, error) {
		atomic.AddInt32(&connectRan, 1)
		return nil, boom
	}

	var closeCount, resolvedCount int32
	var gotErr error
	// a is nil: the failure path never touches the Application.
	ShowAsync(nil, "h", "", nil, 1,
		func() { atomic.AddInt32(&closeCount, 1) },
		func(err error) { atomic.AddInt32(&resolvedCount, 1); gotErr = err },
	)

	deliver := waitDeliver(t, scheduled)
	// Before the UI delivers, neither callback has fired — proving the
	// resolution is marshaled back, not run on the worker.
	if atomic.LoadInt32(&closeCount) != 0 || atomic.LoadInt32(&resolvedCount) != 0 {
		t.Fatal("callbacks fired on the worker instead of via CallSoon")
	}
	if atomic.LoadInt32(&connectRan) != 1 {
		t.Fatalf("connectFn ran %d times, want 1", connectRan)
	}
	deliver()

	if atomic.LoadInt32(&closeCount) != 1 {
		t.Errorf("onClose fired %d times, want exactly 1", closeCount)
	}
	if atomic.LoadInt32(&resolvedCount) != 1 {
		t.Errorf("onResolved fired %d times, want 1", resolvedCount)
	}
	if !errors.Is(gotErr, boom) {
		t.Errorf("onResolved err = %v, want %v", gotErr, boom)
	}
}

// TestShowAsync_SuccessBuildsAndDefersRelease: on a successful connect the
// UI-phase builder runs with the connect result, onResolved fires with a
// nil error, and onClose is NOT called yet — the open browser owns it
// until it closes. The heavy dialog build is stubbed via buildBrowserFn.
func TestShowAsync_SuccessBuildsAndDefersRelease(t *testing.T) {
	scheduled := make(chan func(), 1)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)

	origConnect := connectFn
	defer func() { connectFn = origConnect }()
	origBuild := buildBrowserFn
	defer func() { buildBrowserFn = origBuild }()

	root := t.TempDir()
	client := newTestClient(t)
	res := &connectResult{
		client:    &Client{sftp: client},
		remoteCwd: root,
		tree:      buildRemoteTree(client, root),
		listing:   buildRemoteListing(client, root),
	}
	connectFn = func(alias, controlPath string, hostOpts []string) (*connectResult, error) {
		return res, nil
	}

	var builderRan int32
	var builderGotClose func()
	buildBrowserFn = func(a *fvapp.Application, alias, controlPath string, hostOpts []string, parallel int, r *connectResult, onClose func()) {
		atomic.AddInt32(&builderRan, 1)
		if r != res {
			t.Error("builder received a different connectResult")
		}
		builderGotClose = onClose // real buildBrowser threads this into d.OnClose.
	}

	var closeCount, resolvedCount int32
	resolvedErr := errors.New("unset")
	ShowAsync(nil, "h", "", nil, 1,
		func() { atomic.AddInt32(&closeCount, 1) },
		func(err error) { atomic.AddInt32(&resolvedCount, 1); resolvedErr = err },
	)

	deliver := waitDeliver(t, scheduled)
	deliver()

	if atomic.LoadInt32(&builderRan) != 1 {
		t.Fatalf("builder ran %d times, want 1", builderRan)
	}
	if atomic.LoadInt32(&resolvedCount) != 1 || resolvedErr != nil {
		t.Fatalf("onResolved: count=%d err=%v, want 1/nil", resolvedCount, resolvedErr)
	}
	if atomic.LoadInt32(&closeCount) != 0 {
		t.Fatalf("onClose fired on open; it must wait for the browser to close")
	}
	if builderGotClose == nil {
		t.Fatal("builder was not handed the onClose the browser owns")
	}
	// The onClose the browser holds is the exactly-once release; firing it
	// (as d.OnClose would) balances the ref.
	builderGotClose()
	if atomic.LoadInt32(&closeCount) != 1 {
		t.Fatalf("onClose after browser close fired %d times, want 1", closeCount)
	}
}
