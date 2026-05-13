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
