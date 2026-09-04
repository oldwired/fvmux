package app

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/session"
)

func newTestPane() *session.Pane {
	return &session.Pane{ID: session.NewPaneID()}
}

func TestRecoverFocus_NilRoot(t *testing.T) {
	ws := &windowState{}
	if got := recoverFocus(ws); got != nil {
		t.Fatalf("expected nil focus candidate, got %v", got)
	}
}

func TestRecoverFocus_FocusStillInTree(t *testing.T) {
	a, b := layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	root := layout.Split(views.SplitVertical, a, b)
	ws := &windowState{Root: root, Focus: a}
	if got := recoverFocus(ws); got != a {
		t.Fatalf("expected Focus preserved, got %v", got)
	}
}

func TestRecoverFocus_StaleFocus(t *testing.T) {
	a, b := layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	root := layout.Split(views.SplitVertical, a, b)
	stale := layout.Leaf(newTestPane()) // not in tree
	ws := &windowState{Root: root, Focus: stale}
	got := recoverFocus(ws)
	if got != a && got != b {
		t.Fatalf("expected a live focus candidate, got %v", got)
	}
}

func TestRecoverFocus_NilFocus(t *testing.T) {
	a, b := layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	root := layout.Split(views.SplitVertical, a, b)
	ws := &windowState{Root: root, Focus: nil}
	if got := recoverFocus(ws); got == nil {
		t.Fatalf("expected a focus candidate, got nil")
	}
}

func TestRecoverFocus_AfterClose(t *testing.T) {
	a, b := layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	root := layout.Split(views.SplitVertical, a, b)
	ws := &windowState{Root: root, Focus: a}
	// Simulate doClose: Close removes a, ws.Root becomes b.
	newRoot, removed := layout.Close(root, a)
	if removed {
		t.Fatal("Close should not remove the last window for a two-leaf tree")
	}
	ws.Root = newRoot
	if got := recoverFocus(ws); got != b {
		t.Fatalf("expected focus candidate b after closing a, got %v", got)
	}
}

func TestRecoverFocus_AfterBreakOut(t *testing.T) {
	a, b, c := layout.Leaf(newTestPane()), layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	inner := layout.Split(views.SplitVertical, b, c)
	root := layout.Split(views.SplitHorizontal, a, inner)
	ws := &windowState{Root: root, Focus: a}
	// Simulate doBreakOut on a: ws.Root becomes inner (b,c); a is broken out.
	newSrc, _ := layout.BreakOut(root, a)
	ws.Root = newSrc
	got := recoverFocus(ws)
	if got == nil || got == a {
		t.Fatalf("expected focus candidate b or c, got %v", got)
	}
}
