package config

import (
	"fmt"
	"os"
	"time"
)

// BackupCorrupt renames a file that failed to parse out of the way so it
// is preserved for the user to inspect while fvmux falls back to defaults
// and (for auto-written files) re-creates a clean version. Returns the
// backup path on success. Best-effort: a rename failure is reported but
// must not stop startup.
//
// Used only for files fvmux auto-overwrites (config.toml, state.toml).
// Hand-edited files (profiles.toml, keybindings.toml) are left intact —
// nothing overwrites them, so there is no data-loss risk to mitigate.
func BackupCorrupt(path string) (string, error) {
	bak := fmt.Sprintf("%s.bad-%s", path, time.Now().Format("20060102-150405"))
	// Avoid clobbering a backup from an earlier crash in the same second.
	if _, err := os.Stat(bak); err == nil {
		bak = fmt.Sprintf("%s.bad-%s-%d", path, time.Now().Format("20060102-150405"), os.Getpid())
	}
	if err := os.Rename(path, bak); err != nil {
		return "", err
	}
	return bak, nil
}
