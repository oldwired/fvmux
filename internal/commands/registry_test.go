package commands

import "testing"

func TestRegistryRoundTrip(t *testing.T) {
	r := New()
	c := &Command{ID: 9000, Category: "Test", Name: "t", Chord: "C-x t"}
	r.Register(c)

	if got := r.ByID(9000); got != c {
		t.Fatalf("ByID: got %v want %v", got, c)
	}
	if got := r.LookupChord("C-x t"); got != c {
		t.Fatalf("LookupChord: got %v want %v", got, c)
	}
	if got := r.ByCategory("Test"); len(got) != 1 || got[0] != c {
		t.Fatalf("ByCategory: got %v want [%v]", got, c)
	}
}

func TestRegistryDuplicateIDPanics(t *testing.T) {
	r := New()
	r.Register(&Command{ID: 9001, Name: "a"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate ID")
		}
	}()
	r.Register(&Command{ID: 9001, Name: "b"})
}

func TestLookupChordUnboundReturnsNil(t *testing.T) {
	r := New()
	r.Register(&Command{ID: 9002, Name: "no-chord"}) // empty Chord
	if got := r.LookupChord(""); got != nil {
		t.Fatalf("LookupChord(\"\"): got %v want nil", got)
	}
	if got := r.LookupChord("C-x z"); got != nil {
		t.Fatalf("LookupChord unknown: got %v want nil", got)
	}
}

func TestDefaultsHasStepOneCommands(t *testing.T) {
	r := Defaults()
	for _, id := range []uint16{CmdNewWindow, CmdCheatsheet, CmdQuit, CmdLiteralPrefix} {
		if r.ByID(id) == nil {
			t.Errorf("Defaults missing command ID %d", id)
		}
	}
	if r.LookupChord("C-g c") == nil {
		t.Error("Defaults missing C-g c binding")
	}
	if r.LookupChord("C-g ?") == nil {
		t.Error("Defaults missing C-g ? binding")
	}
}

func TestAllSortedByID(t *testing.T) {
	r := New()
	r.Register(&Command{ID: 30, Name: "c"})
	r.Register(&Command{ID: 10, Name: "a"})
	r.Register(&Command{ID: 20, Name: "b"})
	all := r.All()
	if len(all) != 3 || all[0].ID != 10 || all[1].ID != 20 || all[2].ID != 30 {
		t.Fatalf("All not sorted by ID: %v", all)
	}
}
