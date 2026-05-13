// Package debug ships opt-in file-based logging so instrumentation can
// live in the code without spamming stderr in normal runs.
//
// Toggles (any non-empty value enables):
//
//	FVMUX_DEBUG=1          turn on everything.
//	FVMUX_DEBUG_MOUSE=1    mouse-event logging (MouseView + Mux dispatch).
//	FVMUX_DEBUG_FILE=path  override default log location.
//
// Default log path: $XDG_STATE_HOME/fvmux/debug.log (typically
// ~/.local/state/fvmux/debug.log). The file is appended; lines are
// prefixed with a monotonic timestamp + category. Mutex-guarded, safe
// across goroutines.
//
// To collect a session's worth of logs:
//
//	rm ~/.local/state/fvmux/debug.log
//	FVMUX_DEBUG_MOUSE=1 fvmuxa
//	# reproduce the bug, exit fvmux
//	cat ~/.local/state/fvmux/debug.log
package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	writer  *os.File
	started = time.Now()
)

// Mouse reports whether mouse-event logging is enabled.
func Mouse() bool { return enabled("FVMUX_DEBUG_MOUSE") }

// Logf appends one timestamped line under the given category. Cheap
// when caller has already gated on Mouse() / similar; safe to call
// even when no category toggle is on (writes nothing in that case).
func Logf(category, format string, args ...any) {
	if !anyOn() {
		return
	}
	line := fmt.Sprintf("[%-12s %s] %s\n",
		time.Since(started).Truncate(time.Millisecond),
		category,
		fmt.Sprintf(format, args...))
	emit(line)
}

func emit(line string) {
	mu.Lock()
	defer mu.Unlock()
	if writer == nil {
		writer = openWriter()
		if writer == nil {
			return
		}
	}
	_, _ = writer.WriteString(line)
	_ = writer.Sync()
}

func openWriter() *os.File {
	path := os.Getenv("FVMUX_DEBUG_FILE")
	if path == "" {
		state := os.Getenv("XDG_STATE_HOME")
		if state == "" {
			home, _ := os.UserHomeDir()
			state = filepath.Join(home, ".local", "state")
		}
		path = filepath.Join(state, "fvmux", "debug.log")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// Fall back to stderr; better than silently losing diagnostics.
		fmt.Fprintln(os.Stderr, "fvmux debug: couldn't open log file:", err)
		return nil
	}
	header := fmt.Sprintf("\n=== fvmux debug session %s ===\n",
		time.Now().Format(time.RFC3339))
	_, _ = f.WriteString(header)
	return f
}

func enabled(k string) bool {
	if os.Getenv("FVMUX_DEBUG") != "" {
		return true
	}
	return os.Getenv(k) != ""
}

func anyOn() bool {
	return os.Getenv("FVMUX_DEBUG") != "" ||
		os.Getenv("FVMUX_DEBUG_MOUSE") != ""
}

// LogPath returns the resolved log path for inclusion in error
// messages / docs.
func LogPath() string {
	path := os.Getenv("FVMUX_DEBUG_FILE")
	if path == "" {
		state := os.Getenv("XDG_STATE_HOME")
		if state == "" {
			home, _ := os.UserHomeDir()
			state = filepath.Join(home, ".local", "state")
		}
		path = filepath.Join(state, "fvmux", "debug.log")
	}
	return path
}
