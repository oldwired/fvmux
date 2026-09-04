package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/prefix"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/sshmgr"
	muxtheme "github.com/oldwired/fvmux/internal/theme"
)

type configDiagnostic struct {
	severity string
	file     string
	reason   string
}

// checkConfig validates every user-authored configuration surface without
// initializing a terminal, creating directories, or rewriting corrupt files.
func checkConfig(paths config.Paths, out io.Writer) error {
	var diagnostics []configDiagnostic
	addError := func(file string, err error) {
		if err != nil {
			diagnostics = append(diagnostics, configDiagnostic{"error", file, err.Error()})
		}
	}
	addSemanticError := func(file, reason string) {
		diagnostics = append(diagnostics, configDiagnostic{"error", file, reason})
	}

	cfg, err := config.Load(paths.ConfigFile())
	if err != nil {
		addError("config.toml", err)
		cfg = config.Defaults()
	}
	spec := prefix.Lookup(cfg.General.PrefixKey)
	if cfg.General.PrefixKey != "" && cfg.General.PrefixKey != spec.ConfigKey {
		addSemanticError("config.toml", fmt.Sprintf("unknown general.prefix_key %q (use C-g, C-b, or C-a)", cfg.General.PrefixKey))
	}
	if !oneOf(cfg.General.ConnectSplit, "vertical", "horizontal", "window") {
		addSemanticError("config.toml", fmt.Sprintf("general.connect_split %q is not vertical, horizontal, or window", cfg.General.ConnectSplit))
	}
	if !oneOf(cfg.Appearance.Bell, "off", "flash", "notify", "both") {
		addSemanticError("config.toml", fmt.Sprintf("appearance.bell %q is not off, flash, notify, or both", cfg.Appearance.Bell))
	}
	if !oneOf(cfg.Appearance.PalettePosition, "center", "top-left", "top-center", "top-right") {
		addSemanticError("config.toml", fmt.Sprintf("appearance.palette_position %q is not a supported position", cfg.Appearance.PalettePosition))
	}
	if cfg.Terminal.ScrollbackLines < 0 || cfg.Appearance.DefaultWindowWidth < 0 ||
		cfg.Appearance.DefaultWindowHeight < 0 || cfg.SFTP.Parallel < 0 {
		addSemanticError("config.toml", "numeric sizes, scrollback_lines, and sftp.parallel must not be negative")
	}

	profiles, err := profile.Load(paths.ProfilesFile())
	addError("profiles.toml", err)
	seenProfiles := map[string]bool{}
	for i, p := range profiles {
		if p == nil || strings.TrimSpace(p.Name) == "" {
			addSemanticError("profiles.toml", fmt.Sprintf("profile %d has no name", i+1))
			continue
		}
		if seenProfiles[p.Name] {
			addSemanticError("profiles.toml", fmt.Sprintf("duplicate profile name %q", p.Name))
		}
		seenProfiles[p.Name] = true
	}
	if cfg.General.DefaultProfile != "" && !seenProfiles[cfg.General.DefaultProfile] {
		addSemanticError("config.toml", fmt.Sprintf("general.default_profile %q does not exist in profiles.toml", cfg.General.DefaultProfile))
	}

	if hostsErr := sshmgr.ValidateHostsFile(paths.HostsFile()); hostsErr != nil {
		addError("hosts.toml", hostsErr)
	} else {
		_, err = sshmgr.Load(paths.HostsFile())
		addError("hosts.toml / ssh_config", err)
	}
	_, err = muxtheme.LoadDir(paths.ThemesDir())
	addError("themes", err)

	reg := commands.Defaults()
	overrides, bindingDiagnostics, err := config.LoadKeybindings(paths.KeybindingsFile(), spec.ChordToken)
	addError("keybindings.toml", err)
	if err == nil {
		bindingDiagnostics = append(bindingDiagnostics, reg.ApplyOverrides(overrides)...)
	}
	for _, diagnostic := range bindingDiagnostics {
		diagnostics = append(diagnostics, configDiagnostic{
			severity: diagnostic.Severity,
			file:     "keybindings.toml",
			reason:   diagnostic.String(),
		})
	}

	errorsFound, warningsFound := 0, 0
	for _, diagnostic := range diagnostics {
		if diagnostic.severity == "error" {
			errorsFound++
		} else {
			warningsFound++
		}
		_, _ = fmt.Fprintf(out, "%s %s: %s\n", strings.ToUpper(diagnostic.severity), diagnostic.file, diagnostic.reason)
	}
	if errorsFound > 0 {
		return fmt.Errorf("configuration has %d error(s) and %d warning(s)", errorsFound, warningsFound)
	}
	if warningsFound > 0 {
		_, _ = fmt.Fprintf(out, "configuration valid with %d warning(s)\n", warningsFound)
		return nil
	}
	_, _ = fmt.Fprintln(out, "configuration valid")
	return nil
}

func oneOf(value string, allowed ...string) bool {
	if value == "" {
		return true
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
