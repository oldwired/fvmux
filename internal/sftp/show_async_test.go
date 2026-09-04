package sftp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func TestShowAsync_ConnectErrorResolvesOnUI(t *testing.T) {
	scheduled := make(chan func(), 1)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)
	origConnect := connectFn
	defer func() { connectFn = origConnect }()
	boom := errors.New("dead host")
	var connectRan int32
	connectFn = func(context.Context, string, string, []string, string) (*connectResult, error) {
		atomic.AddInt32(&connectRan, 1)
		return nil, boom
	}
	var resolvedCount int32
	var gotErr error
	ShowAsync(context.Background(), nil, nil, "h", "", nil, 1, OpenOptions{},
		func(_ *Browser, err error) { atomic.AddInt32(&resolvedCount, 1); gotErr = err })
	deliver := waitDeliver(t, scheduled)
	if atomic.LoadInt32(&resolvedCount) != 0 {
		t.Fatal("callback fired before UI delivery")
	}
	if atomic.LoadInt32(&connectRan) != 1 {
		t.Fatalf("connectFn ran %d times", connectRan)
	}
	deliver()
	if atomic.LoadInt32(&resolvedCount) != 1 || !errors.Is(gotErr, boom) {
		t.Fatalf("onResolved count=%d err=%v", resolvedCount, gotErr)
	}
}

func TestBuildBrowserDoesNotStealFocusAfterAuthentication(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 140, 45))
	a := &fvapp.Application{Program: &fvapp.Program{Desktop: desk}}
	files := views.NewWindow(geom.NewRect(2, 2, 112, 34), "waiting", 2)
	terminal := views.NewWindow(geom.NewRect(8, 5, 88, 29), "[prod] Terminal", 1)
	desk.InsertWindow(files)
	desk.InsertWindow(terminal)
	desk.Focus(terminal)
	client := newTestClient(t)
	root := t.TempDir()
	res := &connectResult{client: &Client{sftp: client}, remoteCwd: root, tree: buildRemoteTree(client, root), listing: buildRemoteListing(client, root)}
	browser := buildBrowser(a, files, "prod", "", nil, 1, OpenOptions{RemoteCWD: root}, res)
	if got := desk.Current(); got != terminal.Self() {
		t.Fatalf("ready Files window stole focus from authentication terminal: %T", got)
	}
	done := make(chan struct{})
	browser.Close(func() { close(done) })
	<-done
}

func TestShowAsync_SuccessBuildsRequestedWorkspaceWindow(t *testing.T) {
	scheduled := make(chan func(), 1)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)
	origConnect, origBuild := connectFn, buildBrowserFn
	defer func() { connectFn, buildBrowserFn = origConnect, origBuild }()
	root := t.TempDir()
	client := newTestClient(t)
	res := &connectResult{client: &Client{sftp: client}, remoteCwd: root}
	var gotRequested string
	connectFn = func(_ context.Context, _, _ string, _ []string, requested string) (*connectResult, error) {
		gotRequested = requested
		return res, nil
	}
	wantBrowser := &Browser{Alias: "h"}
	var builderRan int32
	buildBrowserFn = func(_ *fvapp.Application, frame *views.Window, alias, _ string, _ []string, parallel int, opts OpenOptions, r *connectResult) *Browser {
		atomic.AddInt32(&builderRan, 1)
		if frame != nil || alias != "h" || parallel != 2 || r != res || opts.RemoteCWD != root {
			t.Error("builder did not receive requested workspace context")
		}
		return wantBrowser
	}
	var got *Browser
	var gotErr error
	ShowAsync(context.Background(), nil, nil, "h", "", nil, 2, OpenOptions{RemoteCWD: root},
		func(browser *Browser, err error) { got, gotErr = browser, err })
	waitDeliver(t, scheduled)()
	if builderRan != 1 || got != wantBrowser || gotErr != nil || gotRequested != root {
		t.Fatalf("builder=%d browser=%p err=%v cwd=%q", builderRan, got, gotErr, gotRequested)
	}
}

func TestShowAsync_CancelledBeforeUIDeliveryDoesNotBuild(t *testing.T) {
	scheduled := make(chan func(), 1)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)
	origConnect, origBuild := connectFn, buildBrowserFn
	defer func() { connectFn, buildBrowserFn = origConnect, origBuild }()
	client := newTestClient(t)
	connectFn = func(context.Context, string, string, []string, string) (*connectResult, error) {
		return &connectResult{client: &Client{sftp: client}, remoteCwd: "/"}, nil
	}
	var built bool
	buildBrowserFn = func(*fvapp.Application, *views.Window, string, string, []string, int, OpenOptions, *connectResult) *Browser {
		built = true
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	var gotErr error
	ShowAsync(ctx, nil, nil, "h", "", nil, 1, OpenOptions{}, func(_ *Browser, err error) { gotErr = err })
	deliver := waitDeliver(t, scheduled)
	cancel()
	deliver()
	if built {
		t.Fatal("cancelled generation built a browser")
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", gotErr)
	}
}
