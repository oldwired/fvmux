package config

// WithStateLock runs fn while holding an exclusive advisory lock tied to
// statePath, serialising the load-modify-save cycle on state.toml across
// concurrent fvmux instances (fvmux is designed to run multiple instances
// inside one tmux, so two of them updating PaletteMRU / FirstRunDone would
// otherwise lose each other's writes).
//
// Best-effort: if the lock cannot be taken (unsupported platform or
// filesystem), fn runs anyway — the worst case is the pre-existing
// last-writer-wins behaviour, never a hang.
func WithStateLock(statePath string, fn func() error) error {
	unlock, err := lockState(statePath + ".lock")
	if err != nil {
		return fn()
	}
	defer unlock()
	return fn()
}
