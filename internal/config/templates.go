package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/oldwired/fvmux/internal/atomicfile"
)

// SeedDefaults writes any missing config file with a commented template.
// Idempotent: existing files are left alone. Called once at startup
// after EnsureDirs so a brand-new user has working examples to edit.
func SeedDefaults(paths Paths) error {
	files := []struct {
		path, body string
	}{
		{paths.ConfigFile(), templateConfig},
		{paths.ProfilesFile(), templateProfiles},
		{paths.HostsFile(), templateHosts},
		{paths.KeybindingsFile(), templateKeybindings},
		{filepath.Join(paths.ThemesDir(), "example.toml.disabled"), templateThemeExample},
	}
	for _, f := range files {
		if err := writeIfMissing(f.path, f.body); err != nil {
			return err
		}
	}
	return nil
}

// templateThemeExample is dropped into ~/.config/fvmux/themes/ with a
// .disabled extension so the loader skips it. Users rename to *.toml
// to activate. Demonstrates the overlay schema; values are uint16
// fv-go attributes — high byte = bg, low byte = fg.
const templateThemeExample = `# fvmux theme overlay (rename to <name>.toml to activate).
#
# Every field is optional. Anything you omit inherits from the fv-go
# default palette. Values are uint16 attributes packed as fg + bg<<8:
# the easiest way to think about them is "fg colour in the low byte,
# bg colour in the high byte". TOML accepts hex literals (0x...).
#
# 4-bit colour table (terminal-standard):
#   0  black     8  bright black (gray)
#   1  blue      9  bright blue
#   2  green     A  bright green
#   3  cyan      B  bright cyan
#   4  red       C  bright red
#   5  magenta   D  bright magenta
#   6  yellow    E  bright yellow
#   7  gray      F  bright white

name = "example"
tagline = "starter theme — tweak to taste"

[palette]
frame_normal       = 0x0107   # gray on dark blue
frame_active       = 0x010D   # bright magenta on dark blue
window_background  = 0x0107
splitter_bar       = 0x010B   # bright cyan
splitter_handle    = 0x010E   # bright yellow
desktop_background = 0x0008
`

func writeIfMissing(path, body string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists; leave it alone.
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// keybindings.toml can encode prefix-key overrides the user
	// reasonably considers private; the rest are shareable defaults.
	perm := os.FileMode(0o644)
	if filepath.Base(path) == "keybindings.toml" {
		perm = 0o600
	}
	return atomicfile.Write(path, []byte(body), perm)
}

const templateConfig = `# fvmux configuration.
# Edit via File → Edit config.toml inside fvmux, or hand-edit here and
# restart. Most changes take effect on next launch; the prefix key
# can also be changed live via Help → Reset First-Run Wizard.

[general]
# prefix_key: the chord used to enter every binding.
# "C-g" (default), "C-b" (tmux-style), or "C-a" (screen-style).
prefix_key      = "C-g"

# default_profile: which entry in profiles.toml gets used when you
# press <prefix> c. Override per-spawn via <prefix> C.
default_profile = "shell"

# confirm_kill: ask before tearing down windows / panes / fvmux when
# any PTY inside is still running.
confirm_kill    = true

# splash_enabled: show the first-run wizard. Reset any time via
# Help → Reset First-Run Wizard.
splash_enabled  = true

# inherit_cwd_on_split: start a newly split local shell in the focused
# local pane's OSC-7 working directory. SSH panes never donate their
# remote path to a local process. Disable to always use the profile cwd.
inherit_cwd_on_split = true

# connect_split: where <prefix> H (Connect to Host) puts the session.
# The legacy values stay compatible: "vertical" produces a top/bottom
# arrangement (new pane below, like Split Top/Bottom, <prefix> "), while
# "horizontal" produces a left/right arrangement (like Split Left/Right,
# <prefix> %). New configs should read these as arrangements, not divider
# orientation.
# "window" opens a floating window ("window" is also the fallback when
# no pane is focused).
connect_split   = "vertical"

# new_window_command: when non-empty, <prefix> c runs this command
# (via sh -c) instead of default_profile. Empty falls back to profile.
# Example: new_window_command = "ssh prod-1"
new_window_command = ""

[terminal]
# scrollback_lines: cap on per-pane scrollback. 0 keeps fv-go's default.
scrollback_lines = 10000

# shell: override $SHELL for spawned panes. Empty respects $SHELL.
shell            = ""

[appearance]
# theme: one of the built-ins (slate, tokyonight-ish, solarbeach) or
# the name of a user theme dropped into ~/.config/fvmux/themes/.
theme            = "slate"

# bell: how to react to OSC bell. off | flash | notify | both.
bell             = "flash"

# status_clock: Go time-format string for the status-bar clock.
status_clock     = "15:04"

# window_shadow: draw a drop-shadow under floating windows.
window_shadow    = true

# palette_position: where the <prefix> P palette opens. One of:
# "center" | "top-left" | "top-center" | "top-right".
palette_position = "center"

# default_window_width / _height: initial size (in cells) for fresh
# windows. Profiles can override per-profile via window_width /
# window_height. 0 falls back to 80x24.
default_window_width  = 80
default_window_height = 24

[sftp]
# parallel: max concurrent file transfers in each Files window.
parallel = 1
`

const templateProfiles = `# fvmux profiles — named spawn templates.
# Pressing <prefix> C opens a picker over these; <prefix> c uses the
# profile named in config.toml's [general] default_profile.

[[profile]]
name    = "shell"
# command: program to exec. Empty falls back to /bin/sh.
command = ""        # honours $SHELL
cwd     = "~"
# close_on_exit: when true, the pane auto-closes after the process
# exits. Leave false for interactive shells.
close_on_exit = false

# --- examples (uncomment + tweak) -----------------------------------

# [[profile]]
# name    = "prod"
# command = "ssh"
# args    = ["prod-1"]
# title   = "prod"
# close_on_exit = true     # close the pane when the session ends

# [[profile]]
# name = "logs"
# command = "ssh"
# args = ["prod-1", "tail", "-f", "/var/log/app.log"]
# window_width     = 140
# window_height    = 30
# scrollback_lines = 50000     # large buffer for log-tailing.
# close_on_exit    = true

# [[profile]]
# name = "rust"
# command = "cargo"
# args = ["watch", "-x", "test"]
# cwd  = "~/code/myproj"
# env  = { RUST_BACKTRACE = "1" }

# layout: pre-split the profile's window using the session layout DSL
# (each leaf names a profile; see a saved session file for the shape).
# [[profile]]
# name   = "dev"
# layout = 'split-h:0.7{leaf:profile=shell}{leaf:profile=logs}'
`

const templateHosts = `# fvmux additional SSH hosts.
# Merged with ~/.ssh/config — entries here win on alias collisions.
# Surfaced by <prefix> H (Connect to Host) and <prefix> F (SFTP).
#
# [[host]]
# alias = "prod-1"
# user  = "deploy"
# host  = "prod-1.internal"
# port  = 22
# tags  = ["production", "asia"]
# notes = "main API box"
`

const templateKeybindings = `# fvmux key bindings overrides.
#
# Each [[binding]] entry binds <prefix> plus exactly one key to a
# command (by its display name, exactly as shown in the cheatsheet /
# palette). <prefix>, factory C-g, and the currently configured prefix
# are accepted and normalized. Empty command
# removes whatever's currently on that chord.
#
# Reload after editing via Help → Reload Config (no restart needed).
#
# Examples:
#
# [[binding]]
# chord   = "<prefix> u"          # bind prefix+u to "Find Window…"
# command = "Find Window…"       # (C-g u is unused by default — uncommenting
#                                 # an example must never steal a factory chord)
#
# [[binding]]
# chord   = "<prefix> w"          # unbind the default prefix+w
# command = ""
#
# [[binding]]
# chord   = "<prefix> X"          # custom chord for a built-in
# command = "Split Left/Right"
`
