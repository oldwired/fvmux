package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/statusbar"
)

func newBareMux() *Mux {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows: map[views.View]*windowState{},
	}
	m.Opts.Config = config.Defaults()
	return m
}

// TestApplyReloadedConfig_ForwardsClockFormat is the regression for finding
// #31: ReloadConfig must forward appearance.status_clock into the live
// statusbar.Bar, whose ClockFormat is copied once at Build time. The config-
// application core is split into applyReloadedConfig so it can be exercised
// without ReloadConfig's trailing modal msgbox.
func TestApplyReloadedConfig_ForwardsClockFormat(t *testing.T) {
	m := newBareMux()
	bar := statusbar.Build(geom.NewRect(0, 23, 80, 1), "sess", "15:04")
	m.Opts.StatusBar = bar
	if bar.ClockFormat != "15:04" {
		t.Fatalf("precondition: bar.ClockFormat = %q, want 15:04", bar.ClockFormat)
	}

	cfg := config.Defaults()
	cfg.Appearance.StatusClock = "Mon 15:04:05"
	m.applyReloadedConfig(cfg)

	if bar.ClockFormat != "Mon 15:04:05" {
		t.Errorf("applyReloadedConfig did not forward StatusClock; bar.ClockFormat = %q, want %q",
			bar.ClockFormat, "Mon 15:04:05")
	}
	if m.Opts.Config != cfg {
		t.Error("applyReloadedConfig did not swap in the reloaded config")
	}
}

// TestApplyReloadedConfig_ReappliesWindowShadow guards the other half of the
// extracted helper: window_shadow is re-applied to already-open frames (the
// reload path, distinct from registerWindow's new-window path).
func TestApplyReloadedConfig_ReappliesWindowShadow(t *testing.T) {
	m := newBareMux()
	w := views.NewWindow(geom.NewRect(0, 0, 20, 10), "shadowed", 1)
	if !w.GetState(consts.SfShadow) {
		t.Fatal("precondition: fv-go NewWindow should set SfShadow")
	}
	m.windows[w.Self()] = &windowState{ID: session.NewWindowID(), Number: 1, Frame: w}

	cfg := config.Defaults()
	cfg.Appearance.WindowShadow = false
	m.applyReloadedConfig(cfg)
	if w.GetState(consts.SfShadow) {
		t.Error("window_shadow=false but SfShadow still set after applyReloadedConfig")
	}

	// Flipping it back on restores the flag on the same open frame.
	m.applyReloadedConfig(config.Defaults()) // WindowShadow default true
	if !w.GetState(consts.SfShadow) {
		t.Error("window_shadow=true but SfShadow not restored after applyReloadedConfig")
	}
}
