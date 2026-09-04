package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
)

func splitProfileMux(t *testing.T, pane *session.Pane, inherit bool) *Mux {
	t.Helper()
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	w := views.NewWindow(geom.NewRect(1, 1, 41, 13), "test", 1)
	desk.InsertWindow(w)
	leaf := layout.Leaf(pane)
	ws := &windowState{Frame: w, Root: leaf, Focus: leaf}
	return &Mux{
		App:         &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		Opts:        Options{Config: &config.Config{General: config.General{InheritSplitCWD: inherit}}},
		windows:     map[views.View]*windowState{w.Self(): ws},
		windowOrder: []views.View{w.Self()},
	}
}

func TestEffectiveSplitProfileInheritsLocalCWDWithoutMutation(t *testing.T) {
	pane := &session.Pane{ID: session.NewPaneID(), CWD: "/work/project"}
	m := splitProfileMux(t, pane, true)
	source := &profile.Profile{Name: "shell", CWD: "~"}

	got := m.effectiveSplitProfile(source)
	if got == source {
		t.Fatal("effective profile aliases the stored profile")
	}
	if got.CWD != "/work/project" {
		t.Fatalf("effective cwd = %q, want focused local cwd", got.CWD)
	}
	if source.CWD != "~" {
		t.Fatalf("stored profile mutated to cwd %q", source.CWD)
	}
}

func TestEffectiveSplitProfileFallsBackForDisabledEmptyOrSSHSource(t *testing.T) {
	tests := []struct {
		name    string
		inherit bool
		pane    *session.Pane
	}{
		{name: "disabled", inherit: false, pane: &session.Pane{ID: session.NewPaneID(), CWD: "/work"}},
		{name: "empty cwd", inherit: true, pane: &session.Pane{ID: session.NewPaneID()}},
		{name: "ssh cwd", inherit: true, pane: &session.Pane{ID: session.NewPaneID(), CWD: "/remote/app", SSHAlias: "prod"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := splitProfileMux(t, tt.pane, tt.inherit)
			source := &profile.Profile{Name: "shell", CWD: "~/configured"}
			if got := m.effectiveSplitProfile(source); got.CWD != source.CWD {
				t.Fatalf("effective cwd = %q, want profile cwd %q", got.CWD, source.CWD)
			}
		})
	}
}
