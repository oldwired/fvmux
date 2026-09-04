package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsRoundTrip(t *testing.T) {
	c := Defaults()
	if c.General.PrefixKey != "C-g" {
		t.Errorf("default prefix = %q", c.General.PrefixKey)
	}
	if c.Terminal.ScrollbackLines != 10000 {
		t.Errorf("default scrollback = %d", c.Terminal.ScrollbackLines)
	}
	if c.Appearance.Theme != "slate" {
		t.Errorf("default theme = %q", c.Appearance.Theme)
	}
	if !c.General.InheritSplitCWD {
		t.Error("inherit_cwd_on_split should default true")
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	c, err := Load("/nonexistent/fvmux-config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if c.Appearance.Theme != "slate" {
		t.Error("expected defaults when file missing")
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[general]
prefix_key = "C-b"
inherit_cwd_on_split = false

[appearance]
theme = "tokyonight-ish"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.General.PrefixKey != "C-b" {
		t.Errorf("got prefix %q", c.General.PrefixKey)
	}
	if c.Appearance.Theme != "tokyonight-ish" {
		t.Errorf("got theme %q", c.Appearance.Theme)
	}
	if c.General.InheritSplitCWD {
		t.Error("explicit inherit_cwd_on_split=false was not loaded")
	}
	// Untouched key should still hold the default.
	if c.Terminal.ScrollbackLines != 10000 {
		t.Errorf("scrollback should still default; got %d", c.Terminal.ScrollbackLines)
	}
}

func TestPathsWithRoot(t *testing.T) {
	root := "/tmp/x"
	p := Default().WithRoot(root)
	if p.Root != root {
		t.Errorf("Root = %q", p.Root)
	}
	if want := filepath.Join(root, "sessions", "work.toml"); p.SessionFile("work") != want {
		t.Errorf("SessionFile = %q", p.SessionFile("work"))
	}
}
