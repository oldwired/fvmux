package cheatsheet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oldwired/fvmux/internal/commands"
)

// TestBakedCheatsheetInSync guards against registry drift in the committed
// assets/cheatsheet.md: regenerating from commands.Defaults() must match the
// checked-in bytes exactly. GenerateBaked is deterministic (no dated
// tagline), so a mismatch means the asset is stale.
func TestBakedCheatsheetInSync(t *testing.T) {
	got := GenerateBaked(commands.Defaults())
	for _, path := range []string{
		filepath.Join("..", "..", "assets", "cheatsheet.md"),
		filepath.Join("..", "..", "KEYBINDINGS.md"),
	} {
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if got != string(want) {
			t.Errorf("%s is stale — run `go generate ./internal/cheatsheet`", path)
		}
	}
}

func TestGenerateUsesActivePrefixInContextualHelp(t *testing.T) {
	reg := commands.Defaults()
	reg.RebindPrefix("C-g", "C-b")
	got := GenerateBaked(reg)
	if !strings.Contains(got, "**C-b m**") || strings.Contains(got, "**C-g m**") {
		t.Fatalf("contextual help did not render the active prefix")
	}
}

// TestGenerateBakedDeterministic confirms the baked generator omits the
// date-dependent whimsy footer, so repeated runs (and CI on any day) agree.
func TestGenerateBakedDeterministic(t *testing.T) {
	reg := commands.Defaults()
	if a, b := GenerateBaked(reg), GenerateBaked(reg); a != b {
		t.Fatal("GenerateBaked is not reproducible")
	}
	if got := GenerateBaked(reg); len(got) == 0 {
		t.Fatal("GenerateBaked produced empty output")
	} else if got[len(got)-1] == '\n' && wantsTaglineFooter(got) {
		t.Error("GenerateBaked leaked a whimsy tagline footer into the baked asset")
	}
}

func wantsTaglineFooter(s string) bool {
	// The footer is "---\n\n*…*\n"; a baked doc must not carry it.
	return len(s) > 4 && s[len(s)-2] == '*'
}
