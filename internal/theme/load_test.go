package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	fvtheme "github.com/oldwired/fv-go/pkg/fv/theme"
)

func TestLoadFromTOML_AppliesOverlayAndDefaults(t *testing.T) {
	// Mix originally-curated fields with ones only the reflective overlay
	// reaches (menu box, tree view) to prove the full palette is themeable.
	data := []byte(`
name = "demo"
tagline = "hi"

[palette]
frame_active         = 0x010D
menu_box_selected    = 0x0070
tree_focused         = 0x0020
`)
	th, err := LoadFromTOML(data)
	if err != nil {
		t.Fatalf("LoadFromTOML: %v", err)
	}
	if th.Name != "demo" || th.Tagline != "hi" {
		t.Fatalf("metadata = %q/%q", th.Name, th.Tagline)
	}
	if th.Palette.FrameActive != 0x010D {
		t.Errorf("FrameActive = %#x, want 0x010D", th.Palette.FrameActive)
	}
	if th.Palette.MenuBoxSelected != 0x0070 {
		t.Errorf("MenuBoxSelected = %#x, want 0x0070", th.Palette.MenuBoxSelected)
	}
	if th.Palette.TreeFocused != 0x0020 {
		t.Errorf("TreeFocused = %#x, want 0x0020", th.Palette.TreeFocused)
	}
	// An untouched field keeps the fv-go default.
	if th.Palette.FrameIcons != fvtheme.Default.FrameIcons {
		t.Errorf("FrameIcons = %#x, want default %#x", th.Palette.FrameIcons, fvtheme.Default.FrameIcons)
	}
}

func TestLoadFromTOML_MissingName(t *testing.T) {
	if _, err := LoadFromTOML([]byte(`tagline = "x"`)); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadFromTOML_UnknownKey(t *testing.T) {
	data := []byte(`
name = "bad"
[palette]
frame_normal = 0x0107
not_a_real_field = 1
also_bogus = 2
`)
	_, err := LoadFromTOML(data)
	if err == nil {
		t.Fatal("expected error for unknown palette keys")
	}
	for _, want := range []string{"not_a_real_field", "also_bogus"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing mention of %q", err, want)
		}
	}
}

func TestLoadFromTOML_OutOfRange(t *testing.T) {
	data := []byte(`
name = "oor"
[palette]
frame_normal = 70000
`)
	if _, err := LoadFromTOML(data); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error, got %v", err)
	}
}

func TestToSnakeCase(t *testing.T) {
	cases := map[string]string{
		"FrameNormal":        "frame_normal",
		"MenuBoxSelectedHot": "menu_box_selected_hot",
		"StatusBarHot":       "status_bar_hot",
	}
	for in, want := range cases {
		if got := toSnakeCase(in); got != want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadDir_SortedAndErrorSurfacing(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.toml", `name = "bee"`)
	write("a.toml", `name = "ay"`)
	write("broken.toml", `name = "x"`+"\n[palette]\nbogus = 1\n")
	write("ignored.txt", `name = "nope"`) // non-toml skipped.

	themes, err := LoadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "broken.toml") {
		t.Fatalf("expected broken.toml error, got %v", err)
	}
	// The two good themes still load, in alphabetical filename order.
	if len(themes) != 2 || themes[0].Name != "ay" || themes[1].Name != "bee" {
		t.Fatalf("themes = %+v, want [ay bee]", themes)
	}
}

func TestLoadDir_MissingDir(t *testing.T) {
	themes, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil || themes != nil {
		t.Fatalf("missing dir = (%v, %v), want (nil, nil)", themes, err)
	}
}

func TestAll_DiskOverridesBuiltinByName(t *testing.T) {
	dir := t.TempDir()
	// Override the builtin "slate" with a disk theme of the same name.
	if err := os.WriteFile(filepath.Join(dir, "mine.toml"),
		[]byte(`name = "slate"`+"\ntagline = \"mine\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all := All(dir)
	var slates int
	for _, th := range all {
		if th.Name == "slate" {
			slates++
			if th.Tagline != "mine" {
				t.Errorf("slate tagline = %q, want disk override %q", th.Tagline, "mine")
			}
		}
	}
	if slates != 1 {
		t.Fatalf("found %d themes named slate, want exactly 1 (override, not duplicate)", slates)
	}
}
