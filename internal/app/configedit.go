package app

import (
	"fmt"
	"os"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/editor"
)

// openConfigFile opens path in a modal editor dialog. Missing files
// are created empty so the user can write a fresh config from scratch.
// Save commits via Save button (Enter) and writes back to disk; Cancel
// (Esc) discards. Shows the default "Saved <path>" confirmation.
func (m *Mux) openConfigFile(title, path string) {
	m.openConfigFileWithReload(title, path, nil)
}

// Wire helpers used by the four File-menu items.
func (m *Mux) openConfig()      { m.openConfigFile("config", m.Opts.Paths.ConfigFile()) }
func (m *Mux) openProfiles()    { m.openConfigFile("profiles", m.Opts.Paths.ProfilesFile()) }
func (m *Mux) openKeybindings() { m.openConfigFile("keybindings", m.Opts.Paths.KeybindingsFile()) }
func (m *Mux) openHostsEditor() { m.openConfigFile("hosts", m.Opts.Paths.HostsFile()) }

// openConfigFileWithReload is the canonical editor entry. onSave (when
// non-nil) replaces the default "Saved <path>" msgbox — the theme
// editor uses this to fire a live reload instead.
//
// All child views carry GrowMode flags so the editor follows the
// dialog when the user resizes — the editor body stretches in both
// directions, the scrollbar tracks the right edge, and the hint +
// buttons stay pinned to the bottom-right corner.
func (m *Mux) openConfigFileWithReload(title, path string, onSave func()) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
				"Couldn't create %s:\n%s",
				[]any{path, err.Error()}, msgbox.OKOnly)
			return
		}
	}

	desk := m.App.Desktop.BaseView()
	w := 90
	h := 26
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	// Minimum 50×12 — enough for the buttons not to overlap and the
	// editor to still show ~6 rows × ~46 cols of content.
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), title+" — "+path)
	d.SetSizeLimits(geom.Point{X: 50, Y: 12}, geom.Point{})

	// Editor body — sits at (1,1) and stretches to fill the interior
	// minus the bottom hint + button row. The scrollbar lives just
	// inside the right edge of that same band.
	editorRect := geom.NewRect(1, 1, w-2, h-3)
	vbar := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-3))
	vbar.GrowMode = consts.GfGrowLoX | consts.GfGrowHiX | consts.GfGrowHiY
	d.Insert(vbar)

	ed := editor.New(editorRect, nil, vbar)
	if err := ed.LoadFile(path); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't read %s:\n%s",
			[]any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	ed.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	d.Insert(ed)

	// Hint pinned to the bottom band, stretching with the dialog's
	// width. Hint text varies by mode: reload-style editors say so.
	keyHint := "Enter / Save: save and close · Esc: discard"
	if onSave != nil {
		keyHint = "Save commits + reloads · Esc discards"
	}
	// Initial position read via Position() (always valid) so the hint
	// already shows L:C 1:1 before the user touches anything.
	line, col := ed.Position()
	hint := dialogs.NewStaticText(
		geom.NewRect(2, h-3, w-2, h-2),
		formatEditorHint(line, col, keyHint))
	hint.GrowMode = consts.GfGrowLoY | consts.GfGrowHiY | consts.GfGrowHiX
	d.Insert(hint)
	// OnCursorMove fires once per Draw when (line, col) changes. We
	// mutate the hint's Text in place — the hint will repaint on the
	// next dialog draw cycle that the editor's own MarkDirty kicks
	// off (every cursor movement triggers a redraw anyway).
	ed.OnCursorMove = func(line, col int) {
		hint.Text = formatEditorHint(line, col, keyHint)
	}

	// Buttons pinned to the bottom-right corner — GfGrowAll shifts
	// every corner by the parent's delta, which for an x-only / y-only
	// resize keeps them flush to (w-2, h-1).
	cancelBtn := dialogs.NewButton(
		geom.NewRect(w-26, h-2, w-16, h-1),
		"~C~ancel", consts.CmCancel, 0,
	)
	cancelBtn.GrowMode = consts.GfGrowAll
	d.Insert(cancelBtn)

	saveBtn := dialogs.NewButton(
		geom.NewRect(w-12, h-2, w-2, h-1),
		"~S~ave", consts.CmOK, dialogs.BfDefault,
	)
	saveBtn.GrowMode = consts.GfGrowAll
	d.Insert(saveBtn)

	if m.App.Desktop.ExecView(d) != consts.CmOK {
		return
	}
	if err := ed.SaveFile(path); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't write %s:\n%s",
			[]any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	if onSave != nil {
		onSave()
		return
	}
	msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
		"Saved %s\n\n(Some changes apply on restart.)",
		[]any{path}, msgbox.OKOnly)
}

// formatEditorHint renders the bottom-row text with the current
// cursor position on the left followed by the key hints. 1-indexed
// to match the editor's Position() convention.
func formatEditorHint(line, col int, keyHint string) string {
	return fmt.Sprintf("L:C %d:%d  ·  %s", line, col, keyHint)
}
