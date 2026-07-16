// Package config owns fvmux's TOML configuration and XDG path layout.
//
// Locations (Linux/macOS defaults; both honour $XDG_CONFIG_HOME and
// $XDG_STATE_HOME):
//
//	~/.config/fvmux/config.toml           — general settings
//	~/.config/fvmux/profiles.toml         — spawn templates
//	~/.config/fvmux/keybindings.toml      — chord overrides
//	~/.config/fvmux/sessions/<name>.toml  — saved sessions
//	~/.config/fvmux/hosts.toml            — SSH host augmentations (step 9)
//	~/.local/state/fvmux/state.toml       — managed first-run / mru state
//	~/.local/state/fvmux/cm/<alias>.sock  — SSH control sockets (step 9)
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Paths resolves every fvmux file location. Override Root to relocate
// the whole tree (-config flag, tests' tmpdirs, etc.).
type Paths struct {
	Root      string // base config dir
	StateRoot string // base state dir
}

// Default returns the XDG-derived paths. Honours $XDG_CONFIG_HOME and
// $XDG_STATE_HOME if set.
func Default() Paths {
	home, _ := os.UserHomeDir()
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	return Paths{
		Root:      filepath.Join(cfg, "fvmux"),
		StateRoot: filepath.Join(state, "fvmux"),
	}
}

// WithRoot overrides the config root (used by -config and by tests).
// The state root moves under it too: -config exists to isolate an
// instance, and keeping state.toml (first-run flag, palette MRU) and
// the ControlMaster socket dir at the global XDG location would leave
// that isolation half-done — MRU writes interleaving between
// instances, masters shared with the main one.
func (p Paths) WithRoot(root string) Paths {
	if root == "" {
		return p
	}
	return Paths{Root: root, StateRoot: filepath.Join(root, "state")}
}

// EnsureDirs creates every directory fvmux writes into.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{
		p.Root,
		p.SessionsDir(),
		filepath.Join(p.Root, "themes"),
		p.StateRoot,
		filepath.Join(p.StateRoot, "cm"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) ConfigFile() string      { return filepath.Join(p.Root, "config.toml") }
func (p Paths) ProfilesFile() string    { return filepath.Join(p.Root, "profiles.toml") }
func (p Paths) KeybindingsFile() string { return filepath.Join(p.Root, "keybindings.toml") }
func (p Paths) HostsFile() string       { return filepath.Join(p.Root, "hosts.toml") }
func (p Paths) StateFile() string       { return filepath.Join(p.StateRoot, "state.toml") }

// SessionsDir is the directory holding saved session TOMLs.
func (p Paths) SessionsDir() string { return filepath.Join(p.Root, "sessions") }

func (p Paths) SessionFile(name string) string {
	// Defense in depth: a name that fails ValidSessionName (path
	// separators, "..") is hashed — mirroring ControlSocket — so a
	// hostile name can never make an autosave escape the sessions dir,
	// even if a new caller forgets to validate at the entry point.
	if ValidSessionName(name) != nil {
		sum := sha256.Sum256([]byte(name))
		name = hex.EncodeToString(sum[:8])
	}
	return filepath.Join(p.SessionsDir(), name+".toml")
}

// ValidSessionName rejects session names that can't safely be embedded
// in a file path: empty names, path separators, "." / "..", and names
// differing from their own filepath.Base. Every surface that accepts a
// session name (-session flag, Save Session As) must call this before
// the name reaches SessionFile.
func ValidSessionName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("session name is empty")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." ||
		filepath.Base(name) != name {
		return fmt.Errorf("session name %q must be a plain file name (no path separators or ..)", name)
	}
	return nil
}

// ControlSocketDir is the directory holding SSH ControlMaster sockets.
func (p Paths) ControlSocketDir() string { return filepath.Join(p.StateRoot, "cm") }

// ControlSocket returns the ControlMaster socket path for alias. The
// filename is a short hash of the alias rather than the alias itself, so
// that (a) aliases containing path separators / "../" can't escape the
// cm dir, and (b) the assembled path can never exceed the ~104-byte unix
// socket-path limit (a long alias under a deep $XDG_STATE_HOME would
// otherwise make ssh silently fall back to a non-multiplexed connection).
// The hash is deterministic, so the same alias always maps to the same
// socket and masters are reused across connections and fvmux instances.
func (p Paths) ControlSocket(alias string) string {
	sum := sha256.Sum256([]byte(alias))
	return filepath.Join(p.ControlSocketDir(), hex.EncodeToString(sum[:8])+".sock")
}

// ThemesDir is the directory fvmux scans for user theme TOMLs at
// startup. Themes follow the schema in internal/theme/load.go.
func (p Paths) ThemesDir() string { return filepath.Join(p.Root, "themes") }
