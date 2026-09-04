package app

import (
	"strings"
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	fvmenus "github.com/oldwired/fv-go/pkg/fv/menus"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/layout"
	muxmenus "github.com/oldwired/fvmux/internal/menus"
	"github.com/oldwired/fvmux/internal/session"
)

func availabilityMux(t *testing.T) (*Mux, *windowState) {
	t.Helper()
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 100, 30))
	w := views.NewWindow(geom.NewRect(1, 1, 61, 21), "one", 1)
	desk.InsertWindow(w)
	pane := &session.Pane{ID: session.NewPaneID()}
	leaf := layout.Leaf(pane)
	ws := &windowState{Frame: w, Root: leaf, Focus: leaf, Number: 1}
	m := &Mux{
		App:         &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		Reg:         commands.Defaults(),
		windows:     map[views.View]*windowState{w.Self(): ws},
		windowOrder: []views.View{w.Self()},
	}
	m.wireCommandAvailability()
	return m, ws
}

func commandEnabled(t *testing.T, m *Mux, id uint16) bool {
	t.Helper()
	c := m.Reg.ByID(id)
	if c == nil || c.Enabled == nil {
		t.Fatalf("command %d has no availability predicate", id)
	}
	return c.Enabled(nil)
}

func TestCommandAvailabilityTracksPaneLifecycleAndShape(t *testing.T) {
	m, ws := availabilityMux(t)
	if !commandEnabled(t, m, commands.CmdSplitH) {
		t.Error("split should be enabled with a focused pane")
	}
	if commandEnabled(t, m, commands.CmdFocusNext) {
		t.Error("next pane should be disabled with one pane")
	}
	if !commandEnabled(t, m, commands.CmdSendSIGINT) {
		t.Error("signal should be enabled for a live focused pane")
	}
	if commandEnabled(t, m, commands.CmdRespawnPane) {
		t.Error("respawn should be disabled while the pane is live")
	}

	second := &session.Pane{ID: session.NewPaneID()}
	ws.Root = layout.SplitH(ws.Root, ws.Focus, second)
	ws.Focus = ws.Root.FindByID(second.ID)
	if !commandEnabled(t, m, commands.CmdFocusNext) ||
		!commandEnabled(t, m, commands.CmdCycleLayout) ||
		!commandEnabled(t, m, commands.CmdEnterResize) {
		t.Error("multi-pane commands did not enable after splitting")
	}

	ws.Focus.Pane.Dead = true
	if commandEnabled(t, m, commands.CmdSendSIGINT) {
		t.Error("signal stayed enabled for a dead pane")
	}
	if !commandEnabled(t, m, commands.CmdRespawnPane) {
		t.Error("respawn did not enable for a dead pane")
	}
}

func TestJoinAvailabilityRequiresEligibleOtherWindow(t *testing.T) {
	m, _ := availabilityMux(t)
	if commandEnabled(t, m, commands.CmdJoinFrom) {
		t.Error("join should be disabled with no source window")
	}
	w := views.NewWindow(geom.NewRect(5, 5, 45, 17), "two", 2)
	p := &session.Pane{ID: session.NewPaneID()}
	leaf := layout.Leaf(p)
	m.windows[w.Self()] = &windowState{Frame: w, Root: leaf, Focus: leaf, Number: 2}
	m.windowOrder = append(m.windowOrder, w.Self())
	if !commandEnabled(t, m, commands.CmdJoinFrom) {
		t.Error("join did not enable for an eligible single-pane source")
	}
}

func TestMenuAndContextUseRegistryAvailability(t *testing.T) {
	m, ws := availabilityMux(t)
	bar := muxmenus.Build(geom.NewRect(0, 0, 100, 1), m.Reg)
	respawn := findMenuCommand(bar.Menu, commands.CmdRespawnPane)
	if respawn == nil || !respawn.Disabled {
		t.Fatalf("live-pane Respawn menu item = %#v, want disabled", respawn)
	}
	if items := paneContextMenuItems(m.Reg); !hasItemContaining(items, "Respawn Dead Pane [disabled]") {
		t.Fatalf("context menu did not mark Respawn disabled: %v", items)
	}

	m.App.MenuBar = bar
	ws.Focus.Pane.Dead = true
	m.refreshMenuAvailability()
	if respawn.Disabled {
		t.Error("existing menu item did not refresh after pane death")
	}
}

func findMenuCommand(menu *fvmenus.Menu, id uint16) *fvmenus.Item {
	if menu == nil {
		return nil
	}
	for _, item := range menu.Items {
		if item == nil {
			continue
		}
		if item.Command == id {
			return item
		}
		if found := findMenuCommand(item.Sub, id); found != nil {
			return found
		}
	}
	return nil
}

func hasItemContaining(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
