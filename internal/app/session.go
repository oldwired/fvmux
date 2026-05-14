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
)

// SaveSession captures the current window/layout state to TOML under
// the configured paths. No-op (with a status flash) when no session
// name is set.
func (m *Mux) SaveSession() {
	if m.Opts.SessionName == "" {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No session name set. Pass -session=NAME on the command line.",
			msgbox.OKOnly)
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
	cur := m.App.Desktop.Current()
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

// LoadSession restores a snapshot, creating one window per snapshot
// entry and spawning panes via the configured profiles. Existing windows
// are left untouched — callers typically invoke LoadSession before any
// NewWindow is called.
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
	return nil
}

func (m *Mux) openSnapshotWindow(ws *session.WindowSnapshot, bounds geom.Rect) error {
	w := views.NewWindow(bounds, ws.Title, ws.Number)
	interior := windowInterior(w)

	spawn := func(spec layout.LeafSpec) (*session.Pane, error) {
		prof := profile.Find(m.Opts.Profiles, spec.Profile)
		if prof == nil {
			prof = profile.Defaults()[0]
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
