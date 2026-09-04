package app

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/sftp"
	"github.com/oldwired/fvmux/internal/sshmgr"
)

// SaveSession captures the current window/layout state to TOML under
// the configured paths. When no session is currently named (i.e., the
// user didn't pass -session=NAME and hasn't picked one yet) falls
// through to Save As so the user gets prompted instead of hitting a
// dead end.
func (m *Mux) SaveSession() {
	if m.Opts.SessionName == "" {
		m.saveSessionAs()
		return
	}
	snap := m.buildSnapshot()
	snap.MetaPath = m.Opts.Paths.SessionFile(m.Opts.SessionName)
	if err := snap.Save(); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't save session %s:\n%s",
			[]any{m.Opts.SessionName, err.Error()},
			msgbox.OKOnly)
	}
}

// SaveSessionSilent persists without showing UI dialogs. Used by
// shutdown hooks.
func (m *Mux) SaveSessionSilent() error {
	if m.Opts.SessionName == "" {
		return nil
	}
	snap := m.buildSnapshot()
	snap.MetaPath = m.Opts.Paths.SessionFile(m.Opts.SessionName)
	return snap.Save()
}

func (m *Mux) buildSnapshot() *session.Snapshot {
	snap := &session.Snapshot{
		Version: session.SnapshotVersion,
		Name:    m.Opts.SessionName,
		Created: time.Now(),
		Active:  0,
	}
	// Live SFTP browsers — one alias per open dialog. Dedup so two
	// browsers to the same alias only record once (restore will only
	// reopen once anyway).
	seen := map[string]bool{}
	for _, mgr := range sftp.LiveManagers() {
		if mgr == nil || mgr.Alias == "" || seen[mgr.Alias] {
			continue
		}
		seen[mgr.Alias] = true
		snap.SFTPAliases = append(snap.SFTPAliases, mgr.Alias)
	}
	// Active-window index: Desktop.Current() can be a dialog (SFTP
	// browser, msgbox) — fall back to m.lastFocused so we capture the
	// most recently focused fvmux window, not whichever popup happens
	// to be on top.
	cur := m.activeWindowKey()
	for i, key := range m.windowOrder {
		ws := m.windows[key]
		if ws == nil {
			continue
		}
		if key == cur {
			snap.Active = i
		}
		bv := ws.Frame.BaseView()
		bounds := geom.NewRect(bv.Origin.X, bv.Origin.Y,
			bv.Origin.X+bv.Size.X, bv.Origin.Y+bv.Size.Y)
		// Capture which leaf is focused / zoomed by index into the
		// leaves order serde preserves, so reload restores them rather
		// than always falling back to the first leaf, unzoomed.
		leaves := ws.Root.CollectLeaves()
		focusIdx := 0
		zoomed := 0 // 0 = not zoomed; 1-based leaf index otherwise.
		for i, l := range leaves {
			if l == ws.Focus {
				focusIdx = i
			}
			if ws.Zoomed != nil && l.Pane != nil && l.Pane.ID == *ws.Zoomed {
				zoomed = i + 1
			}
		}
		snap.Windows = append(snap.Windows, &session.WindowSnapshot{
			ID:        uint64(ws.ID),
			Number:    ws.Number,
			Title:     ws.Title,
			UserTitle: ws.UserTitle,
			Pos: session.RectTOML{
				X: bounds.A.X, Y: bounds.A.Y,
				W: bounds.Width(), H: bounds.Height(),
			},
			Layout:     layout.Marshal(ws.Root),
			FocusIndex: focusIdx,
			Zoomed:     zoomed,
			SyncInput:  ws.SyncInput,
		})
	}
	return snap
}

// LoadSession restores a snapshot: one window per snapshot entry +
// every SFTP browser the user had open. Callers should close existing
// windows first (openSessionPicker does); LoadSession itself does
// not — that lets it be used both for full session swap and for
// initial bootstrap.
func (m *Mux) LoadSession(snap *session.Snapshot) error {
	if snap == nil {
		return nil
	}
	if snap.Version > session.SnapshotVersion {
		// Forward-compat is best-effort: load anyway, unknown fields are
		// ignored. Pre-alpha — no migration, just a heads-up in the log.
		slog.Warn("session snapshot is from a newer fvmux",
			"file_version", snap.Version, "supported", session.SnapshotVersion)
	}
	// Restore is per-window best-effort: one un-execable profile must
	// not take down the other four windows (or, at startup, make
	// -session exit entirely). Failures are collected and reported once
	// the event loop is running.
	var failed []string
	var restoredKeys []views.View
	var restoredOriginalIndices []int
	// One hosts parse shared by every pane's profile fallback below.
	hostLookup := m.hostLookupOnce()
	for i, ws := range snap.Windows {
		bounds := geom.NewRect(ws.Pos.X, ws.Pos.Y,
			ws.Pos.X+ws.Pos.W, ws.Pos.Y+ws.Pos.H)
		if err := m.openSnapshotWindow(ws, bounds, hostLookup); err != nil {
			slog.Warn("session window failed to restore",
				"window", ws.Title, "err", err)
			failed = append(failed, fmt.Sprintf("%s: %v", ws.Title, err))
			continue
		}
		if len(m.windowOrder) > 0 {
			restoredKeys = append(restoredKeys, m.windowOrder[len(m.windowOrder)-1])
			restoredOriginalIndices = append(restoredOriginalIndices, i)
		}
	}
	if len(snap.Windows) > 0 && len(restoredKeys) == 0 {
		return fmt.Errorf("no window could be restored:\n%s",
			strings.Join(failed, "\n"))
	}
	if i := nearestRestoredIndex(snap.Active, len(snap.Windows), restoredOriginalIndices); i >= 0 {
		m.App.Desktop.Focus(restoredKeys[i])
	}
	if len(failed) > 0 {
		// Deferred via CallSoon: at startup LoadSession runs before
		// a.Run(), where a modal msgbox can't pump events yet.
		count, total := len(failed), len(snap.Windows)
		list := strings.Join(failed, "\n")
		views.CallSoon(func() {
			msgbox.Showf(&m.App.Desktop.Group, msgbox.Warning,
				"%d of %d windows couldn't be restored:\n%s",
				[]any{count, total, list}, msgbox.OKOnly)
		})
	}
	// SFTP browsers — schedule each. The restored ssh pane (if any)
	// for the same alias is already up and running; we just poll for
	// the master socket and open SFTP non-interactively when it
	// appears. If auth never completes within the timeout, the browser
	// silently doesn't open.
	for _, alias := range snap.SFTPAliases {
		m.scheduleSftpRestore(alias)
	}
	return nil
}

// nearestRestoredIndex maps a snapshot's active-window index to the closest
// window that actually restored. Distances are measured in the original
// snapshot, not the compressed success list; ties prefer the earlier window.
func nearestRestoredIndex(active, originalCount int, restoredOriginalIndices []int) int {
	if active < 0 || active >= originalCount || len(restoredOriginalIndices) == 0 {
		return -1
	}
	best, bestDistance := -1, originalCount+1
	for i, original := range restoredOriginalIndices {
		distance := original - active
		if distance < 0 {
			distance = -distance
		}
		if best == -1 || distance < bestDistance || (distance == bestDistance && original < restoredOriginalIndices[best]) {
			best, bestDistance = i, distance
		}
	}
	return best
}

// resolveProfileFallback fires when a saved pane's Profile name isn't
// a registered profile. The common case is ssh sessions opened via
// Ctrl-G H, whose Pane.Profile == the host alias. We look that alias
// up via lookup (hosts.toml + ~/.ssh/config); on a hit, synthesize an
// ssh command (with ControlOpts so the master is reused). Otherwise
// fall back to the default shell profile. lookup is injected so batch
// callers (LoadSession) can amortize one config parse across panes.
func (m *Mux) resolveProfileFallback(name string, lookup func(string) *sshmgr.Host) *profile.Profile {
	if name != "" && m.sshPool != nil {
		if h := lookup(name); h != nil {
			return m.sshProfile(h, h.Alias)
		}
	}
	return profile.Defaults()[0]
}

func (m *Mux) openSnapshotWindow(ws *session.WindowSnapshot, bounds geom.Rect, hostLookup func(string) *sshmgr.Host) error {
	// Sanitize the persisted number: snapshots written before the
	// numbering fix can contain duplicates, and a duplicate would make
	// one window unreachable via Ctrl-G <n> forever after.
	num := ws.Number
	if num <= 0 || m.windowNumberInUse(num) {
		num = m.nextWindowNumber()
	}
	w := views.NewWindow(bounds, ws.Title, num)
	interior := windowInterior(w)

	var spawned []*session.Pane
	spawn := func(spec layout.LeafSpec) (*session.Pane, error) {
		prof := profile.Find(m.Opts.Profiles, spec.Profile)
		if prof == nil {
			// Profile name not in profiles.toml — see if it's an
			// SSH host alias from hosts.toml / ~/.ssh/config and
			// synthesize an inline ssh profile if so. Without this
			// fallback, restored ssh panes silently turn into
			// generic shells.
			prof = m.resolveProfileFallback(spec.Profile, hostLookup)
		}
		pane, err := m.instantiateProfile(prof, interior)
		if err != nil {
			return nil, err
		}
		spawned = append(spawned, pane)
		if spec.Title != "" {
			pane.UserTitle = spec.Title
		}
		return pane, nil
	}

	root, err := layout.Unmarshal(ws.Layout, spawn)
	if err != nil {
		// Stop whatever panes spawned before the failure — a failed
		// window must not leak PTYs (or the ssh pool refs they own).
		for _, p := range spawned {
			m.stopPane(p)
		}
		return err
	}
	if root == nil {
		return fmt.Errorf("empty layout")
	}

	leaves := root.CollectLeaves()
	state := &windowState{
		ID:        session.WindowID(ws.ID),
		Number:    num,
		Title:     ws.Title,
		UserTitle: ws.UserTitle,
		Frame:     w,
		Root:      root,
		SyncInput: ws.SyncInput,
	}
	var focus *layout.PaneNode
	switch {
	case ws.FocusIndex >= 0 && ws.FocusIndex < len(leaves):
		focus = leaves[ws.FocusIndex]
	case len(leaves) > 0:
		focus = leaves[0]
	}
	// Zoomed is 1-based (0 = none); restore it if the index is still valid.
	if ws.Zoomed > 0 && ws.Zoomed <= len(leaves) && leaves[ws.Zoomed-1].Pane != nil {
		id := leaves[ws.Zoomed-1].Pane.ID
		state.Zoomed = &id
		// Zoom is a focus state: the visible leaf is also the command target.
		focus = leaves[ws.Zoomed-1]
	}

	body := layout.Materialize(root, interior, state.Zoomed)
	w.Insert(body)

	m.registerWindow(w, state)
	m.setPaneFocus(state, focus)
	return nil
}
