package menus

import (
	"strings"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	fvmenus "github.com/oldwired/fv-go/pkg/fv/menus"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
)

func TestBuildWithExtrasAlwaysEnablesRawKeyPassThrough(t *testing.T) {
	reg := commands.Defaults()
	for i := 0; i < 2; i++ {
		bar := BuildWithExtras(geom.NewRect(0, 0, 80, 1), reg, Extras{})
		if !bar.PassThroughRawKeys {
			t.Fatalf("menu rebuild %d disabled PassThroughRawKeys", i+1)
		}
	}
}

func TestBuiltMenuActivationPolicyTracksFocusedView(t *testing.T) {
	for _, raw := range []bool{true, false} {
		t.Run(map[bool]string{true: "terminal", false: "normal"}[raw], func(t *testing.T) {
			defer views.ResetForTest()
			host := views.NewGroup(geom.NewRect(0, 0, 80, 25))
			content := views.NewGroup(geom.NewRect(0, 1, 80, 24))
			focused := views.NewBase(geom.NewRect(0, 1, 80, 24))
			focused.SetSelf(&focused)
			focused.Options |= consts.OfSelectable
			if raw {
				focused.State |= consts.SfRawKeys
			}
			content.Insert(&focused)
			host.Insert(content)
			host.Insert(Build(geom.NewRect(0, 0, 80, 1), commands.Defaults()))

			for _, event := range []drivers.Event{
				{What: consts.EvKeyDown, KeyCode: consts.KbF10},
				{What: consts.EvKeyDown, KeyCode: consts.KbAltF, KeyShift: consts.KbAltShift, UnicodeChar: 'f'},
			} {
				if !raw {
					queue := drivers.NewQueue()
					views.SetEventQueue(queue)
					queue.Put(drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEsc})
				}
				host.HandleEvent(&event)
				if raw && event.What != consts.EvKeyDown {
					t.Fatalf("raw-focused activation key was consumed: %+v", event)
				}
				if !raw && event.What != consts.EvNothing {
					t.Fatalf("normal-focused activation key was not consumed: %+v", event)
				}
			}
		})
	}
}

func TestMenuCommandsAndMnemonicsAreUniquePerScope(t *testing.T) {
	reg := commands.Defaults()
	bar := Build(geom.NewRect(0, 0, 120, 1), reg)
	var walk func(scope string, menu *fvmenus.Menu)
	walk = func(scope string, menu *fvmenus.Menu) {
		seen := map[byte]string{}
		for _, item := range menu.Items {
			if item == nil || item.IsSeparator() {
				continue
			}
			if hot := mnemonic(item.Name); hot != 0 {
				if previous := seen[hot]; previous != "" {
					t.Errorf("menu %s has duplicate mnemonic %q in %q and %q", scope, hot, previous, item.Name)
				}
				seen[hot] = item.Name
			}
			if item.Command != 0 && reg.ByID(item.Command) == nil {
				t.Errorf("menu %s item %q references unknown command %d", scope, item.Name, item.Command)
			}
			if item.Sub != nil {
				walk(scope+" / "+strings.ReplaceAll(item.Name, "~", ""), item.Sub)
			}
		}
	}
	walk("top", bar.Menu)
}

func TestMenuShortcutHintsFollowLiveRegistry(t *testing.T) {
	reg := commands.Defaults()
	reg.RebindPrefix("C-g", "C-b")
	bar := Build(geom.NewRect(0, 0, 120, 1), reg)
	for _, id := range []uint16{commands.CmdResetFirstRun, commands.CmdNewWindow, commands.CmdEnterResize} {
		item := findCommandItem(bar.Menu, id)
		command := reg.ByID(id)
		if item == nil {
			t.Fatalf("command %d missing from menu", id)
		}
		if item.Shortcut != command.Chord {
			t.Errorf("command %q shortcut = %q, want live chord %q", command.Name, item.Shortcut, command.Chord)
		}
	}
}

func findCommandItem(menu *fvmenus.Menu, id uint16) *fvmenus.Item {
	if menu == nil {
		return nil
	}
	for _, item := range menu.Items {
		if item.Command == id {
			return item
		}
		if found := findCommandItem(item.Sub, id); found != nil {
			return found
		}
	}
	return nil
}

func mnemonic(label string) byte {
	for i := 0; i+2 < len(label); i++ {
		if label[i] == '~' && label[i+2] == '~' {
			c := label[i+1]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			return c
		}
	}
	return 0
}
