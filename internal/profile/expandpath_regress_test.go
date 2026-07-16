package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExpandPath_TildeForms pins review finding #27: only bare "~" and
// "~/…" expand to the current user's home. The "~user/…" form is left
// UNTOUCHED — the earlier naive expansion glued it under $HOME, silently
// pointing the pane at $HOME/user/… instead of surfacing an honest
// no-such-directory error. $VAR / ${VAR} expansion still happens either way.
func TestExpandPath_TildeForms(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skipf("no home dir available: %v", err)
	}

	// Bare "~" → home.
	if got := expandPath("~"); got != home {
		t.Errorf("expandPath(%q) = %q, want %q", "~", got, home)
	}
	// "~/x" → home/x.
	if got, want := expandPath("~/projects"), filepath.Join(home, "projects"); got != want {
		t.Errorf("expandPath(%q) = %q, want %q", "~/projects", got, want)
	}
	// "~user/x" → untouched (NOT glued under $HOME).
	if got := expandPath("~otheruser/projects"); got != "~otheruser/projects" {
		t.Errorf("expandPath(%q) = %q, want it left untouched", "~otheruser/projects", got)
	}

	// $VAR still expands.
	t.Setenv("FVMUX_EXPAND_TEST", "/srv/data")
	if got := expandPath("$FVMUX_EXPAND_TEST/sub"); got != "/srv/data/sub" {
		t.Errorf("expandPath($VAR/sub) = %q, want /srv/data/sub", got)
	}
	// A "~user" prefix stays verbatim while an embedded $VAR still expands.
	t.Setenv("FVMUX_EXPAND_SUB", "deep")
	if got := expandPath("~bob/$FVMUX_EXPAND_SUB"); got != "~bob/deep" {
		t.Errorf("expandPath(%q) = %q, want ~bob/deep", "~bob/$FVMUX_EXPAND_SUB", got)
	}
}
