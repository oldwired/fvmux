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
// missing file yields nil slices and nil error — empty config is a
// valid state. A parse error yields nil slices and the error: the file
// is hand-edited and never auto-overwritten, so the caller warns and runs
// with no overrides (leaving the user's file intact to fix) rather than
// applying a partial set.
//
// diagnostics contains syntax/reachability errors and portability warnings.
// Invalid entries are never returned as overrides, so applying the valid
// subset cannot accidentally strip a command's working factory chord.
func LoadKeybindings(path, activePrefix string) (overrides []commands.Override, diagnostics []commands.BindingDiagnostic, err error) {
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
	for i, b := range f.Bindings {
		index := i + 1
		chord, keyDiagnostics := keys.ValidateBindingChord(b.Chord, activePrefix)
		invalid := false
		for _, d := range keyDiagnostics {
			diagnostics = append(diagnostics, commands.BindingDiagnostic{
				Severity: string(d.Severity), Index: index, Chord: b.Chord,
				Command: b.Command, Reason: d.Reason,
			})
			if d.Severity == keys.SeverityError {
				invalid = true
			}
		}
		if invalid {
			continue
		}
		overrides = append(overrides, commands.Override{
			Index:   index,
			Chord:   chord,
			Command: b.Command,
		})
	}
	return overrides, diagnostics, nil
}
