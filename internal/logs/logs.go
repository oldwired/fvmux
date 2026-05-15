// Package logs is fvmux's slog plumbing: structured log records flow
// into a bounded in-memory ring (so the log viewer can show the tail
// of recent activity) AND, when -log=path is set, into a text file.
//
// Both sinks are attached at startup via Init; subsequent slog calls
// from anywhere in fvmux automatically populate them.
package logs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Entry is one structured record. The logviewer widget renders these
// with severity-coloured levels and timestamps; the file sink writes
// them as one line of text per entry.
type Entry struct {
	Time   time.Time
	Level  slog.Level
	Source string
	Msg    string
}

var (
	once    sync.Once
	initErr error

	mu        sync.Mutex
	ring      []Entry
	ringHead  int
	ringCap   int
	sink      io.Writer
	sinkClose func() error // nil for ring-only init; closes the file sink.
)

// Init sets up the ring and (optionally) the file sink. ringSize must
// be ≥ 16; values below that are clamped. path may be empty (no file
// sink). Safe to call multiple times — only the first call takes
// effect. Returns the file-open error (if any); ring-only init never
// fails.
func Init(path string, ringSize int) error {
	once.Do(func() {
		if ringSize < 16 {
			ringSize = 16
		}
		ringCap = ringSize
		ring = make([]Entry, 0, ringSize)

		if path != "" {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				initErr = err
			} else {
				sink = f
				sinkClose = f.Close
			}
		}

		slog.SetDefault(slog.New(&handler{}))
		// First entry confirms the log is live.
		slog.Info("fvmux logs initialised", "ring_size", ringSize, "file", path)
	})
	return initErr
}

// Close flushes and closes the file sink (if any). Idempotent and safe
// to call from a process-exit defer. The ring buffer remains
// readable after Close so deferred log-dump callers still get data.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if sinkClose == nil {
		return nil
	}
	err := sinkClose()
	sinkClose = nil
	sink = nil
	return err
}

// Entries returns a copy of the current ring contents in chronological
// order (oldest first).
func Entries() []Entry {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Entry, 0, len(ring))
	if len(ring) < ringCap {
		out = append(out, ring...)
		return out
	}
	out = append(out, ring[ringHead:]...)
	out = append(out, ring[:ringHead]...)
	return out
}

// handler is our slog.Handler that fans out to ring + file.
type handler struct{ attrs []slog.Attr }

func (h *handler) Enabled(_ context.Context, lv slog.Level) bool { return lv >= slog.LevelDebug }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	e := Entry{Time: r.Time, Level: r.Level, Msg: r.Message}
	// Source: first "src" attr (handler-attached or per-record), else
	// empty. Check the handler's accumulated attrs first so slog.With(
	// "src", "...") sticks across subsequent calls.
	for _, a := range h.attrs {
		if a.Key == "src" {
			e.Source = a.Value.String()
			break
		}
	}
	if e.Source == "" {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "src" {
				e.Source = a.Value.String()
				return false
			}
			return true
		})
	}
	appendRing(e)
	// Read sink under the same lock that protects sinkClose so a
	// concurrent Close can't race with an in-flight write.
	mu.Lock()
	w := sink
	mu.Unlock()
	if w != nil {
		_, _ = fmt.Fprintf(w, "%s [%s] %s %s\n",
			e.Time.Format("2006-01-02T15:04:05.000"),
			levelString(e.Level),
			e.Source,
			formatAttrs(e.Msg, h.attrs, &r),
		)
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
}

func (h *handler) WithGroup(string) slog.Handler { return h }

func appendRing(e Entry) {
	mu.Lock()
	defer mu.Unlock()
	if len(ring) < ringCap {
		ring = append(ring, e)
		return
	}
	ring[ringHead] = e
	ringHead = (ringHead + 1) % ringCap
}

func levelString(lv slog.Level) string {
	switch {
	case lv >= slog.LevelError:
		return "ERROR"
	case lv >= slog.LevelWarn:
		return "WARN "
	case lv >= slog.LevelInfo:
		return "INFO "
	default:
		return "DEBUG"
	}
}

func formatAttrs(msg string, handlerAttrs []slog.Attr, r *slog.Record) string {
	var s = msg
	for _, a := range handlerAttrs {
		s += " " + a.Key + "=" + a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		s += " " + a.Key + "=" + a.Value.String()
		return true
	})
	return s
}
