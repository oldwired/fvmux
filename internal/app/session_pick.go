package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"

	"github.com/oldwired/fvmux/internal/session"
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
	// Close existing windows before loading the new layout. Snapshot
	// the order first because removeWindow mutates m.windowOrder.
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
	m.Opts.SessionName = name
	if err := m.SaveSessionSilent(); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Save failed:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
		"Saved as %s.", []any{name}, msgbox.OKOnly)
}

// renameSessionFile asks for a new name and renames the on-disk file.
// Updates m.Opts.SessionName.
func (m *Mux) renameSessionFile() {
	if m.Opts.SessionName == "" {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"No active session to rename. Start fvmux with -session=NAME first.",
			msgbox.OKOnly)
		return
	}
	newName, ok := promptString(m.App, "Rename Session",
		"New name:", m.Opts.SessionName)
	if !ok || strings.TrimSpace(newName) == "" {
		return
	}
	newName = strings.TrimSpace(newName)
	if newName == m.Opts.SessionName {
		return
	}
	oldPath := m.Opts.Paths.SessionFile(m.Opts.SessionName)
	newPath := m.Opts.Paths.SessionFile(newName)
	if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Rename failed:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}
	m.Opts.SessionName = newName
}

func (m *Mux) savedSessionNames() []string {
	dir := filepath.Join(m.Opts.Paths.Root, "sessions")
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
