// Package atomicfile writes a file by way of a temp + rename so a
// crash mid-write can never leave the original truncated. Used for
// every fvmux config / state / session write where a partially-
// written file would be worse than a stale-but-intact one.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces path with data, atomically. The write goes to
// path+".tmp", is fsynced, then renamed over the target. perm is the
// final mode of the renamed file. If anything fails, the .tmp file is
// removed and the original path is untouched.
//
// Callers should pass 0o600 for files that may contain sensitive
// per-user state (state.toml, keybindings.toml, sessions/*.toml) and
// 0o644 for shareable config (config.toml, profiles.toml, hosts.toml).
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp := path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
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
