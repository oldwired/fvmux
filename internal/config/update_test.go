package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandboxPaths returns a Paths tree fully rooted inside dir so tests
// never write into the real $XDG / $HOME dirs.
func sandboxPaths(dir string) Paths {
	return Paths{Root: dir, StateRoot: filepath.Join(dir, "state")}
}

// TestUpdateKeys_PreservesCommentsAndUnknownKeys is the core of finding
// #16: a surgical edit must keep comments, blank lines, unknown keys and
// ordering, only rewriting the assignments named in the updates.
func TestUpdateKeys_PreservesCommentsAndUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := strings.Join([]string{
		"# fvmux config",
		"",
		"[general]",
		`prefix_key = "C-g"  # the prefix`,
		"confirm_kill = true",
		"my_custom_key = 5",
		"",
		"[appearance]",
		`theme = "slate"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateKeys(path,
		KV{Section: "appearance", Key: "theme", Value: "night"},
		KV{Section: "general", Key: "prefix_key", Value: "C-b"},
	); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	if !strings.Contains(got, "# fvmux config") {
		t.Error("top-of-file comment was lost")
	}
	if !strings.Contains(got, "my_custom_key = 5") {
		t.Error("unknown key my_custom_key was lost")
	}
	if !strings.Contains(got, "# the prefix") {
		t.Errorf("inline trailing comment was lost:\n%s", got)
	}
	if !strings.Contains(got, `prefix_key = "C-b"`) {
		t.Errorf("prefix_key was not rewritten:\n%s", got)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Appearance.Theme != "night" {
		t.Errorf("Theme = %q; want night", cfg.Appearance.Theme)
	}
	if cfg.General.PrefixKey != "C-b" {
		t.Errorf("PrefixKey = %q; want C-b", cfg.General.PrefixKey)
	}
	if !cfg.General.ConfirmKill {
		t.Error("ConfirmKill should still be true")
	}
}

// A missing key in an existing section is appended inside that section,
// before the next [table] header — not at the end of the file.
func TestUpdateKeys_AppendsMissingKeyInsideSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := strings.Join([]string{
		"[general]",
		`prefix_key = "C-g"`,
		"",
		"[appearance]",
		`theme = "slate"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateKeys(path,
		KV{Section: "general", Key: "default_profile", Value: "work"},
	); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	keyLine, appearanceHdr := -1, -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "default_profile") {
			keyLine = i
		}
		if strings.TrimSpace(ln) == "[appearance]" {
			appearanceHdr = i
		}
	}
	if keyLine < 0 {
		t.Fatalf("default_profile not written:\n%s", data)
	}
	if appearanceHdr < 0 || keyLine >= appearanceHdr {
		t.Errorf("default_profile (line %d) must land inside [general], before [appearance] (line %d)",
			keyLine, appearanceHdr)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.General.DefaultProfile != "work" {
		t.Errorf("DefaultProfile = %q; want work", cfg.General.DefaultProfile)
	}
}

// A missing section is appended at the end of the file, and Load parses it.
func TestUpdateKeys_AppendsMissingSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "[general]\n" + `prefix_key = "C-g"` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateKeys(path,
		KV{Section: "appearance", Key: "theme", Value: "night"},
	); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "[appearance]") {
		t.Errorf("[appearance] section not appended:\n%s", data)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Appearance.Theme != "night" {
		t.Errorf("Theme = %q; want night", cfg.Appearance.Theme)
	}
}

// A missing file is created and Load parses the result.
func TestUpdateKeys_CreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := UpdateKeys(path,
		KV{Section: "appearance", Key: "theme", Value: "night"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Appearance.Theme != "night" {
		t.Errorf("Theme = %q; want night", cfg.Appearance.Theme)
	}
}

// A value containing a double quote and a backslash must TOML-escape and
// round-trip cleanly back through Load.
func TestUpdateKeys_EscapesQuotesAndBackslashes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	want := `a"b\c`

	if err := UpdateKeys(path,
		KV{Section: "general", Key: "default_profile", Value: want},
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed (bad escaping?): %v", err)
	}
	if cfg.General.DefaultProfile != want {
		t.Errorf("DefaultProfile = %q; want %q", cfg.General.DefaultProfile, want)
	}
}

// Against the real seeded template, UpdateKeys must leave the comments in
// place and still yield a Load-able file with the new theme.
func TestUpdateKeys_OnSeededTemplateKeepsComments(t *testing.T) {
	paths := sandboxPaths(t.TempDir())
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaults(paths); err != nil {
		t.Fatal(err)
	}
	cfgPath := paths.ConfigFile()

	if err := UpdateKeys(cfgPath,
		KV{Section: "appearance", Key: "theme", Value: "night"},
	); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "#") {
		t.Errorf("seeded template comments were flattened:\n%s", data)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Appearance.Theme != "night" {
		t.Errorf("Theme = %q; want night", cfg.Appearance.Theme)
	}
}

// TestUpdateKeys_SectionHeaderTrailingComment pins the header-comment
// regression: "[appearance] # colors" is a valid TOML header, and the
// parser must match the section by the name inside the brackets — not
// treat the comment as part of it, miss the section, and append a
// duplicate [appearance] table that corrupts the file.
func TestUpdateKeys_SectionHeaderTrailingComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := strings.Join([]string{
		"[general]",
		`prefix_key = "C-g"`,
		"",
		"[appearance] # colors",
		`theme = "slate"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateKeys(path,
		KV{Section: "appearance", Key: "theme", Value: "night"}); err != nil {
		t.Fatalf("UpdateKeys: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "[appearance]"); got != 1 {
		t.Fatalf("file has %d [appearance] headers, want 1 — duplicate table corrupts the config:\n%s", got, data)
	}
	if !strings.Contains(string(data), "[appearance] # colors") {
		t.Errorf("header trailing comment lost:\n%s", data)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load after update: %v", err)
	}
	if cfg.Appearance.Theme != "night" {
		t.Errorf("theme = %q; want night", cfg.Appearance.Theme)
	}
}
