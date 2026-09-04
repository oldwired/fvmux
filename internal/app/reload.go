package app

import (
	"log/slog"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/prefix"
	"github.com/oldwired/fvmux/internal/profile"
	muxtheme "github.com/oldwired/fvmux/internal/theme"
)

// ReloadConfig re-reads config.toml / profiles.toml / keybindings.toml
// from disk, restores the registry to factory chords, then re-applies
// the prefix-key choice and any user overrides. Refreshes menu + status
// bar so the new chord strings show up immediately.
//
// Errors are surfaced via msgbox; partial reloads are best-effort —
// each file is independent.
func (m *Mux) ReloadConfig() {
	slog.Info("reload: starting")
	paths := m.Opts.Paths
	hadError := false
	warnings := 0

	if cfg, err := config.Load(paths.ConfigFile()); err == nil {
		m.applyReloadedConfig(cfg)
	} else {
		hadError = true
		slog.Warn("reload: config.toml", "err", err)
	}

	if profiles, err := profile.Load(paths.ProfilesFile()); err == nil {
		m.Opts.Profiles = profiles
	} else {
		hadError = true
		slog.Warn("reload: profiles.toml", "err", err)
	}

	activeSpec := prefix.Lookup(m.Opts.Config.General.PrefixKey)
	overrides, diagnostics, err := config.LoadKeybindings(paths.KeybindingsFile(), activeSpec.ChordToken)
	if err != nil {
		hadError = true
		slog.Warn("reload: keybindings.toml", "err", err)
	}

	// Factory baseline → user overrides → prefix-key. Order matters:
	// overrides are authored in the default "C-g" space, so they must be
	// applied before the prefix rebind sweeps every chord onto the
	// configured prefix (the rebind then carries an overridden "C-g w"
	// to "C-b w" too). Resetting first ensures a removed user override
	// actually goes away.
	m.Reg.ResetChords()
	if len(overrides) > 0 {
		diagnostics = append(diagnostics, m.Reg.ApplyOverrides(overrides)...)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			hadError = true
			slog.Error("reload: keybindings.toml", "diagnostic", diagnostic.String())
		} else {
			warnings++
			slog.Warn("reload: keybindings.toml", "diagnostic", diagnostic.String())
		}
	}
	// Resolve through Lookup FIRST and rebind with the resolved token —
	// never the raw config string — so the registry's chords and the
	// armed listener can't diverge (an unlisted prefix_key falls back to
	// the default for BOTH). SetSpec runs unconditionally: reverting to
	// C-g must resync a listener still armed on the old prefix.
	spec := prefix.Lookup(m.Opts.Config.General.PrefixKey)
	if pk := m.Opts.Config.General.PrefixKey; pk != "" && pk != spec.ConfigKey {
		slog.Warn("reload: unrecognised prefix_key, using default",
			"prefix_key", pk, "using", spec.ConfigKey)
	}
	if spec.ChordToken != "C-g" {
		m.Reg.RebindPrefix("C-g", spec.ChordToken)
	}
	if m.prefix != nil {
		m.prefix.SetSpec(spec)
	}

	m.reloadThemesInternal()

	if m.Opts.RefreshUI != nil {
		m.Opts.RefreshUI()
	}
	m.refreshStatusBar()
	slog.Info("reload: done", "overrides", len(overrides), "had_error", hadError, "warnings", warnings)
	switch {
	case hadError:
		msgbox.Show(&m.App.Desktop.Group, msgbox.Warning,
			"Config reload completed with errors. Open the Log Viewer for full details.", msgbox.OKOnly)
	case warnings > 0:
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
			"Config reloaded with %d warning(s). Open the Log Viewer for details.",
			[]any{warnings}, msgbox.OKOnly)
	default:
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"Config reloaded.", msgbox.OKOnly)
	}
}

// applyReloadedConfig swaps in the freshly-loaded config and forwards the
// settings that live outside m.Opts.Config to their runtime owners: the
// status bar's clock format (copied once at Build time) and window_shadow on
// already-open frames (new windows pick it up in registerWindow). Split out
// of ReloadConfig so it can be exercised without the trailing modal msgboxes —
// pure state application, no UI/modal side effects.
func (m *Mux) applyReloadedConfig(cfg *config.Config) {
	m.Opts.Config = cfg
	// The status bar copied ClockFormat once at Build time — forward the
	// (possibly changed) format or "Reload Config" silently doesn't reload
	// one of the settings it claims to.
	if m.Opts.StatusBar != nil {
		m.Opts.StatusBar.ClockFormat = cfg.Appearance.StatusClock
	}
	// Re-apply window_shadow to already-open windows; new windows pick it up
	// in registerWindow.
	for _, ws := range m.windows {
		if ws == nil || ws.Frame == nil {
			continue
		}
		if cfg.Appearance.WindowShadow {
			ws.Frame.State |= consts.SfShadow
		} else {
			ws.Frame.State &^= consts.SfShadow
		}
	}
}

// ReloadThemes re-runs theme.All against the themes dir and re-applies
// the currently-active theme so palette edits land without a restart.
// Called from ReloadConfig and from the theme editor's save path.
func (m *Mux) ReloadThemes() {
	m.reloadThemesInternal()
	if m.Opts.RefreshUI != nil {
		m.Opts.RefreshUI()
	}
	m.refreshStatusBar()
}

// reloadThemesInternal is the lock-free body shared between
// ReloadConfig (which already does its own UI refresh) and
// ReloadThemes (the standalone path).
func (m *Mux) reloadThemesInternal() {
	m.Opts.Themes = muxtheme.All(m.Opts.Paths.ThemesDir())
	active := m.Opts.Config.Appearance.Theme
	if active == "" {
		return
	}
	if t := muxtheme.Find(m.Opts.Themes, active); t != nil {
		t.Apply()
	}
}
