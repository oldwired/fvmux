package app

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/layout"
)

func TestComposeTitle(t *testing.T) {
	cases := []struct {
		user, shell, fallback, pane, want string
	}{
		{"", "", "shell", "", "shell"},
		{"", "vim x", "shell", "", "vim x"},
		{"logs", "", "shell", "", "[logs]"},
		{"logs", "tail -f", "shell", "", "[logs] tail -f"},
		{"", "", "", "", ""},

		// Pane title equals shell title → suppressed (already shown).
		{"", "vim x", "shell", "vim x", "vim x"},
		// Pane title distinct → appended after `·`.
		{"", "vim x", "shell", "build", "vim x · build"},
		// Pane title appears in user-bracketed base — also suppressed.
		{"edit", "", "shell", "edit", "[edit]"},
		// Pane title combined with user + shell.
		{"edit", "vim x", "shell", "build", "[edit] vim x · build"},
		// Pane title with no shell, no user → falls into fallback.
		{"", "", "shell", "build", "shell · build"},
	}
	for _, c := range cases {
		got := composeTitle(c.user, c.shell, c.fallback, c.pane)
		if got != c.want {
			t.Errorf("composeTitle(%q,%q,%q,%q) = %q; want %q",
				c.user, c.shell, c.fallback, c.pane, got, c.want)
		}
	}
}

func TestPaneFocusRefreshesShellAndWindowTitle(t *testing.T) {
	m, ws, paneA, paneB := newFocusMux(t)
	ws.Title = "shell"
	paneA.Pane.Title = "shell-a"
	paneA.Pane.ShellTitle = "vim"
	paneB.Pane.Title = "shell-b"

	m.setPaneFocus(ws, paneA)
	if ws.ShellTitle != "vim" || ws.Frame.Title() != "vim" {
		t.Fatalf("pane A focus: ShellTitle=%q frame=%q, want vim", ws.ShellTitle, ws.Frame.Title())
	}

	// An OSC title from an unfocused pane belongs to that pane only.
	m.wireTerminalCallbacks(paneB.Pane)
	paneB.Pane.Term.OnTitle("htop")
	if ws.ShellTitle != "vim" {
		t.Fatalf("unfocused OSC replaced window ShellTitle with %q", ws.ShellTitle)
	}

	m.setPaneFocus(ws, paneB)
	if ws.ShellTitle != "htop" || ws.Frame.Title() != "htop" {
		t.Fatalf("pane B focus: ShellTitle=%q frame=%q, want htop without stale vim", ws.ShellTitle, ws.Frame.Title())
	}

	// Removal transitions through the same focus helper and immediately
	// restores the surviving pane's existing OSC title.
	m.doClose()
	if ws.Focus != paneA || ws.ShellTitle != "vim" || ws.Frame.Title() != "vim" {
		t.Fatalf("after closing B: focus=%v ShellTitle=%q frame=%q; want A/vim", ws.Focus, ws.ShellTitle, ws.Frame.Title())
	}
}

func TestTerminalCallbacksFollowPaneAcrossWindowsWithoutRewiring(t *testing.T) {
	m, source, survivor, moved := newFocusMux(t)
	m.wireTerminalCallbacks(moved.Pane)

	remaining, detached := layout.BreakOut(source.Root, moved)
	source.Root = remaining
	m.setPaneFocus(source, survivor)
	m.rerender(source)

	w := views.NewWindow(geom.NewRect(2, 2, 42, 14), "detached", 2)
	destination := m.finishWindow(w, 2, "detached", detached, windowInterior(w))
	moved.Pane.Term.OnTitle("ssh prod")

	if destination.ShellTitle != "ssh prod" || destination.Frame.Title() != "ssh prod" {
		t.Fatalf("moved pane title reached destination as ShellTitle=%q frame=%q",
			destination.ShellTitle, destination.Frame.Title())
	}
	if source.ShellTitle == "ssh prod" {
		t.Fatal("moved pane callback still targeted its source window")
	}
}
