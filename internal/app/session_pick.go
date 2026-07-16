package app

import (
	"os"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/sftp"
)

// openSessionPicker fuzzy-picks a saved session and loads it. Saves
// the current session first if one is in flight; otherwise just
// switches.
func (m *Mux) openSessionPicker() {
	names := m.savedSessionNames()
	if len(names) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No saved sessions found.\nUse Ctrl-G S after starting fvmux with -session=NAME.",
			msgbox.OKOnly)
		return
	}
	desk := m.App.Desktop.BaseView()
	w, h := 60, 14
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	idx := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), names).Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(names) {
		return
	}
	pick := names[idx]

	// Save current session if we have one.
	_ = m.SaveSessionSilent()

	snap, err := session.Load(m.Opts.Paths.SessionFile(pick))
	if err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't load session %s:\n%s",
			[]any{pick, err.Error()}, msgbox.OKOnly)
		return
	}
	// Close existing windows + browsers before loading the new
	// layout. Browsers are dialogs (not in windowOrder) so they need
	// the dedicated registry sweep. Also cancel any pending SFTP
	// restore polls so a stale one from the previous session doesn't
	// fire after we switch. Snapshot windowOrder first since
	// removeWindow mutates it.
	m.sftpRestore.cancelAll()
	sftp.CloseAllBrowsers()
	keys := append([]views.View(nil), m.windowOrder...)
	for _, key := range keys {
		if ws := m.windows[key]; ws != nil {
			m.removeWindow(ws)
		}
	}
	m.Opts.SessionName = pick
	if err := m.LoadSession(snap); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Loading session %s failed:\n%s",
			[]any{pick, err.Error()}, msgbox.OKOnly)
	}
}

// newSession is File → New Session: close every existing window
// (after a single confirm if any have live panes), clear the session
// name, then open one starter window from the default profile. Stays
// un-named until the user does Save (which becomes Save As) or Save
// Session As.
func (m *Mux) newSession() {
	if !m.canCloseAllWindows() {
		return
	}
	// Persist the outgoing named session first — every other close-all
	// path (quit, signal, panic, picker switch) saves; without this the
	// layout work since the last manual save is silently discarded.
	_ = m.SaveSessionSilent()
	// SFTP browsers are desktop dialogs, not windows in m.windowOrder
	// — close them via the sftp package's own registry before the
	// window loop runs. Also cancel any pending session-restore SFTP
	// polls so a stale one doesn't pop a browser after the user has
	// moved on.
	m.sftpRestore.cancelAll()
	sftp.CloseAllBrowsers()
	keys := append([]views.View(nil), m.windowOrder...)
	for _, key := range keys {
		if ws := m.windows[key]; ws != nil {
			m.removeWindow(ws)
		}
	}
	m.Opts.SessionName = ""
	if _, err := m.NewWindow(""); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't open starter window:\n%s",
			[]any{err.Error()}, msgbox.OKOnly)
	}
}

// canCloseAllWindows shows a single confirm dialog when any window
// has live panes. Returns true to proceed, false to abort.
func (m *Mux) canCloseAllWindows() bool {
	alive := 0
	for _, ws := range m.windows {
		if ws == nil || ws.Root == nil {
			continue
		}
		ws.Root.Leaves(func(l *layout.PaneNode) {
			if l.Pane != nil && !l.Pane.Dead {
				alive++
			}
		})
	}
	if alive == 0 {
		return true
	}
	if !m.Opts.Config.General.ConfirmKill {
		return true
	}
	body := "Close every window and start a fresh session?"
	got := msgbox.Show(&m.App.Desktop.Group, msgbox.Question, body, msgbox.YesNo)
	return got == consts.CmYes
}

// saveSessionAs prompts for a new name and writes the current snapshot
// there. Updates m.Opts.SessionName so subsequent Save / autosave hits
// the new file.
func (m *Mux) saveSessionAs() {
	name, ok := promptString(m.App, "Save Session As",
		"Session name:", m.Opts.SessionName)
	if !ok || strings.TrimSpace(name) == "" {
		return
	}
	name = strings.TrimSpace(name)
	if err := config.ValidSessionName(name); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Invalid session name:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	m.Opts.SessionName = name
	if err := m.SaveSessionSilent(); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Save failed:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
		"Saved as %s.", []any{name}, msgbox.OKOnly)
}

// renameSession (Ctrl-G $) renames the current named session: the file
// on disk moves and the live name follows, so autosaves land in the new
// file and the old name stops haunting the session picker. An un-named
// session falls through to Save As — there's nothing on disk to rename.
func (m *Mux) renameSession() {
	if m.Opts.SessionName == "" {
		m.saveSessionAs()
		return
	}
	old := m.Opts.SessionName
	name, ok := promptString(m.App, "Rename Session", "New name:", old)
	if !ok || strings.TrimSpace(name) == "" {
		return
	}
	name = strings.TrimSpace(name)
	if name == old {
		return
	}
	if err := config.ValidSessionName(name); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Invalid session name:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	newPath := m.Opts.Paths.SessionFile(name)
	if _, err := os.Stat(newPath); err == nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"A session named %q already exists.", []any{name}, msgbox.OKOnly)
		return
	}
	oldPath := m.Opts.Paths.SessionFile(old)
	if _, err := os.Stat(oldPath); err == nil {
		if err := os.Rename(oldPath, newPath); err != nil {
			msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
				"Rename failed:\n%s", []any{err.Error()}, msgbox.OKOnly)
			return
		}
	}
	m.Opts.SessionName = name
	_ = m.SaveSessionSilent()
	m.refreshStatusBar()
}

// deleteSession (File → Delete Session…) removes a saved session file
// picked from the list, after a confirm. Deleting the session that's
// currently open only removes the file — the live windows stay, and
// the name is cleared so the next Save prompts instead of silently
// recreating the deleted file.
func (m *Mux) deleteSession() {
	names := m.savedSessionNames()
	if len(names) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No saved sessions found.", msgbox.OKOnly)
		return
	}
	desk := m.App.Desktop.BaseView()
	w, h := 60, 14
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	idx := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), names).Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(names) {
		return
	}
	pick := names[idx]
	got := msgbox.Showf(&m.App.Desktop.Group, msgbox.Question,
		"Delete saved session %q?\nOpen windows are unaffected.",
		[]any{pick}, msgbox.YesNo)
	if got != consts.CmYes {
		return
	}
	if err := os.Remove(m.Opts.Paths.SessionFile(pick)); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Delete failed:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	if m.Opts.SessionName == pick {
		m.Opts.SessionName = ""
		m.refreshStatusBar()
	}
}

func (m *Mux) savedSessionNames() []string {
	dir := m.Opts.Paths.SessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".toml"))
	}
	return out
}
