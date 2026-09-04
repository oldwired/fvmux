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
	"github.com/oldwired/fvmux/internal/prefix"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/statusbar"
	"github.com/oldwired/fvmux/internal/sysmon"
)

func main() {
	// All fatal-error paths return out of run() so its defers unwind —
	// most importantly a.Done(), which restores the terminal out of raw
	// mode/alternate screen. An os.Exit inside run() would skip that and
	// leave the user's shell wedged (and the error message invisible
	// inside the alternate screen), so the message is printed here,
	// after the restore.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fvmux:", err)
		os.Exit(1)
	}
}

func run() error {
	f := parseFlags()
	if f.ShowVersion {
		fmt.Println("fvmux", Version)
		return nil
	}

	if f.Session != "" {
		if err := config.ValidSessionName(f.Session); err != nil {
			return err
		}
	}
	paths := config.Default().WithRoot(f.Config)
	if f.CheckConfig {
		return checkConfig(paths, os.Stdout)
	}
	if err := paths.EnsureDirs(); err != nil {
		return fmt.Errorf("ensuring config dirs: %w", err)
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
		// config.toml is auto-overwritten on the next theme/prefix change,
		// so a malformed file would be silently destroyed. Back it up and
		// start clean instead of clobbering the user's edits.
		if bak, berr := config.BackupCorrupt(paths.ConfigFile()); berr == nil {
			fmt.Fprintf(os.Stderr, "fvmux: config.toml failed to parse (%v); backed up to %s, continuing on defaults\n", err, bak)
		} else {
			fmt.Fprintln(os.Stderr, "fvmux: warning loading config:", err)
		}
		cfg = config.Defaults()
	}
	// state.toml is likewise auto-written — back up a corrupt one so the
	// next SaveState doesn't overwrite it.
	if _, serr := config.LoadState(paths.StateFile()); serr != nil {
		if bak, berr := config.BackupCorrupt(paths.StateFile()); berr == nil {
			fmt.Fprintf(os.Stderr, "fvmux: state.toml failed to parse (%v); backed up to %s\n", serr, bak)
		} else {
			fmt.Fprintln(os.Stderr, "fvmux: warning loading state:", serr)
		}
	}
	profiles, err := profile.Load(paths.ProfilesFile())
	if err != nil {
		// Hand-edited file, never auto-overwritten: warn and run on
		// defaults, leaving the user's file intact to fix.
		fmt.Fprintln(os.Stderr, "fvmux: warning loading profiles (using defaults):", err)
	}

	reg := commands.Defaults()
	prefixSpec := prefix.Lookup(cfg.General.PrefixKey)
	if pk := cfg.General.PrefixKey; pk != "" && pk != prefixSpec.ConfigKey {
		slog.Warn("unrecognised prefix_key, using default",
			"prefix_key", pk, "using", prefixSpec.ConfigKey)
	}
	// Apply user overrides from keybindings.toml FIRST, while the registry
	// is still in the default "C-g" prefix space — keybindings.toml chords
	// are normalized into factory space (the template recommends
	// "<prefix> X"). Do this before terminal initialization so a concise
	// diagnostic summary remains visible in the user's shell.
	// Empty / missing file is fine; invalid entries are skipped with
	// structured diagnostics while valid entries remain usable.
	if overrides, diagnostics, err := config.LoadKeybindings(paths.KeybindingsFile(), prefixSpec.ChordToken); err == nil {
		if len(overrides) > 0 {
			diagnostics = append(diagnostics, reg.ApplyOverrides(overrides)...)
		}
		errors, warnings := 0, 0
		for _, diagnostic := range diagnostics {
			if diagnostic.Severity == "error" {
				errors++
				slog.Error("keybindings.toml", "diagnostic", diagnostic.String())
			} else {
				warnings++
				slog.Warn("keybindings.toml", "diagnostic", diagnostic.String())
			}
		}
		if errors+warnings > 0 {
			fmt.Fprintf(os.Stderr, "fvmux: keybindings.toml: %d error(s), %d warning(s); full details are available in the log viewer\n", errors, warnings)
		}
	} else {
		fmt.Fprintln(os.Stderr, "fvmux: warning loading keybindings:", err)
	}
	// THEN rebind the whole registry — defaults and overrides alike — onto
	// the configured prefix, so a "C-g w" override correctly follows to
	// "C-b w" when the user runs with Ctrl-B. Resolve through Lookup and
	// rebind with the resolved token, never the raw config value: an
	// unlisted prefix_key ("C-x", "ctrl-b") must fall back to the default
	// for the chords AND the listener together, or every prefix binding
	// goes dead at startup.
	if prefixSpec.ChordToken != "C-g" {
		reg.RebindPrefix("C-g", prefixSpec.ChordToken)
	}

	a, err := fvapp.NewApplication()
	if err != nil {
		return err
	}
	defer a.Done()

	cols, rows := a.BaseView().Size.X, a.BaseView().Size.Y
	var mux *muxapp.Mux // captured by the rebuild closure; assigned below.
	rebuildMenu := func() {
		var extras menus.Extras
		if mux != nil {
			extras = mux.BuildMenuExtras()
		}
		// Read the desktop width at rebuild time — capturing the startup
		// cols would size every later rebuild (theme save, reload,
		// wizard) to the original terminal width.
		w := a.BaseView().Size.X
		a.SetMenuBar(menus.BuildWithExtras(geom.NewRect(0, 0, w, 1), reg, extras))
	}

	bar := statusbar.Build(
		geom.NewRect(0, rows-1, cols, rows),
		f.Session,
		cfg.Appearance.StatusClock,
	)

	mux = muxapp.NewMux(a, reg, muxapp.Options{
		Paths:       paths,
		Config:      cfg,
		Profiles:    profiles,
		StatusBar:   bar,
		SessionName: f.Session,
		Version:     Version,
		RefreshUI:   rebuildMenu,
	})

	rebuildMenu()
	a.SetStatusLine(bar.Line)

	a.OnCommand = func(cmd uint16, ev *drivers.Event) bool {
		// Dynamic-menu items (themes/profiles/sessions/etc.) are
		// stored in the Mux's per-rebuild dispatch table.
		if mux.DispatchDynamic(cmd) {
			return true
		}
		return muxapp.Dispatch(reg, &commands.Ctx{App: a}, cmd)
	}
	a.OnQuitRequest = mux.OnQuitRequested
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
		// Last-ditch session save before the panic unwinds. Runs on the
		// recover path (UI goroutine), so reading window state is safe.
		_ = mux.SaveSessionSilent()
		slog.Error("panic in main loop", "recovered", fmt.Sprintf("%v", recovered))
	}

	if err := bootstrapInitial(mux, paths, f); err != nil {
		return err
	}

	mux.InstallPrefixListener()
	mux.InstallSignalHandlers()
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
	return nil
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
