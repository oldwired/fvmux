//go:build windows

package config

// lockState is a no-op on Windows: fvmux's multi-instance story is the
// tmux/unix workflow, and the state file is written atomically regardless.
// Concurrent Windows instances fall back to last-writer-wins.
func lockState(string) (func(), error) {
	return func() {}, nil
}
