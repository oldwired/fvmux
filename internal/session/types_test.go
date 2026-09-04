package session

import "testing"

func TestPaneDisplayTitlePrecedence(t *testing.T) {
	p := &Pane{Title: "profile", ShellTitle: "vim", UserTitle: "editor"}
	if got := p.DisplayTitle(); got != "editor" {
		t.Fatalf("DisplayTitle with user title = %q, want editor", got)
	}
	p.UserTitle = ""
	if got := p.DisplayTitle(); got != "vim" {
		t.Fatalf("DisplayTitle with shell title = %q, want vim", got)
	}
	p.ShellTitle = ""
	if got := p.DisplayTitle(); got != "profile" {
		t.Fatalf("DisplayTitle fallback = %q, want profile", got)
	}
	var nilPane *Pane
	if got := nilPane.DisplayTitle(); got != "" {
		t.Fatalf("nil Pane DisplayTitle = %q, want empty", got)
	}
}
