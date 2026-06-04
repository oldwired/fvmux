//go:build !windows

package config

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockState takes an exclusive flock on a dedicated lock file (never the
// state file itself, which atomicfile replaces via rename). The returned
// closure releases the lock and closes the descriptor.
func lockState(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}
