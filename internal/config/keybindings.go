package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/keys"
)

// keybindingsFile is the on-disk shape of ~/.config/fvmux/keybindings.toml.
type keybindingsFile struct {
	Bindings []bindingTOML `toml:"binding"`
}

type bindingTOML struct {
	Chord   string `toml:"chord"`
	Command string `toml:"command"` // empty ⇒ removes the binding
}

// LoadKeybindings parses path into a slice of registry overrides. A
// missing file yields a nil slice and nil error — empty config is a
// valid state. A parse error yields a nil slice and the error: the file
// is hand-edited and never auto-overwritten, so the caller warns and runs
// with no overrides (leaving the user's file intact to fix) rather than
// applying a partial set.
func LoadKeybindings(path string) ([]commands.Override, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f keybindingsFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	out := make([]commands.Override, 0, len(f.Bindings))
	for _, b := range f.Bindings {
		out = append(out, commands.Override{
			// Canonicalise so "c-g tab" / "C-g Tab" both match the
			// registry's binding form regardless of how the user spelled
			// it. (Chords stay in the default-prefix space; a non-default
			// prefix is applied afterwards — see main.go.)
			Chord:   keys.Canonical(b.Chord),
			Command: b.Command,
		})
	}
	return out, nil
}
