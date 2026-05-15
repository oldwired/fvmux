package session

import (
	"errors"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/oldwired/fvmux/internal/atomicfile"
)

// Snapshot is the persistent form of one fvmux session, written to
// ~/.config/fvmux/sessions/<name>.toml. It captures every window's
// position, title, and layout (encoded via internal/layout/serde) so a
// later run with -session=<name> can restore the same workspace.
type Snapshot struct {
	Name     string            `toml:"name"`
	Created  time.Time         `toml:"created"`
	Active   int               `toml:"active"`
	Windows  []*WindowSnapshot `toml:"window"`
	MetaPath string            `toml:"-"`
}

// WindowSnapshot is one window inside a Snapshot.
type WindowSnapshot struct {
	ID        uint64   `toml:"id"`
	Number    int      `toml:"number"`
	Title     string   `toml:"title"`      // profile-derived fallback caption.
	UserTitle string   `toml:"user_title"` // sticky user-set name; empty = none.
	Pos       RectTOML `toml:"pos"`
	Layout    string   `toml:"layout"` // see internal/layout/serde for grammar
}

// RectTOML is geom.Rect spelt with explicit x/y/w/h keys for clarity
// in the TOML file.
type RectTOML struct {
	X int `toml:"x"`
	Y int `toml:"y"`
	W int `toml:"w"`
	H int `toml:"h"`
}

// Load reads a Snapshot from path. A missing file is reported as an
// error (the caller's -session=name asked for a specific session).
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := toml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.MetaPath = path
	return &s, nil
}

// Save writes s to s.MetaPath (must be set by the caller). Snapshots
// hold per-user window geometry and titles; written atomically with
// 0o600 perms so a crash mid-write can't truncate the session file.
func (s *Snapshot) Save() error {
	if s.MetaPath == "" {
		return errors.New("session.Snapshot.Save: MetaPath unset")
	}
	data, err := toml.Marshal(s)
	if err != nil {
		return err
	}
	return atomicfile.Write(s.MetaPath, data, 0o600)
}
