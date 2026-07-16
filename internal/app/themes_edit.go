package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"

	"github.com/oldwired/fvmux/internal/atomicfile"
	"github.com/oldwired/fvmux/internal/ui"
)

// editThemes is the View → Edit Themes… entry. Lists every TOML in the
// themes dir (and the disabled example template) plus a "Create new
// theme…" row. Picking a theme opens it in the embedded editor; saving
// triggers a theme reload so live preview catches up.
func (m *Mux) editThemes() {
	dir := m.Opts.Paths.ThemesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't create %s:\n%s",
			[]any{dir, err.Error()}, msgbox.OKOnly)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't list themes:\n%s", []any{err.Error()}, msgbox.OKOnly)
		return
	}

	type row struct {
		label string
		path  string
		isNew bool
	}
	var rows []row
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") && !strings.HasSuffix(name, ".toml.disabled") {
			continue
		}
		rows = append(rows, row{
			label: "edit: " + name,
			path:  filepath.Join(dir, name),
		})
	}
	rows = append(rows, row{label: "+  Create new theme…", isNew: true})

	items := make([]string, len(rows))
	for i, r := range rows {
		items[i] = r.label
	}
	desk := m.App.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 60, 16, 4)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	ff := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), items)
	idx := ff.Run(&m.App.Desktop.Group)
	if idx < 0 || idx >= len(rows) {
		return
	}
	if rows[idx].isNew {
		m.newThemeFlow(dir)
		return
	}
	m.openConfigFileWithReload("theme", rows[idx].path, m.ReloadThemes)
}

// newThemeFlow prompts for a theme name, copies the example template
// (or a minimal skeleton if it's missing) into themes/<name>.toml,
// opens the editor, and reloads on save.
func (m *Mux) newThemeFlow(dir string) {
	name, ok := promptString(m.App, "New theme", "Theme name (alphanumeric):", "")
	if !ok {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if !validThemeName(name) {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Invalid theme name %q.\nUse letters, digits, '-', '_' only.",
			[]any{name}, msgbox.OKOnly)
		return
	}
	path := filepath.Join(dir, name+".toml")
	if _, err := os.Stat(path); err == nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Theme %q already exists. Edit it instead.",
			[]any{name}, msgbox.OKOnly)
		return
	}

	body := themeSkeleton(name, dir)
	if err := atomicfile.Write(path, []byte(body), 0o644); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't create %s:\n%s", []any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	m.openConfigFileWithReload("theme", path, m.ReloadThemes)
}

// themeSkeleton returns the starter body for a new theme TOML. Prefers
// the seeded example.toml.disabled if it's present (so users see the
// full schema with comments); otherwise emits a minimal fallback.
func themeSkeleton(name, dir string) string {
	if data, err := os.ReadFile(filepath.Join(dir, "example.toml.disabled")); err == nil {
		// Rewrite the example's `name = "example"` line so the new
		// theme has the user's chosen name out of the gate.
		body := string(data)
		body = strings.Replace(body,
			`name = "example"`, `name = "`+name+`"`, 1)
		return body
	}
	return "# fvmux theme — see ~/.config/fvmux/themes/example.toml.disabled\n" +
		"name = \"" + name + "\"\n" +
		"tagline = \"\"\n\n" +
		"[palette]\n" +
		"# Add per-field overrides here.\n"
}

func validThemeName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
