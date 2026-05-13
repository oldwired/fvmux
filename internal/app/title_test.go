package app

import "testing"

func TestComposeTitle(t *testing.T) {
	cases := []struct {
		user, shell, fallback, want string
	}{
		{"", "", "shell", "shell"},
		{"", "vim x", "shell", "vim x"},
		{"logs", "", "shell", "[logs]"},
		{"logs", "tail -f", "shell", "[logs] tail -f"},
		{"", "", "", ""},
	}
	for _, c := range cases {
		got := composeTitle(c.user, c.shell, c.fallback)
		if got != c.want {
			t.Errorf("composeTitle(%q,%q,%q) = %q; want %q",
				c.user, c.shell, c.fallback, got, c.want)
		}
	}
}
