package config

import (
	"errors"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// State is the managed runtime state file (state.toml). Holds first-run
// flag, last-version stamp, and the welcome-shown timestamp.
type State struct {
	FirstRunDone   bool      `toml:"first_run_done"`
	LastSession    string    `toml:"last_session"`
	LastVersion    string    `toml:"last_version"`
	WelcomeShownAt time.Time `toml:"welcome_shown_at"`
}

// LoadState reads path. Missing file ⇒ zero-value State + nil error.
func LoadState(path string) (*State, error) {
	st := &State{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := toml.Unmarshal(data, st); err != nil {
		return st, err
	}
	return st, nil
}

// SaveState writes s to path.
func SaveState(path string, s *State) error {
	data, err := toml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
