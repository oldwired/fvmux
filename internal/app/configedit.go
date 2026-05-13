package app

import (
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
// (Esc) discards.
func (m *Mux) openConfigFile(title, path string) {
	if path == "" {
		return
	}
	// Ensure the file exists — Editor.LoadFile errors on ENOENT.
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
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), title+" — "+path)

	// Editor occupies the body; reserve two rows at the bottom for
	// hint + buttons.
	editorRect := geom.NewRect(1, 1, w-2, h-3)
	vbar := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-3))
	d.Insert(vbar)
	ed := editor.New(editorRect, nil, vbar)
	if err := ed.LoadFile(path); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't read %s:\n%s",
			[]any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	d.Insert(ed)
	d.Insert(dialogs.NewStaticText(
		geom.NewRect(2, h-3, w-2, h-2),
		"Enter / OK: save and close · Esc: discard",
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-12, h-2, w/2-2, h-1),
		"~S~ave", consts.CmOK, dialogs.BfDefault,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2+2, h-2, w/2+12, h-1),
		"~C~ancel", consts.CmCancel, 0,
	))

	if m.App.Desktop.ExecView(d) != consts.CmOK {
		return
	}
	if err := ed.SaveFile(path); err != nil {
		msgbox.Showf(&m.App.Desktop.Group, msgbox.Error,
			"Couldn't write %s:\n%s",
			[]any{path, err.Error()}, msgbox.OKOnly)
		return
	}
	msgbox.Showf(&m.App.Desktop.Group, msgbox.Info,
		"Saved %s\n\n(Some changes apply on restart.)",
		[]any{path}, msgbox.OKOnly)
}

// Wire helpers used by the four File-menu items.
func (m *Mux) openConfig()      { m.openConfigFile("config", m.Opts.Paths.ConfigFile()) }
func (m *Mux) openProfiles()    { m.openConfigFile("profiles", m.Opts.Paths.ProfilesFile()) }
func (m *Mux) openKeybindings() { m.openConfigFile("keybindings", m.Opts.Paths.KeybindingsFile()) }
func (m *Mux) openHostsEditor() { m.openConfigFile("hosts", m.Opts.Paths.HostsFile()) }
