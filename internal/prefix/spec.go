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
