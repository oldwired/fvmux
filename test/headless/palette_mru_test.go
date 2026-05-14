package headless

import (
	"reflect"
	"testing"

	"github.com/oldwired/fvmux/internal/palette"
)

// TestMRUBumpFreshEntry: bumping an id not yet in the ring prepends it.
func TestMRUBumpFreshEntry(t *testing.T) {
	got := palette.MRUBump([]uint16{1, 2, 3}, 4)
	want := []uint16{4, 1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MRUBump fresh = %v, want %v", got, want)
	}
}

// TestMRUBumpExistingMovesToFront: re-bumping a known id reorders.
func TestMRUBumpExistingMovesToFront(t *testing.T) {
	got := palette.MRUBump([]uint16{1, 2, 3}, 2)
	want := []uint16{2, 1, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MRUBump existing = %v, want %v", got, want)
	}
}

// TestMRUBumpCapsAtTen: the ring never grows past MRUCap entries.
func TestMRUBumpCapsAtTen(t *testing.T) {
	in := []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got := palette.MRUBump(in, 11)
	if len(got) != palette.MRUCap {
		t.Errorf("MRUBump cap: got len=%d, want %d", len(got), palette.MRUCap)
	}
	if got[0] != 11 {
		t.Errorf("MRUBump cap: front=%d, want 11", got[0])
	}
}
