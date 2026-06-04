// Package atomicfile writes a file by way of a temp + rename so a
// crash mid-write can never leave the original truncated. Used for
// every fvmux config / state / session write where a partially-
// written file would be worse than a stale-but-intact one.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces path with data, atomically. The write goes to a
// uniquely-named temp file in the same directory, is fsynced, then
// renamed over the target. perm is the final mode of the renamed file.
// If anything fails, the temp file is removed and the original path is
// untouched.
//
// The temp file is created with a unique name (O_EXCL via os.CreateTemp)
// rather than a fixed path+".tmp", so two processes writing the same
// target concurrently never clobber each other's temp file — fvmux is
// designed to run as multiple instances inside one tmux.
//
// Callers should pass 0o600 for files that may contain sensitive
// per-user state (state.toml, keybindings.toml, sessions/*.toml) and
// 0o644 for shareable config (config.toml, profiles.toml, hosts.toml).
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()

	// CreateTemp makes the file 0o600; bring it to the requested mode
	// before publishing so shareable configs land 0o644.
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// fsync the directory so the rename itself is durable. Best-effort
	// — directory fsync is unsupported on some filesystems.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
