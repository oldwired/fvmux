package headless

import (
	"path/filepath"
	"testing"

	"github.com/oldwired/fvmux/internal/theme"
)

const themeFixture = `
name = "indigo"
tagline = "for late-night sessions"

[palette]
frame_normal       = 0x0107
frame_active       = 0x010D
window_background  = 0x0107
splitter_bar       = 0x010B
splitter_handle    = 0x010E
desktop_background = 0x0008
`

// TestThemeLoadFromTOML parses an overlay and confirms the resulting
// Theme carries the user-supplied name + tagline + overlay fields.
func TestThemeLoadFromTOML(t *testing.T) {
	th, err := theme.LoadFromTOML([]byte(themeFixture))
	if err != nil {
		t.Fatalf("LoadFromTOML: %v", err)
	}
	if th.Name != "indigo" {
		t.Errorf("Name = %q, want indigo", th.Name)
	}
	if th.Tagline != "for late-night sessions" {
		t.Errorf("Tagline = %q, want %q", th.Tagline, "for late-night sessions")
	}
	if th.Palette == nil {
		t.Fatal("Palette is nil")
	}
	if th.Palette.FrameActive != 0x010D {
		t.Errorf("FrameActive = %#x, want 0x010D", th.Palette.FrameActive)
	}
}

// TestThemeLoadDirMissing returns nil/nil when the directory doesn't
// exist — we don't want fvmux to refuse to start because users haven't
// created ~/.config/fvmux/themes/.
func TestThemeLoadDirMissing(t *testing.T) {
	out, err := theme.LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Errorf("LoadDir missing: err = %v, want nil", err)
	}
	if len(out) != 0 {
		t.Errorf("LoadDir missing: got %d themes, want 0", len(out))
	}
}

// TestThemeAllIncludesBuiltins ensures All() returns at least the
// three builtins when the disk dir is empty.
func TestThemeAllIncludesBuiltins(t *testing.T) {
	out := theme.All(t.TempDir())
	if len(out) < 3 {
		t.Errorf("All() = %d themes, want ≥3 (builtins)", len(out))
	}
}
