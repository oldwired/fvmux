package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/keys"
	"github.com/oldwired/fvmux/internal/prefix"
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
// missing file yields nil slices and nil error — empty config is a
// valid state. A parse error yields nil slices and the error: the file
// is hand-edited and never auto-overwritten, so the caller warns and runs
// with no overrides (leaving the user's file intact to fix) rather than
// applying a partial set.
//
// rejected lists bindings whose chord the prefix dispatcher can never
// emit (arrows, F-keys, A-/S- modifiers, wrong step count). They are
// NOT applied — applying one would strip the command's factory chord
// while the replacement never fires, leaving the command unreachable —
// and callers must surface them to the user.
func LoadKeybindings(path string) (overrides []commands.Override, rejected []string, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var f keybindingsFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, nil, err
	}
	overrides = make([]commands.Override, 0, len(f.Bindings))
	for _, b := range f.Bindings {
		// Canonicalise so "c-g tab" / "C-g Tab" both match the
		// registry's binding form regardless of how the user spelled
		// it. (Chords stay in the default-prefix space; a non-default
		// prefix is applied afterwards — see main.go.)
		chord := keys.Canonical(b.Chord)
		// Removal entries (command = "") are exempt: they only ever
		// delete an existing binding, so an odd chord is a no-op.
		if b.Command != "" && !prefix.DispatchableChord(chord) {
			rejected = append(rejected,
				fmt.Sprintf("%s → %s (the dispatcher can't emit this chord)", b.Chord, b.Command))
			continue
		}
		overrides = append(overrides, commands.Override{
			Chord:   chord,
			Command: b.Command,
		})
	}
	return overrides, rejected, nil
}
