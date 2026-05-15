package sftp

import "testing"

func TestSniffExtensions(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		want MediaKind
	}{
		{"README.md", []byte("# hi"), KindMarkdown},
		{"image.png", []byte{0x89, 0x50, 0x4E, 0x47}, KindImage},
		{"code.go", []byte("package main"), KindText},
		{"config.toml", []byte("[a]"), KindText},
	}
	for _, c := range cases {
		got := Sniff(c.name, c.buf)
		if got != c.want {
			t.Errorf("Sniff(%q) = %v want %v", c.name, got, c.want)
		}
	}
}

func TestSniffNullByteIsBinary(t *testing.T) {
	buf := []byte("hello\x00world more text follows")
	if got := Sniff("unknown.bin", buf); got != KindBinary {
		t.Errorf("null byte should yield KindBinary; got %v", got)
	}
}

func TestSniffELF(t *testing.T) {
	buf := []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}
	if got := Sniff("a.out", buf); got != KindBinary {
		t.Errorf("ELF magic should yield KindBinary; got %v", got)
	}
}

func TestSniffPrintableText(t *testing.T) {
	buf := []byte("This is a perfectly normal text file with sentences.\n")
	if got := Sniff("unknown.x", buf); got != KindText {
		t.Errorf("printable ASCII should yield KindText; got %v", got)
	}
}

func TestSniffEmpty(t *testing.T) {
	if got := Sniff("x", nil); got != KindText {
		t.Errorf("empty buffer should default to KindText; got %v", got)
	}
}

func TestSniffUTF8NonASCII(t *testing.T) {
	cases := []struct {
		label string
		buf   []byte
	}{
		{"russian", []byte("Привет, мир — это обычный текстовый файл.\n")},
		{"japanese", []byte("こんにちは、これは普通のテキストファイルです。\n")},
		{"arabic", []byte("مرحبا بالعالم - هذا ملف نصي عادي.\n")},
		{"emoji", []byte("Status: 🚀 ✅ all good\n")},
		{"accented_latin", []byte("Café résumé naïveté\n")},
	}
	for _, c := range cases {
		got := Sniff("unknown.x", c.buf)
		if got != KindText {
			t.Errorf("%s: UTF-8 text should be KindText, got %v", c.label, got)
		}
	}
}

func TestSniffTruncatedUTF8DoesNotPanic(t *testing.T) {
	// Russian "Привет" — 12 bytes, truncate to 11 so the last rune is
	// incomplete. utf8.Valid returns false; we fall through to the
	// printable ratio (which classifies as binary since high-bit
	// bytes aren't "printable"). Important: must not panic.
	full := []byte("Привет, мир")
	truncated := full[:len(full)-1]
	got := Sniff("unknown.x", truncated)
	// Either classification is acceptable here; the test exists to
	// guard against panics in the boundary case.
	_ = got
}
