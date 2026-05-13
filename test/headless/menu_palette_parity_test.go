package headless

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	fvmenus "github.com/oldwired/fv-go/pkg/fv/menus"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/menus"
	"github.com/oldwired/fvmux/internal/palette"
)

// TestMenuAndPaletteCoverEveryVisibleCommand asserts the load-bearing
// contract: every non-Hidden command in the default registry is reachable
// via *both* the menu bar and the command palette, exactly once each.
//
// This guards against the two failure modes that can creep in as the
// registry grows: (1) a category typo so the menu can't find the entry
// even though the palette can, and (2) a duplicate hardcoded menu item
// (the early build.go shipped a redundant File→Quit; this test would
// have caught it).
func TestMenuAndPaletteCoverEveryVisibleCommand(t *testing.T) {
	reg := commands.Defaults()

	want := map[uint16]*commands.Command{}
	for _, c := range reg.All() {
		if c.Hidden {
			continue
		}
		want[c.ID] = c
	}

	mb := menus.Build(geom.NewRect(0, 0, 200, 1), reg)
	menuIDs := map[uint16]int{}
	walkMenu(mb.Menu, menuIDs)

	paletteIDs := map[uint16]int{}
	for _, c := range palette.VisibleCommands(reg) {
		paletteIDs[c.ID]++
	}

	for id, c := range want {
		if menuIDs[id] == 0 {
			t.Errorf("menu missing command id=%d (%s, category %q)",
				id, c.Name, c.Category)
		}
		if menuIDs[id] > 1 {
			t.Errorf("menu has duplicate command id=%d (%s) — %d copies",
				id, c.Name, menuIDs[id])
		}
		if paletteIDs[id] == 0 {
			t.Errorf("palette missing command id=%d (%s)", id, c.Name)
		}
		if paletteIDs[id] > 1 {
			t.Errorf("palette has duplicate command id=%d (%s) — %d copies",
				id, c.Name, paletteIDs[id])
		}
	}

	// Reverse direction: nothing surfaces that isn't in the registry.
	for id := range menuIDs {
		if _, ok := want[id]; !ok && id != 0 {
			t.Errorf("menu shows command id=%d that's not in the registry "+
				"(or is Hidden)", id)
		}
	}
}

// walkMenu accumulates every leaf Item.Command in counts. Submenus
// recurse; separators (Command==0 + Sub==nil) are skipped.
func walkMenu(m *fvmenus.Menu, counts map[uint16]int) {
	if m == nil {
		return
	}
	for _, it := range m.Items {
		if it.Sub != nil {
			walkMenu(it.Sub, counts)
			continue
		}
		if it.IsSeparator() {
			continue
		}
		counts[it.Command]++
	}
}
