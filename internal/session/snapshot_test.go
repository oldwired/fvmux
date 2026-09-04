package session

import (
	"path/filepath"
	"testing"
)

func TestSnapshot_RoundTripPreservesFidelityFields(t *testing.T) {
	dir := t.TempDir()
	snap := &Snapshot{
		Version: SnapshotVersion,
		Name:    "work",
		Active:  1,
		Windows: []*WindowSnapshot{
			{Kind: "terminal", ID: 1, Number: 1, Title: "shell", Layout: "leaf shell",
				FocusIndex: 0, Zoomed: 0, SyncInput: false},
			{Kind: "terminal", ID: 2, Number: 2, Title: "logs", Layout: "split h 0.5 leaf a leaf b",
				FocusIndex: 1, Zoomed: 2, SyncInput: true},
			{Kind: "files", ID: 3, Number: 4, Alias: "prod", RemoteCWD: "/srv/app", LocalCWD: "/tmp/work", FocusSide: "local"},
		},
		MetaPath: filepath.Join(dir, "work.toml"),
	}
	if err := snap.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(snap.MetaPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != SnapshotVersion || got.Active != 1 {
		t.Fatalf("header lost: version=%d active=%d", got.Version, got.Active)
	}
	if len(got.Windows) != 3 {
		t.Fatalf("window count = %d", len(got.Windows))
	}
	w := got.Windows[1]
	if w.FocusIndex != 1 {
		t.Errorf("FocusIndex = %d, want 1", w.FocusIndex)
	}
	if w.Zoomed != 2 {
		t.Errorf("Zoomed = %d, want 2", w.Zoomed)
	}
	if !w.SyncInput {
		t.Error("SyncInput lost in round-trip")
	}
	f := got.Windows[2]
	if f.Kind != "files" || f.Alias != "prod" || f.RemoteCWD != "/srv/app" || f.LocalCWD != "/tmp/work" || f.FocusSide != "local" {
		t.Fatalf("files window fidelity lost: %#v", f)
	}
}

// A snapshot written before these fields existed (zero values) must
// restore the prior defaults: first leaf focused, not zoomed, no sync.
func TestSnapshot_ZeroValuesAreOldDefaults(t *testing.T) {
	w := &WindowSnapshot{} // as an old snapshot would unmarshal.
	if w.FocusIndex != 0 {
		t.Errorf("FocusIndex zero value = %d, want 0 (first leaf)", w.FocusIndex)
	}
	if w.Zoomed != 0 {
		t.Errorf("Zoomed zero value = %d, want 0 (none)", w.Zoomed)
	}
	if w.SyncInput {
		t.Error("SyncInput zero value should be false")
	}
}
