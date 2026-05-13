// Package profile owns spawn-template definitions: a profile is the
// read-only "what to run" recipe (command, args, env, cwd, title);
// instantiating one produces a live session.Pane.
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	CWD     string            `toml:"cwd"`    // "~" / "$VARS" expanded at Instantiate time
	Title   string            `toml:"title"`  // initial pane title; OSC overrides
	Layout  string            `toml:"layout"` // optional pre-split spec; ignored in step 4

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
// that runs $SHELL (or /bin/sh).
func Defaults() []*Profile {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return []*Profile{{
		Name:    "shell",
		Command: shell,
		CWD:     "~",
	}}
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
func Instantiate(p *Profile, bounds geom.Rect, defaultScrollback int) (*session.Pane, error) {
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
	if len(p.Env) > 0 {
		env := os.Environ()
		for k, v := range p.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, expandPath(v)))
		}
		t.SetEnv(env)
	}
	cmd := p.Command
	if cmd == "" {
		cmd = "/bin/sh"
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
		CloseOnExit: p.CloseOnExit,
	}, nil
}

// expandPath expands a leading "~" to $HOME and $VAR / ${VAR} forms.
func expandPath(s string) string {
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "~") {
		home, _ := os.UserHomeDir()
		s = filepath.Join(home, strings.TrimPrefix(s, "~"))
	}
	return os.ExpandEnv(s)
}
