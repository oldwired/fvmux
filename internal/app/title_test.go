package app

import "testing"

func TestComposeTitle(t *testing.T) {
	cases := []struct {
		user, shell, fallback, pane, want string
	}{
		{"", "", "shell", "", "shell"},
		{"", "vim x", "shell", "", "vim x"},
		{"logs", "", "shell", "", "[logs]"},
		{"logs", "tail -f", "shell", "", "[logs] tail -f"},
		{"", "", "", "", ""},

		// Pane title equals shell title → suppressed (already shown).
		{"", "vim x", "shell", "vim x", "vim x"},
		// Pane title distinct → appended after `·`.
		{"", "vim x", "shell", "build", "vim x · build"},
		// Pane title appears in user-bracketed base — also suppressed.
		{"edit", "", "shell", "edit", "[edit]"},
		// Pane title combined with user + shell.
		{"edit", "vim x", "shell", "build", "[edit] vim x · build"},
		// Pane title with no shell, no user → falls into fallback.
		{"", "", "shell", "build", "shell · build"},
	}
	for _, c := range cases {
		got := composeTitle(c.user, c.shell, c.fallback, c.pane)
		if got != c.want {
			t.Errorf("composeTitle(%q,%q,%q,%q) = %q; want %q",
				c.user, c.shell, c.fallback, c.pane, got, c.want)
		}
	}
}
