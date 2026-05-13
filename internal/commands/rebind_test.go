package commands

import "testing"

func TestRebindPrefixRewritesAllChords(t *testing.T) {
	r := New()
	r.Register(&Command{ID: 1, Name: "split-h", Chord: "C-g %"})
	r.Register(&Command{ID: 2, Name: "literal", Chord: "C-g C-g"})
	r.Register(&Command{ID: 3, Name: "no-chord", Chord: ""})

	r.RebindPrefix("C-g", "C-b")

	if got := r.ByID(1).Chord; got != "C-b %" {
		t.Errorf("split-h chord = %q want %q", got, "C-b %")
	}
	if got := r.ByID(2).Chord; got != "C-b C-b" {
		t.Errorf("literal chord = %q want %q", got, "C-b C-b")
	}
	if got := r.ByID(3).Chord; got != "" {
		t.Errorf("no-chord should stay empty; got %q", got)
	}
	if r.LookupChord("C-g %") != nil {
		t.Error("old chord still resolvable")
	}
	if r.LookupChord("C-b %") == nil {
		t.Error("new chord not resolvable")
	}
	if r.LookupChord("C-b C-b") == nil {
		t.Error("new literal chord not resolvable")
	}
}

func TestRebindPrefixIdempotent(t *testing.T) {
	r := New()
	r.Register(&Command{ID: 1, Name: "x", Chord: "C-g x"})
	r.RebindPrefix("C-g", "C-g")
	if r.ByID(1).Chord != "C-g x" {
		t.Errorf("no-op rebind altered chord: %q", r.ByID(1).Chord)
	}
}
