package headless

import (
	"strings"
	"testing"
	"unicode"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	fvmenus "github.com/oldwired/fv-go/pkg/fv/menus"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/menus"
	"github.com/oldwired/fvmux/internal/palette"
)

// TestMenuAndPaletteCoverEveryVisibleCommand asserts the load-bearing
// contract: every non-Hidden command in the default registry is reachable
// from the command palette and, unless explicitly self-referential, from the
// menu bar exactly once. Activate Menu Bar intentionally has no row inside the
// menu it opens.
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
		menuOptional := id == commands.CmdOpenMenu
		if menuIDs[id] == 0 && !menuOptional {
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

// TestMenuHotkeysUniqueWithinEachMenu guards against Borland ~X~ hotkey
// collisions: within any single menu (and each submenu independently),
// no two entries may claim the same accelerator letter, or the second is
// unreachable by keyboard. The bar row and every submenu are separate
// namespaces.
func TestMenuHotkeysUniqueWithinEachMenu(t *testing.T) {
	reg := commands.Defaults()
	mb := menus.Build(geom.NewRect(0, 0, 200, 1), reg)
	checkMenuHotkeys(t, "menubar", mb.Menu)
}

func checkMenuHotkeys(t *testing.T, path string, m *fvmenus.Menu) {
	if m == nil {
		return
	}
	seen := map[rune]string{}
	for _, it := range m.Items {
		if it.IsSeparator() {
			continue
		}
		if hk, ok := hotkeyOf(it.Name); ok {
			if prev, dup := seen[hk]; dup {
				t.Errorf("menu %q: hotkey %q collides between %q and %q",
					path, string(hk), prev, it.Name)
			} else {
				seen[hk] = it.Name
			}
		}
		if it.Sub != nil {
			checkMenuHotkeys(t, path+" › "+it.Name, it.Sub)
		}
	}
}

// hotkeyOf extracts the lowercased accelerator letter from a "~X~" marker
// in a menu label, or reports false when the label has none.
func hotkeyOf(name string) (rune, bool) {
	i := strings.Index(name, "~")
	if i < 0 || i+2 >= len(name) || name[i+2] != '~' {
		return 0, false
	}
	return unicode.ToLower(rune(name[i+1])), true
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
