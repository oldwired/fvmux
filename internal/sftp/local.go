package sftp

import "os"

// defaultLocalRoot returns the user's home directory or "." if home
// can't be resolved. Used as the initial cwd of the dual-pane
// browser's local-side panel.
func defaultLocalRoot() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
