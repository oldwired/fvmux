package app

import (
	"log/slog"
	"strings"

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

	if cfg, err := config.Load(paths.ConfigFile()); err == nil {
		m.Opts.Config = cfg
	} else {
		slog.Warn("reload: config.toml", "err", err)
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Warning,
			"Couldn't reload config.toml:\n%s",
			[]any{err.Error()}, msgbox.OKOnly)
	}

	if profiles, err := profile.Load(paths.ProfilesFile()); err == nil {
		m.Opts.Profiles = profiles
	} else {
		slog.Warn("reload: profiles.toml", "err", err)
	}

	overrides, rejected, err := config.LoadKeybindings(paths.KeybindingsFile())
	if err != nil {
		slog.Warn("reload: keybindings.toml", "err", err)
	}
	if len(rejected) > 0 {
		slog.Warn("reload: keybindings.toml has undispatchable chords", "rejected", rejected)
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Warning,
			"keybindings.toml: %d binding(s) use chords the prefix dispatcher can't emit and were skipped:\n%s",
			[]any{len(rejected), strings.Join(rejected, "\n")}, msgbox.OKOnly)
	}

	// Factory baseline → user overrides → prefix-key. Order matters:
	// overrides are authored in the default "C-g" space, so they must be
	// applied before the prefix rebind sweeps every chord onto the
	// configured prefix (the rebind then carries an overridden "C-g w"
	// to "C-b w" too). Resetting first ensures a removed user override
	// actually goes away.
	m.Reg.ResetChords()
	if len(overrides) > 0 {
		m.Reg.ApplyOverrides(overrides)
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
	slog.Info("reload: done", "overrides", len(overrides))
	msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
		"Config reloaded.", msgbox.OKOnly)
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
