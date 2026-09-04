package sftp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	pkgsftp "github.com/pkg/sftp"
)

func TestRemotePreviewDecodeConcurrencyIsBounded(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	p := &previewPane{
		previewSlots: make(chan struct{}, 1),
		render: func(*pkgsftp.Client, string, geom.Rect) views.View {
			n := active.Add(1)
			for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
			}
			entered <- struct{}{}
			<-release
			active.Add(-1)
			return nil
		},
	}
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_ = p.buildRemote(context.Background(), "/image.png", geom.NewRect(0, 0, 10, 10))
			done <- struct{}{}
		}()
	}
	<-entered
	select {
	case <-entered:
		t.Fatal("a second remote preview decoded concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	<-entered
	release <- struct{}{}
	<-done
	<-done
	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent preview decodes = %d, want 1", got)
	}
}

func TestRemotePreviewWaitingForSlotIsCancellable(t *testing.T) {
	called := atomic.Bool{}
	p := &previewPane{
		previewSlots: make(chan struct{}, 1),
		render: func(*pkgsftp.Client, string, geom.Rect) views.View {
			called.Store(true)
			return nil
		},
	}
	p.previewSlots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan views.View, 1)
	go func() { done <- p.buildRemote(ctx, "/image.png", geom.Rect{}) }()
	cancel()
	select {
	case got := <-done:
		if got != nil || called.Load() {
			t.Fatalf("cancelled preview = %T, render called=%v; want nil, false", got, called.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("preview waiting for its decode slot ignored cancellation")
	}
	<-p.previewSlots
}
