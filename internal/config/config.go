package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

// Config is the parsed contents of ~/.config/fvmux/config.toml.
type Config struct {
	General    General    `toml:"general"`
	Terminal   Terminal   `toml:"terminal"`
	Appearance Appearance `toml:"appearance"`
	SFTP       SFTP       `toml:"sftp"`
}

// General captures the prefix-key, default-profile and similar top-level
// preferences.
type General struct {
	PrefixKey      string `toml:"prefix_key"`      // "C-g", "C-b", …
	DefaultProfile string `toml:"default_profile"` // "shell"
	ConfirmKill    bool   `toml:"confirm_kill"`
	SplashEnabled  bool   `toml:"splash_enabled"`
	ConnectSplit   string `toml:"connect_split"` // "vertical" or "horizontal"

	// NewWindowCommand, when non-empty, overrides what Ctrl-G c runs.
	// Interpreted as a shell command (passed through `sh -c`) so pipes,
	// args, and quoting all work — e.g.
	//   new_window_command = "ssh prod-1"
	//   new_window_command = "tmux new-session -A -s work"
	// Empty falls back to DefaultProfile.
	NewWindowCommand string `toml:"new_window_command"`
}

// Terminal captures per-pane defaults.
type Terminal struct {
	ScrollbackLines int    `toml:"scrollback_lines"`
	Shell           string `toml:"shell"`
}

// Appearance captures theme + bell + status-bar styling.
type Appearance struct {
	Theme        string `toml:"theme"`
	Bell         string `toml:"bell"` // off | flash | notify | both
	StatusClock  string `toml:"status_clock"`
	WindowShadow bool   `toml:"window_shadow"`

	// PalettePosition controls where Ctrl-G P opens. One of:
	// "center" (default), "top-left", "top-center", "top-right".
	PalettePosition string `toml:"palette_position"`

	// DefaultWindowWidth / Height set the initial size of new windows
	// when the spawning profile doesn't override. 0 falls back to 80×24.
	DefaultWindowWidth  int `toml:"default_window_width"`
	DefaultWindowHeight int `toml:"default_window_height"`
}

// SFTP captures file-browser tunables.
type SFTP struct {
	Parallel int `toml:"parallel"`
}

// Defaults returns the baked-in config: what fvmux runs as if no
// config.toml exists.
func Defaults() *Config {
	return &Config{
		General: General{
			PrefixKey:      "C-g",
			DefaultProfile: "shell",
			ConfirmKill:    true,
			SplashEnabled:  true,
			ConnectSplit:   "vertical",
		},
		Terminal: Terminal{
			ScrollbackLines: 10000,
			Shell:           "", // empty ⇒ honour $SHELL
		},
		Appearance: Appearance{
			Theme:           "slate",
			Bell:            "flash",
			StatusClock:     "15:04",
			WindowShadow:    true,
			PalettePosition: "center",
		},
		SFTP: SFTP{Parallel: 1},
	}
}

// Load reads cfg from path, merged onto Defaults() so missing keys
// pick up sensible values. A non-existent file is not an error — the
// caller gets Defaults() unmodified.
func Load(path string) (*Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes cfg back to path as TOML. Used by the first-run wizard
// to persist the prefix-key choice; other settings are still
// hand-edited.
func Save(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
