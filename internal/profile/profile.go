// Package profile owns spawn-template definitions: a profile is the
// read-only "what to run" recipe (command, args, env, cwd, title);
// instantiating one produces a live session.Pane.
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/session"
)

// Profile is one spawn template, sourced from ~/.config/fvmux/profiles.toml.
type Profile struct {
	Name    string            `toml:"name"`
	Command string            `toml:"command"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
	CWD     string            `toml:"cwd"`   // "~" / "$VARS" expanded at Instantiate time
	Title   string            `toml:"title"` // initial pane title; OSC overrides

	// Layout, when non-empty, pre-splits the profile's window using the
	// same DSL session snapshots use (see internal/layout/serde.go);
	// each leaf names a profile. Invalid DSL degrades to a single pane
	// with a warning.
	Layout string `toml:"layout"`

	// CloseOnExit, when true, automatically closes the pane after the
	// child process exits. Default false — the pane stays visible with
	// its scrollback so users can read the final output.
	CloseOnExit bool `toml:"close_on_exit"`

	// WindowWidth / WindowHeight set the initial size (in cells) of a
	// freshly-spawned window using this profile. 0 falls back to the
	// global default from [appearance] default_window_width / _height,
	// which in turn falls back to 80×24.
	WindowWidth  int `toml:"window_width"`
	WindowHeight int `toml:"window_height"`

	// ScrollbackLines overrides the global [terminal] scrollback_lines
	// for panes spawned from this profile. 0 falls back to that
	// setting; if both are 0, fv-go's built-in default applies.
	ScrollbackLines int `toml:"scrollback_lines"`

	// SSHAlias marks a synthesized interactive-ssh profile (never set
	// from profiles.toml): the profile was built via an SSH pool
	// Acquire for this alias, and the pane spawned from it owns that
	// refcount — app.stopPane releases it when the pane's terminal
	// stops, and spawn-failure paths release it directly.
	SSHAlias string `toml:"-"`
}

type profilesFile struct {
	Profiles []*Profile `toml:"profile"`
}

// Load parses path into a profile list. A missing file yields the
// built-in defaults; a malformed file returns the parse error along
// with the defaults so callers can still operate.
func Load(path string) ([]*Profile, error) {
	defaults := Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	var f profilesFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return defaults, err
	}
	if len(f.Profiles) == 0 {
		return defaults, nil
	}
	return f.Profiles, nil
}

// Defaults returns the baked-in profile list: just a "shell" profile
// that runs $SHELL (or the platform fallback).
func Defaults() []*Profile {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = FallbackShell()
	}
	return []*Profile{{
		Name:    "shell",
		Command: shell,
		CWD:     "~",
	}}
}

// FallbackShell is the last resort of the shell-resolution chain
// (profile → config → $SHELL → this). /bin/sh does not exist on
// Windows — a supported target — so honour %COMSPEC% there and fall
// back to cmd.exe.
func FallbackShell() string {
	if runtime.GOOS == "windows" {
		if cs := os.Getenv("COMSPEC"); cs != "" {
			return cs
		}
		return "cmd.exe"
	}
	return "/bin/sh"
}

// ShellCommand returns the system-shell invocation that runs cmdline as
// a single shell command: "/bin/sh -c cmdline", or "%COMSPEC% /c
// cmdline" on Windows. Used for config values documented as being
// interpreted by the shell (new_window_command).
func ShellCommand(cmdline string) (name string, args []string) {
	if runtime.GOOS == "windows" {
		return FallbackShell(), []string{"/c", cmdline}
	}
	return "/bin/sh", []string{"-c", cmdline}
}

// Find returns the profile with the given name, or nil.
func Find(profiles []*Profile, name string) *Profile {
	for _, p := range profiles {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// Instantiate constructs a live session.Pane from p, starting the
// terminal at the given bounds. defaultScrollback is the global fallback
// (from [terminal] scrollback_lines) used when p.ScrollbackLines == 0.
// defaultShell is the configured [terminal] shell — used when the
// profile leaves Command empty (chain: profile → config → $SHELL →
// /bin/sh).
func Instantiate(p *Profile, bounds geom.Rect, defaultScrollback int, defaultShell string) (*session.Pane, error) {
	if p == nil {
		return nil, errors.New("profile.Instantiate: nil profile")
	}
	t := terminal.New(bounds)
	switch {
	case p.ScrollbackLines > 0:
		t.ScrollbackLines = p.ScrollbackLines
	case defaultScrollback > 0:
		t.ScrollbackLines = defaultScrollback
	}
	if p.CWD != "" {
		t.SetWorkingDir(expandPath(p.CWD))
	}
	if env := spawnEnv(p.Env); env != nil {
		t.SetEnv(env)
	}
	cmd := p.Command
	if cmd == "" {
		cmd = defaultShell
	}
	if cmd == "" {
		cmd = os.Getenv("SHELL")
	}
	if cmd == "" {
		cmd = FallbackShell()
	}
	if err := t.Start(cmd, p.Args, nil); err != nil {
		return nil, err
	}
	title := p.Title
	if title == "" {
		title = p.Name
	}
	return &session.Pane{
		ID:          session.NewPaneID(),
		Term:        t,
		Title:       title,
		Profile:     p.Name,
		SSHAlias:    p.SSHAlias,
		CloseOnExit: p.CloseOnExit,
	}, nil
}

// spawnEnv builds the child environment for a profile that defines env
// vars; nil when it defines none (fv-go then applies its own TERM
// patch). fv-go's Terminal.Start patches TERM=xterm-256color only when
// it resolves a nil env; supplying any profile var suppresses that, so
// the same patch is re-applied here — otherwise the child inherits the
// outer TERM (tmux-256color under tmux) while talking to fv-go's xterm
// emulator, garbling full-screen apps. The patch is appended before the
// profile's own vars so an explicit TERM in profiles.toml still wins
// (exec keeps the last duplicate — the same contract fv-go's own patch
// relies on).
func spawnEnv(profileEnv map[string]string) []string {
	if len(profileEnv) == 0 {
		return nil
	}
	env := append(os.Environ(), "TERM=xterm-256color")
	for k, v := range profileEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, expandPath(v)))
	}
	return env
}

// expandPath expands a leading "~" to $HOME and $VAR / ${VAR} forms.
// Only bare "~" and "~/…" refer to the current user's home; the
// "~user/…" form is left untouched (naively gluing it onto $HOME would
// silently point the pane at $HOME/user/… — the classic pitfall) so it
// surfaces as an honest no-such-directory error instead.
func expandPath(s string) string {
	if s == "" {
		return s
	}
	if s == "~" || strings.HasPrefix(s, "~/") {
		home, _ := os.UserHomeDir()
		s = filepath.Join(home, strings.TrimPrefix(s, "~"))
	}
	return os.ExpandEnv(s)
}
