package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/profile"
)

func headlessMuxForKeyboardTest(t *testing.T) *Mux {
	t.Helper()
	backend := term.NewHeadless(80, 24)
	program := fvapp.NewProgram(backend)
	program.SetDesktop(fvapp.NewDesktop(geom.NewRect(0, 1, 80, 23)))
	a := &fvapp.Application{Program: program}
	t.Cleanup(a.Done)
	paths := config.Paths{Root: t.TempDir(), StateRoot: t.TempDir()}
	return NewMux(a, commands.Defaults(), Options{
		Paths: paths, Config: config.Defaults(), Profiles: profile.Defaults(),
	})
}

func TestOpenMenuCommandPostsCmMenu(t *testing.T) {
	m := headlessMuxForKeyboardTest(t)
	command := m.Reg.ByID(commands.CmdOpenMenu)
	if command == nil || command.Action == nil {
		t.Fatal("Activate Menu Bar command is not wired")
	}
	command.Action(&commands.Ctx{App: m.App})
	event, ok := views.GetEventQueue().Get()
	if !ok || event.What != consts.EvCommand || event.Command != consts.CmMenu {
		t.Fatalf("posted event = %+v, ok=%v; want CmMenu", event, ok)
	}
}

func TestEveryBoundDefaultCommandHasAction(t *testing.T) {
	m := headlessMuxForKeyboardTest(t)
	for _, command := range m.Reg.All() {
		if command.Chord != "" && command.Action == nil {
			t.Errorf("bound command %q (%s) has no action", command.Name, command.Chord)
		}
	}
}
