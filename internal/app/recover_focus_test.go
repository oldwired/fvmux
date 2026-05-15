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
	recoverFocus(ws)
	if ws.Focus != nil {
		t.Fatalf("expected nil Focus, got %v", ws.Focus)
	}
}

func TestRecoverFocus_FocusStillInTree(t *testing.T) {
	a, b := layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	root := layout.Split(views.SplitVertical, a, b)
	ws := &windowState{Root: root, Focus: a}
	recoverFocus(ws)
	if ws.Focus != a {
		t.Fatalf("expected Focus preserved, got %v", ws.Focus)
	}
}

func TestRecoverFocus_StaleFocus(t *testing.T) {
	a, b := layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	root := layout.Split(views.SplitVertical, a, b)
	stale := layout.Leaf(newTestPane()) // not in tree
	ws := &windowState{Root: root, Focus: stale}
	recoverFocus(ws)
	if ws.Focus != a && ws.Focus != b {
		t.Fatalf("expected Focus to recover to a live leaf, got %v", ws.Focus)
	}
}

func TestRecoverFocus_NilFocus(t *testing.T) {
	a, b := layout.Leaf(newTestPane()), layout.Leaf(newTestPane())
	root := layout.Split(views.SplitVertical, a, b)
	ws := &windowState{Root: root, Focus: nil}
	recoverFocus(ws)
	if ws.Focus == nil {
		t.Fatalf("expected Focus to be assigned, got nil")
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
	recoverFocus(ws)
	if ws.Focus != b {
		t.Fatalf("expected Focus to land on b after closing a, got %v", ws.Focus)
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
	recoverFocus(ws)
	if ws.Focus == nil || ws.Focus == a {
		t.Fatalf("expected Focus to recover to b or c, got %v", ws.Focus)
	}
}
