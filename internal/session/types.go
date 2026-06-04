// Package session holds the long-lived runtime types: panes and the
// persistent snapshot format used by Save / Load.
//
// The Mux owns live per-window state directly; session's role is the
// persistent shape: Snapshot, WindowSnapshot, and the helpers that
// marshal them via TOML.
package session

import (
	"sync/atomic"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"
)

// PaneID is a process-unique identifier for one terminal pane.
type PaneID uint64

var nextPaneID atomic.Uint64

// NewPaneID allocates a fresh, unique PaneID.
func NewPaneID() PaneID { return PaneID(nextPaneID.Add(1)) }

// WindowID is a process-unique identifier for one fvmux window.
type WindowID uint64

var nextWindowID atomic.Uint64

// NewWindowID allocates a fresh, unique WindowID.
func NewWindowID() WindowID { return WindowID(nextWindowID.Add(1)) }

// Pane is one terminal pane: a running PTY + the metadata fvmux needs
// to render its status, debounce activity flashes, and serialize a
// session snapshot.
//
// Activity / Bell are updated by the Mux from the terminal's OnActivity
// / OnBell callbacks; consumers read them but don't write.
type Pane struct {
	ID      PaneID
	Term    *terminal.Terminal
	Title   string
	Profile string // empty for ad-hoc spawns

	// CloseOnExit, copied from the source profile, asks the Mux to
	// remove the pane (and possibly the window) as soon as the child
	// process exits — set by per-profile config, default off.
	CloseOnExit bool

	// Lifecycle state.
	Dead    bool
	ExitErr error

	// Rot13, when set, makes the pane's OnFeed filter rotate output (the
	// :rot13 easter egg). An atomic so the egg's timer can flip it back
	// without racing the terminal read loop that consults it — the filter
	// itself is installed once at spawn, never swapped on a live terminal.
	Rot13 atomic.Bool

	// Most-recent rect we materialized this pane into (window-local).
	// FocusDir compares centres in this space.
	LastRect geom.Rect

	// CWD reported by the shell via OSC 7.
	CWD string

	// Activity is the timestamp of the most recent debounced byte
	// arrival from the child. BellAt is the timestamp of the most
	// recent OnBell — IsBellActive() flags one in the past 4 seconds.
	Activity time.Time
	BellAt   time.Time
}

// IsActivityRecent reports whether activity arrived within the
// flash window (4 s) — drives the '-' marker in the status-line
// window list.
func (p *Pane) IsActivityRecent() bool {
	if p == nil || p.Activity.IsZero() {
		return false
	}
	return time.Since(p.Activity) < 4*time.Second
}

// IsBellActive reports whether the bell rang within the flash window
// (4 s) — drives the '!' marker in the status-line window list.
func (p *Pane) IsBellActive() bool {
	if p == nil || p.BellAt.IsZero() {
		return false
	}
	return time.Since(p.BellAt) < 4*time.Second
}
