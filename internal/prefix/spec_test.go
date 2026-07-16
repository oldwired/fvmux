package prefix

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
)

// TestSpec_LiteralByte is the regression for finding #19: the double-tap
// literal-prefix forward derives its raw byte from the live spec's Ctrl-letter
// key code (Ctrl-A…Ctrl-Z ⇒ 0x01…0x1a), never a hardcoded BEL. Each preset in
// Available resolves through Lookup to the expected control byte.
func TestSpec_LiteralByte(t *testing.T) {
	cases := []struct {
		configKey string
		want      byte
	}{
		{"C-g", 0x07},
		{"C-b", 0x02},
		{"C-a", 0x01},
	}
	for _, tc := range cases {
		spec := Lookup(tc.configKey)
		if spec.ConfigKey != tc.configKey {
			t.Fatalf("Lookup(%q) resolved to %q — not the expected preset", tc.configKey, spec.ConfigKey)
		}
		if got := spec.LiteralByte(); got != tc.want {
			t.Errorf("Spec(%s).LiteralByte() = %#x, want %#x", tc.configKey, got, tc.want)
		}
	}
}

// TestSpec_LiteralByte_NonCtrlIsZero: a Spec whose KeyCode isn't a Ctrl-letter
// (e.g. a bare Tab) has no single control-byte representation, so LiteralByte
// returns 0 rather than emitting a stray byte.
func TestSpec_LiteralByte_NonCtrlIsZero(t *testing.T) {
	s := Spec{KeyCode: consts.KbTab, ChordToken: "Tab", ConfigKey: "Tab"}
	if got := s.LiteralByte(); got != 0 {
		t.Errorf("non-Ctrl Spec.LiteralByte() = %#x, want 0", got)
	}
}
