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
//	~/.local/state/fvmux/cm/<alias>.sock  — SSH control sockets (step 9;
//	                                         deep roots use a short /tmp dir)
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// controlSocketPathLimit is deliberately below the shortest common Unix
// sockaddr_un.sun_path capacity (104 bytes on macOS, including its trailing
// NUL). Keeping a few bytes in reserve avoids platform-specific off-by-one
// behaviour in ssh and the Go net package.
const controlSocketPathLimit = 100

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
// the ControlMaster socket identity at the global XDG location would leave
// that isolation half-done — MRU writes interleaving between instances,
// masters shared with the main one. Deep roots may use ControlSocketDir's
// hashed /tmp fallback, whose hash still includes this relocated StateRoot.
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
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// Control sockets may live under a shared temporary parent when a deep
	// -config root would exceed sockaddr_un's path limit. Keep the leaf private
	// even if its parent is world-writable.
	socketDir := p.ControlSocketDir()
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(socketDir, 0o700); err != nil {
		return err
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

// ControlSocketDir is the directory holding SSH ControlMaster sockets. The
// normal location is StateRoot/cm. Unix-domain socket paths have a small
// platform limit, though, so a deeply nested -config root uses a deterministic
// private directory directly under /tmp instead. The StateRoot hash preserves
// instance isolation without embedding the long root in the socket path.
func (p Paths) ControlSocketDir() string {
	preferred := filepath.Join(p.StateRoot, "cm")
	if runtime.GOOS == "windows" || len(filepath.Join(preferred, strings.Repeat("x", 16)+".sock")) <= controlSocketPathLimit {
		return preferred
	}
	home, _ := os.UserHomeDir()
	sum := sha256.Sum256([]byte(home + "\x00" + filepath.Clean(p.StateRoot)))
	return filepath.Join("/tmp", "fvmux-cm-"+hex.EncodeToString(sum[:8]))
}

// ControlSocket returns the ControlMaster socket path for alias. The
// filename is a short hash of the alias rather than the alias itself, so
// that (a) aliases containing path separators / "../" can't escape the
// cm dir, and (b) together with ControlSocketDir's short-path fallback,
// the assembled path stays below Unix socket-path limits.
// The hash is deterministic, so the same alias always maps to the same
// socket and masters are reused across connections and fvmux instances.
func (p Paths) ControlSocket(alias string) string {
	sum := sha256.Sum256([]byte(alias))
	return filepath.Join(p.ControlSocketDir(), hex.EncodeToString(sum[:8])+".sock")
}

// ThemesDir is the directory fvmux scans for user theme TOMLs at
// startup. Themes follow the schema in internal/theme/load.go.
func (p Paths) ThemesDir() string { return filepath.Join(p.Root, "themes") }
