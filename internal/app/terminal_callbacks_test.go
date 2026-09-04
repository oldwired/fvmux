package app

import (
	"os"
	"testing"
	"time"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/profile"
)

func TestInstantiateProfileDoesNotMissImmediateExit(t *testing.T) {
	scheduled := make(chan func(), 4)
	views.SetCallSoon(func(fn func()) { scheduled <- fn })
	defer views.SetCallSoon(nil)

	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	m.Opts.Config = config.Defaults()
	p := &profile.Profile{
		Name:    "immediate-exit",
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestTerminalCallbackHelperProcess$"},
		Env:     map[string]string{"GO_WANT_FVMUX_EXIT_HELPER": "1"},
	}
	pane, err := m.instantiateProfile(p, geom.NewRect(0, 0, 40, 12))
	if err != nil {
		t.Fatalf("instantiateProfile: %v", err)
	}

	var deliver func()
	select {
	case deliver = <-scheduled:
	case <-time.After(2 * time.Second):
		t.Fatal("immediately exiting child never scheduled OnExit")
	}
	deliver()
	if !pane.Dead {
		t.Fatal("pre-start callback wiring missed the child's immediate exit")
	}
}

func TestTerminalCallbackHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_FVMUX_EXIT_HELPER") != "1" {
		return
	}
	os.Exit(0)
}
