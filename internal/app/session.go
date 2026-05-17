package app

import (
	"fmt"
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
		snap.Windows = append(snap.Windows, &session.WindowSnapshot{
			ID:        uint64(ws.ID),
			Number:    ws.Number,
			Title:     ws.Title,
			UserTitle: ws.UserTitle,
			Pos: session.RectTOML{
				X: bounds.A.X, Y: bounds.A.Y,
				W: bounds.Width(), H: bounds.Height(),
			},
			Layout: layout.Marshal(ws.Root),
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
	for _, ws := range snap.Windows {
		bounds := geom.NewRect(ws.Pos.X, ws.Pos.Y,
			ws.Pos.X+ws.Pos.W, ws.Pos.Y+ws.Pos.H)
		if err := m.openSnapshotWindow(ws, bounds); err != nil {
			return fmt.Errorf("window %s: %w", ws.Title, err)
		}
	}
	if snap.Active >= 0 && snap.Active < len(m.windowOrder) {
		m.App.Desktop.Focus(m.windowOrder[snap.Active])
	}
	// SFTP browsers — schedule each. The restored ssh pane (if any)
	// for the same alias is already up and running; we just poll for
	// the master socket and open SFTP non-interactively when it
	// appears. If auth never completes within the timeout, the browser
	// silently doesn't open.
	for _, alias := range snap.SFTPAliases {
		alias := alias
		m.scheduleSftpRestore(alias)
	}
	return nil
}

// resolveProfileFallback fires when a saved pane's Profile name isn't
// a registered profile. The common case is ssh sessions opened via
// Ctrl-G H, whose Pane.Profile == the host alias. We look that alias
// up in hosts.toml + ~/.ssh/config; on a hit, synthesize an ssh
// command (with ControlOpts so the master is reused). Otherwise fall
// back to the default shell profile.
func (m *Mux) resolveProfileFallback(name string) *profile.Profile {
	if name != "" && m.sshPool != nil {
		if hosts, _ := sshmgr.Load(m.Opts.Paths.HostsFile()); len(hosts) > 0 {
			for _, h := range hosts {
				if h != nil && h.Alias == name {
					sock := m.sshPool.Acquire(h.Alias)
					args := append([]string{}, sshmgr.ControlOpts(sock)...)
					args = append(args, h.Alias)
					return &profile.Profile{
						Name:    h.Alias,
						Command: "ssh",
						Args:    args,
						Title:   h.Alias,
					}
				}
			}
		}
	}
	return profile.Defaults()[0]
}

func (m *Mux) openSnapshotWindow(ws *session.WindowSnapshot, bounds geom.Rect) error {
	w := views.NewWindow(bounds, ws.Title, ws.Number)
	interior := windowInterior(w)

	spawn := func(spec layout.LeafSpec) (*session.Pane, error) {
		prof := profile.Find(m.Opts.Profiles, spec.Profile)
		if prof == nil {
			// Profile name not in profiles.toml — see if it's an
			// SSH host alias from hosts.toml / ~/.ssh/config and
			// synthesize an inline ssh profile if so. Without this
			// fallback, restored ssh panes silently turn into
			// generic shells.
			prof = m.resolveProfileFallback(spec.Profile)
		}
		pane, err := profile.Instantiate(prof, interior, m.Opts.Config.Terminal.ScrollbackLines, m.Opts.Config.Terminal.Shell)
		if err != nil {
			return nil, err
		}
		if spec.Title != "" {
			pane.Title = spec.Title
		}
		m.wireTerminalCallbacks(pane, w)
		return pane, nil
	}

	root, err := layout.Unmarshal(ws.Layout, spawn)
	if err != nil {
		return err
	}
	if root == nil {
		return fmt.Errorf("empty layout")
	}

	leaves := root.CollectLeaves()
	state := &windowState{
		ID:        session.WindowID(ws.ID),
		Number:    ws.Number,
		Title:     ws.Title,
		UserTitle: ws.UserTitle,
		Frame:     w,
		Root:      root,
	}
	if len(leaves) > 0 {
		state.Focus = leaves[0]
	}

	body := layout.Materialize(root, interior, nil)
	w.Insert(body)

	m.registerWindow(w, state)
	if state.Focus != nil && state.Focus.Pane != nil {
		focusTerminalPath(w, state.Focus.Pane.Term)
	}
	// Apply user-set caption immediately so restored sessions display
	// their custom names even before the shell emits an OSC title.
	m.refreshWindowTitle(state)
	return nil
}
