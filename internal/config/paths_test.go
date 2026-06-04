package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestControlSocket_HashedNameIsSafeAndBounded(t *testing.T) {
	p := Paths{Root: "/cfg", StateRoot: "/state"}

	// An alias with path separators / traversal must not escape the cm dir.
	got := p.ControlSocket("../../etc/evil")
	if dir := filepath.Dir(got); dir != p.ControlSocketDir() {
		t.Fatalf("socket escaped cm dir: %q (dir %q)", got, dir)
	}
	base := filepath.Base(got)
	if strings.ContainsAny(base, "/.\\") && !strings.HasSuffix(base, ".sock") {
		t.Fatalf("socket basename has unsafe chars: %q", base)
	}

	// Deterministic: same alias → same socket (computed independently).
	first := p.ControlSocket("prod")
	second := Paths{Root: "/cfg", StateRoot: "/state"}.ControlSocket("prod")
	if first != second {
		t.Fatalf("ControlSocket not deterministic: %q vs %q", first, second)
	}
	// Distinct aliases → distinct sockets.
	if p.ControlSocket("prod") == p.ControlSocket("staging") {
		t.Fatal("distinct aliases collided")
	}

	// Even a very long alias under a deep state root stays well under the
	// ~104-byte unix socket path limit.
	long := strings.Repeat("very-long-alias-segment.", 20)
	deep := Paths{StateRoot: "/home/somebody/.local/state/fvmux"}
	if n := len(deep.ControlSocket(long)); n > 104 {
		t.Fatalf("socket path too long: %d bytes", n)
	}
}
