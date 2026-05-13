package layout

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
)

func TestMarshalLeaf(t *testing.T) {
	p := &session.Pane{ID: session.NewPaneID(), Profile: "shell", Title: "zsh"}
	got := Marshal(Leaf(p))
	want := `leaf:profile=shell,title="zsh"`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestMarshalNestedSplit(t *testing.T) {
	a := &session.Pane{ID: session.NewPaneID(), Profile: "shell"}
	b := &session.Pane{ID: session.NewPaneID(), Profile: "shell"}
	c := &session.Pane{ID: session.NewPaneID(), Profile: "shell"}

	root := Leaf(a)
	root = SplitH(root, root, b)
	leafB := root.FindByID(b.ID)
	root = SplitV(root, leafB, c)

	got := Marshal(root)
	want := `split-v:0.500{leaf:profile=shell}{split-h:0.500{leaf:profile=shell}{leaf:profile=shell}}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestUnmarshalRoundtrip(t *testing.T) {
	cases := []string{
		`leaf:profile=shell`,
		`leaf:profile=shell,title="zsh"`,
		`split-v:0.500{leaf:profile=shell}{leaf:profile=shell}`,
		`split-h:0.300{leaf:profile=shell,title="a"}{split-v:0.500{leaf:profile=shell}{leaf:profile=shell}}`,
	}
	spawn := func(spec LeafSpec) (*session.Pane, error) {
		return &session.Pane{ID: session.NewPaneID(), Profile: spec.Profile, Title: spec.Title}, nil
	}
	for _, src := range cases {
		root, err := Unmarshal(src, spawn)
		if err != nil {
			t.Fatalf("Unmarshal(%q): %v", src, err)
		}
		round := Marshal(root)
		if round != src {
			t.Errorf("round-trip mismatch:\nsrc  %s\nback %s", src, round)
		}
	}
}

func TestUnmarshalRejectsBadInput(t *testing.T) {
	spawn := func(LeafSpec) (*session.Pane, error) {
		return &session.Pane{ID: session.NewPaneID()}, nil
	}
	for _, src := range []string{
		"garbage",
		"split-v:1.5{leaf:profile=x}{leaf:profile=x}",
		"split-v:0.5{leaf:profile=x}",                  // missing second panel
		"split-v:0.5{leaf:profile=x}{leaf:profile=x}x", // trailing garbage
	} {
		if _, err := Unmarshal(src, spawn); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

func TestMarshalOrientation(t *testing.T) {
	a := &session.Pane{ID: session.NewPaneID(), Profile: "p"}
	b := &session.Pane{ID: session.NewPaneID(), Profile: "p"}

	root := Leaf(a)
	root = SplitH(root, root, b)
	if root.Orientation != views.SplitVertical {
		t.Fatal("SplitH must produce SplitVertical")
	}
	got := Marshal(root)
	if got[:7] != "split-v" {
		t.Errorf("SplitVertical should marshal as split-v: got %q", got)
	}
}
