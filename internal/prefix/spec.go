package prefix

import "github.com/oldwired/fv-go/pkg/fv/consts"

// Spec describes one prefix-key choice: the fv-go KeyCode the listener
// arms on, and the chord-string token (the leading step every
// registry chord shares — "C-g", "C-b", "C-a", …).
type Spec struct {
	KeyCode    uint16
	ChordToken string // e.g., "C-g"
	Label      string // e.g., "Ctrl-G (default)"
	ConfigKey  string // canonical value for config.toml — matches ChordToken
}

// Available is the v1 menu of prefix choices. v2 may grow this with
// Custom… input; for now we ship three presets.
var Available = []Spec{
	{KeyCode: consts.KbCtrlG, ChordToken: "C-g", Label: "Ctrl-G (default)", ConfigKey: "C-g"},
	{KeyCode: consts.KbCtrlB, ChordToken: "C-b", Label: "Ctrl-B (tmux-style)", ConfigKey: "C-b"},
	{KeyCode: consts.KbCtrlA, ChordToken: "C-a", Label: "Ctrl-A (screen-style)", ConfigKey: "C-a"},
}

// Default is the fallback used when config.toml's prefix_key is empty
// or unrecognised.
var Default = Available[0]

// Lookup returns the Spec matching configKey (e.g., "C-g"), or Default
// if none matches.
func Lookup(configKey string) Spec {
	for _, s := range Available {
		if s.ConfigKey == configKey {
			return s
		}
	}
	return Default
}

// LiteralByte is the raw byte the prefix key feeds a terminal
// (Ctrl-A…Ctrl-Z ⇒ 0x01…0x1a). The double-tap-prefix command forwards
// this — derived from the live spec, never hardcoded, so rebinding the
// prefix to Ctrl-B forwards 0x02 rather than a stray BEL. Returns 0
// when the key code isn't a Ctrl-letter.
func (s Spec) LiteralByte() byte {
	if l, ok := ctrlLetters[s.KeyCode]; ok {
		return byte(l-'a') + 1
	}
	return 0
}
