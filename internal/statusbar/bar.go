// Package statusbar composes fvmux's bottom-row status line: session
// name, window list with activity/bell markers, focused pane title,
// clock — laid out via fv-go's LeftItems / RightItems slots on
// menus.StatusLine.
package statusbar

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/menus"
)

// Bar wraps a *menus.StatusLine and a clock-format. The host (Mux)
// calls Refresh(Snapshot) periodically (1s ticker) and after state
// changes to keep the visible items in sync.
type Bar struct {
	Line *menus.StatusLine

	ClockFormat string
}

// WindowEntry is one window's representation in the status-line window
// list. The first marker wins: bell '!' > focused '*' > activity '-'.
type WindowEntry struct {
	Number   int
	Title    string
	Focused  bool
	Activity bool
	Bell     bool
}

// Snapshot is the live state the bar paints. The host builds one fresh
// on every refresh.
type Snapshot struct {
	SessionName  string
	Windows      []WindowEntry
	FocusedTitle string
	FocusedCWD   string
	SyncInput    bool
	PrefixArmed  bool
	ResizeMode   bool
}

// Build returns a *Bar holding a freshly-constructed status line.
// Initial items are placeholders until the first Refresh call.
func Build(bounds geom.Rect, sessionName, clockFormat string) *Bar {
	if clockFormat == "" {
		clockFormat = "15:04"
	}
	b := &Bar{
		Line:        menus.NewStatusLine(bounds, nil),
		ClockFormat: clockFormat,
	}
	b.Refresh(Snapshot{SessionName: sessionName})
	return b
}

// Refresh rewrites both items in place. Cheap; safe to call once a
// second.
func (b *Bar) Refresh(s Snapshot) {
	if b == nil || b.Line == nil || b.Line.Defs == nil {
		return
	}

	left := b.formatLeft(s)
	right := b.formatRight(s)

	setItem(&b.Line.Defs.LeftItems, left)
	setItem(&b.Line.Defs.RightItems, right)
}

func (b *Bar) formatLeft(s Snapshot) string {
	var sb strings.Builder
	if s.PrefixArmed {
		sb.WriteString("*PREFIX* ")
	}
	if s.ResizeMode {
		sb.WriteString("*RESIZE* ")
	}
	sb.WriteByte('[')
	if s.SessionName == "" {
		sb.WriteString("fvmux")
	} else {
		sb.WriteString(escapeStatus(s.SessionName))
	}
	sb.WriteByte(']')
	if s.SyncInput {
		sb.WriteString(" [SYNC]")
	}
	for _, w := range s.Windows {
		sb.WriteByte(' ')
		fmt.Fprintf(&sb, "%d:%s", w.Number, escapeStatus(truncate(w.Title, 12)))
		switch {
		case w.Bell:
			sb.WriteByte('!')
		case w.Focused:
			sb.WriteByte('*')
		case w.Activity:
			sb.WriteByte('-')
		}
	}
	return sb.String()
}

func (b *Bar) formatRight(s Snapshot) string {
	var sb strings.Builder
	if s.FocusedTitle != "" {
		sb.WriteString(escapeStatus(truncate(s.FocusedTitle, 24)))
		if s.FocusedCWD != "" {
			sb.WriteString(" ▸ ")
			sb.WriteString(escapeStatus(filepath.Base(s.FocusedCWD)))
		}
		sb.WriteString("  ")
	}
	sb.WriteString(time.Now().Format(b.ClockFormat))
	return sb.String()
}

func setItem(slot *[]*menus.StatusItem, text string) {
	if len(*slot) == 0 {
		*slot = []*menus.StatusItem{{Text: text}}
		return
	}
	(*slot)[0].Text = text
}

// escapeStatus strips tildes — fv-go's status-line renderer treats
// `~text~` as a hot-key region, so user-controlled strings (window
// titles, session name) would silently mis-render.
func escapeStatus(s string) string {
	if !strings.Contains(s, "~") {
		return s
	}
	return strings.ReplaceAll(s, "~", "")
}

func truncate(s string, max int) string {
	if max <= 1 {
		return s
	}
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
