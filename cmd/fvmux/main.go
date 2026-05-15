// Command fvmux is a terminal multiplexer with draggable floating
// windows, each containing a split tree of panes. Built on the fv-go
// TUI framework.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"

	muxapp "github.com/oldwired/fvmux/internal/app"
	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/logs"
	"github.com/oldwired/fvmux/internal/menus"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/statusbar"
	"github.com/oldwired/fvmux/internal/sysmon"
)

func main() {
	f := parseFlags()
	if f.ShowVersion {
		fmt.Println("fvmux", Version)
		return
	}

	paths := config.Default().WithRoot(f.Config)
	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintln(os.Stderr, "fvmux: ensuring config dirs:", err)
		os.Exit(1)
	}
	if err := config.SeedDefaults(paths); err != nil {
		fmt.Fprintln(os.Stderr, "fvmux: warning seeding default configs:", err)
	}
	if err := logs.Init(f.Log, 4096); err != nil {
		fmt.Fprintln(os.Stderr, "fvmux: warning opening log file:", err)
	}
	defer func() { _ = logs.Close() }()
	// Start system-stats sampler before anything that reads from it.
	sysmon.Start(1 * time.Second)
	defer sysmon.Stop()

	cfg, err := config.Load(paths.ConfigFile())
	if err != nil {
		fmt.Fprintln(os.Stderr, "fvmux: warning loading config:", err)
	}
	profiles, err := profile.Load(paths.ProfilesFile())
	if err != nil {
		fmt.Fprintln(os.Stderr, "fvmux: warning loading profiles:", err)
	}

	a, err := fvapp.NewApplication()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fvmux:", err)
		os.Exit(1)
	}
	defer a.Done()

	cols, rows := a.BaseView().Size.X, a.BaseView().Size.Y
	reg := commands.Defaults()
	// Rebind chords if the user previously chose a non-default prefix
	// — Defaults() registers everything under "C-g " so the menu/palette
	// stay consistent with whichever prefix is configured.
	if cfg.General.PrefixKey != "" && cfg.General.PrefixKey != "C-g" {
		reg.RebindPrefix("C-g", cfg.General.PrefixKey)
	}
	// Apply any user overrides from keybindings.toml. Empty / missing
	// file is fine; bad entries are skipped silently (Lookups return nil).
	if overrides, err := config.LoadKeybindings(paths.KeybindingsFile()); err == nil {
		if len(overrides) > 0 {
			reg.ApplyOverrides(overrides)
		}
	} else {
		fmt.Fprintln(os.Stderr, "fvmux: warning loading keybindings:", err)
	}
	var mux *muxapp.Mux // captured by the rebuild closure; assigned below.
	rebuildMenu := func() {
		var extras menus.Extras
		if mux != nil {
			extras = mux.BuildMenuExtras()
		}
		a.SetMenuBar(menus.BuildWithExtras(geom.NewRect(0, 0, cols, 1), reg, extras))
	}

	bar := statusbar.Build(
		geom.NewRect(0, rows-1, cols, rows),
		f.Session,
		cfg.Appearance.StatusClock,
	)

	mux = muxapp.NewMux(a, reg, muxapp.Options{
		Paths:           paths,
		Config:          cfg,
		Profiles:        profiles,
		StatusBar:       bar,
		SessionName:     f.Session,
		StartingProfile: f.Profile,
		Version:         Version,
		RefreshUI:       rebuildMenu,
	})

	rebuildMenu()
	a.SetStatusLine(bar.Line)

	a.OnCommand = func(cmd uint16, ev *drivers.Event) bool {
		// Internal triggers that carry a payload on InfoPtr need to
		// see the event directly; the registry's Action(ctx) signature
		// has no slot for it. Route them here, fall through otherwise.
		if cmd == commands.CmdAutoClosePane {
			if pane, ok := ev.InfoPtr.(*session.Pane); ok {
				mux.AutoClosePane(pane)
				return true
			}
		}
		// Dynamic-menu items (themes/profiles/sessions/etc.) are
		// stored in the Mux's per-rebuild dispatch table.
		if mux.DispatchDynamic(cmd) {
			return true
		}
		return muxapp.Dispatch(reg, &commands.Ctx{App: a}, cmd)
	}
	a.OnQuitRequest = func() bool {
		if !mux.CanQuit() {
			return false
		}
		// Persist session on graceful quit (no-op if no session name).
		_ = mux.SaveSessionSilent()
		return true
	}
	// Surface fv-go's new diagnostic hooks via slog so the in-app log
	// viewer (Ctrl-G L) catches backend / queue failures that would
	// otherwise go unobserved. Defaults are conservative — these only
	// fire on genuine errors / overflow.
	a.OnBackendError = func(err error) {
		slog.Warn("backend error", "err", err)
	}
	a.OnEventDropped = func(ev drivers.Event) {
		slog.Warn("event dropped", "what", ev.What, "command", ev.Command)
	}
	a.OnPanic = func(recovered any) {
		slog.Error("panic in main loop", "recovered", fmt.Sprintf("%v", recovered))
	}

	if err := bootstrapInitial(mux, paths, f); err != nil {
		fmt.Fprintln(os.Stderr, "fvmux:", err)
		os.Exit(1)
	}

	mux.InstallPrefixListener()
	mux.StartTicker()
	defer mux.StopTicker()
	defer mux.ShutdownSSHPool()
	defer mux.StopEggs()

	if !f.NoSplash && cfg.General.SplashEnabled {
		state, _ := config.LoadState(paths.StateFile())
		if !state.FirstRunDone {
			mux.RunFirstRunWizard()
		} else {
			mux.MaybeShowVersionBump()
		}
	}

	a.Run()
}

// bootstrapInitial decides between loading a saved session and opening a
// single starter window. If -session names a saved session, load it;
// otherwise spawn one window using -profile (or the default).
func bootstrapInitial(mux *muxapp.Mux, paths config.Paths, f flags) error {
	if f.Session != "" {
		snap, err := session.Load(paths.SessionFile(f.Session))
		switch {
		case err == nil:
			if err := mux.LoadSession(snap); err != nil {
				return fmt.Errorf("loading session %s: %w", f.Session, err)
			}
			return nil
		case errors.Is(err, fs.ErrNotExist):
			// No saved session yet — fall through and open a fresh window
			// so the user can populate the workspace, then C-g S to save.
		default:
			return fmt.Errorf("loading session %s: %w", f.Session, err)
		}
	}
	_, err := mux.NewWindow(f.Profile)
	return err
}
