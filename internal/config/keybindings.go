package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/oldwired/fvmux/internal/commands"
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
// valid state. Parse errors return the file's error along with whatever
// bindings parsed successfully (BurntSushi's behaviour on partial
// decode), so callers can still ApplyOverrides on the good ones.
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
			Chord:   b.Chord,
			Command: b.Command,
		})
	}
	return out, nil
}
