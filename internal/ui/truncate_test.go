package ui

import (
	"testing"
	"unicode/utf8"

	fvutf8 "github.com/oldwired/fv-go/pkg/fv/utf8"
)

func TestTruncRight_ASCII(t *testing.T) {
	cases := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"}, // shorter than max: unchanged.
		{"hello", 5, "hello"},  // exactly max: unchanged.
		{"hello world", 5, "hell…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"hello", -3, ""},
	}
	for _, c := range cases {
		if got := TruncRight(c.s, c.max); got != c.want {
			t.Errorf("TruncRight(%q, %d) = %q; want %q", c.s, c.max, got, c.want)
		}
	}
}

// A 🏠-prefixed window title (the whimsy home glyph is 2 runes wide in
// UTF-8) must never be sliced mid-rune: the old byte-based truncate cut
// s[:max-1] and produced invalid UTF-8 here.
func TestTruncRight_MultiByteNeverSplitsRune(t *testing.T) {
	cases := []string{
		"🏠home-server-very-long-name",
		"Übergröße-fenster",
		"日本語のタイトルがとても長い",
		"café-résumé-naïve-window",
	}
	for _, s := range cases {
		for max := 1; max <= 12; max++ {
			got := TruncRight(s, max)
			if !utf8.ValidString(got) {
				t.Fatalf("TruncRight(%q, %d) = %q is not valid UTF-8", s, max, got)
			}
			if n := utf8.RuneCountInString(got); n > max {
				t.Fatalf("TruncRight(%q, %d) = %q has %d runes > max", s, max, got, n)
			}
		}
	}
}

// Exact case from the statusbar finding: window title truncated to 12.
func TestTruncRight_StatusbarWindowTitle(t *testing.T) {
	got := TruncRight("🏠production-database", 12)
	if !utf8.ValidString(got) {
		t.Fatalf("got invalid UTF-8: %q", got)
	}
	// Cell budget, not rune count: 🏠 occupies 2 cells, so 12 cells hold
	// the glyph + 9 ASCII runes + the 1-cell ellipsis.
	if w := fvutf8.StringDisplayWidth(got); w != 12 {
		t.Fatalf("got %d cells; want 12: %q", w, got)
	}
	r := []rune(got)
	if r[len(r)-1] != '…' {
		t.Fatalf("expected trailing ellipsis, got %q", got)
	}
}

// TestTruncRight_WideRunesRespectCellBudget pins the review scenario:
// twelve CJK characters are 24 cells — a rune-count truncation would
// pass them through a 12-column slot and overflow adjacent UI regions.
func TestTruncRight_WideRunesRespectCellBudget(t *testing.T) {
	s := "十二个汉字宽度测试标签啊" // 12 CJK runes = 24 cells
	got := TruncRight(s, 12)
	if got == s {
		t.Fatalf("12-rune/24-cell string passed a 12-cell budget unchanged")
	}
	if w := fvutf8.StringDisplayWidth(got); w > 12 {
		t.Fatalf("got %d cells; want <= 12: %q", w, got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("got invalid UTF-8: %q", got)
	}
	r := []rune(got)
	if r[len(r)-1] != '…' {
		t.Fatalf("expected trailing ellipsis, got %q", got)
	}
}

// TestTruncLeftPath_WideRunesRespectCellBudget mirrors the header case:
// the kept TAIL must fit the cell budget even when it is all wide runes.
func TestTruncLeftPath_WideRunesRespectCellBudget(t *testing.T) {
	got := TruncLeftPath("/srv/資料/重要文件/最終版", 10)
	if w := fvutf8.StringDisplayWidth(got); w > 10 {
		t.Fatalf("got %d cells; want <= 10: %q", w, got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("got invalid UTF-8: %q", got)
	}
	if []rune(got)[0] != '…' {
		t.Fatalf("expected leading ellipsis, got %q", got)
	}
}

func TestTruncLeftPath_ASCII(t *testing.T) {
	cases := []struct {
		path string
		max  int
		want string
	}{
		{"/home/user", 20, "/home/user"}, // shorter than max: unchanged.
		{"/home/user", 10, "/home/user"}, // exactly max: unchanged.
		{"/home/user/projects/deep", 10, "…ects/deep"},
		{"/a/b/c/d", 3, "c/d"}, // max <= 3: no room for ellipsis, keep tail.
		{"/a/b/c/d", 0, ""},
	}
	for _, c := range cases {
		if got := TruncLeftPath(c.path, c.max); got != c.want {
			t.Errorf("TruncLeftPath(%q, %d) = %q; want %q", c.path, c.max, got, c.want)
		}
	}
}

// Exact case from the SFTP header finding: a non-ASCII remote path
// truncated from the left must stay valid UTF-8 and keep its tail.
func TestTruncLeftPath_MultiByteNeverSplitsRune(t *testing.T) {
	path := "/srv/файлы/项目/café/naïve"
	for max := 1; max <= 20; max++ {
		got := TruncLeftPath(path, max)
		if !utf8.ValidString(got) {
			t.Fatalf("TruncLeftPath(%q, %d) = %q is not valid UTF-8", path, max, got)
		}
		if n := utf8.RuneCountInString(got); n > max {
			t.Fatalf("TruncLeftPath(%q, %d) = %q has %d runes > max", path, max, got, n)
		}
	}
}
