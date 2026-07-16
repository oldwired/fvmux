package theme

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	fvtheme "github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/ui"
)

// PickLive opens a small modal picker that applies the focused theme
// live on every cursor move. Enter accepts the highlighted choice
// (returns its index in themes); Esc reverts to whatever palette was
// active when the picker opened and returns -1.
//
// Used by both Ctrl-G T (the theme picker chord) and the first-run
// wizard's theme step — keeping one preview implementation means the
// two surfaces stay in sync.
func PickLive(a *fvapp.Application, themes []*Theme, initial string) int {
	if len(themes) == 0 {
		return -1
	}
	cursor := 0
	for i, t := range themes {
		if t.Name == initial {
			cursor = i
			break
		}
	}

	desk := a.Desktop.BaseView()
	w := 40
	h := len(themes) + 4
	if h > 14 {
		h = 14
	}
	// Previously this picker clamped nothing and could overflow a tiny
	// terminal; give it the standard centre-and-clamp.
	r := ui.CenterRect(desk.Size, w, h, 2)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()

	p := &livePicker{
		Base:    views.NewBase(geom.NewRect(x, y, x+w, y+h)),
		themes:  themes,
		cursor:  cursor,
		initial: clonePalette(fvtheme.Get()),
		chosen:  -2,
	}
	p.Options |= consts.OfPreProcess
	p.SetSelf(p)
	p.State |= consts.SfVisible | consts.SfExposed
	p.apply() // initial preview.

	a.Desktop.Insert(p)
	defer a.Desktop.Delete(p)

	q := views.GetEventQueue()
	if q == nil {
		fvtheme.Set(p.initial)
		return -1
	}
	for p.chosen == -2 {
		if pump := views.GetPump(); pump != nil {
			pump()
		}
		ev, ok := q.Get()
		if !ok {
			if wait := views.GetWait(); wait != nil {
				wait()
			}
			continue
		}
		p.handle(&ev)
		views.MarkDirty()
	}
	if p.chosen == -1 {
		fvtheme.Set(p.initial)
	}
	return p.chosen
}

// livePicker is the modal view PickLive runs. Internal — callers
// use PickLive.
type livePicker struct {
	views.Base
	themes  []*Theme
	cursor  int
	initial *fvtheme.Palette
	chosen  int // -2 = still running, -1 = cancelled, else = picked index.
}

// GetTypeID for serial registry.
func (p *livePicker) GetTypeID() string { return "theme-livepicker" }

// apply sets fv-go's active palette to the cursor's theme.
func (p *livePicker) apply() {
	if p.cursor >= 0 && p.cursor < len(p.themes) {
		p.themes[p.cursor].Apply()
	}
}

func (p *livePicker) handle(ev *drivers.Event) {
	if ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbEsc:
		p.chosen = -1
	case consts.KbEnter:
		p.chosen = p.cursor
	case consts.KbUp:
		if p.cursor > 0 {
			p.cursor--
			p.apply()
		}
	case consts.KbDown:
		if p.cursor+1 < len(p.themes) {
			p.cursor++
			p.apply()
		}
	}
	ev.Clear()
}

// Draw paints frame + theme list + footer hint. Re-uses fv-go's
// PopupMenu palette so the picker reads as part of the family.
func (p *livePicker) Draw() {
	pal := fvtheme.Get()
	w, h := p.Size.X, p.Size.Y
	frame := pal.PopupMenuFrame
	normal := pal.PopupMenuNormal
	sel := pal.PopupMenuSelected

	// Top border with title.
	top := screen.MakeDrawBuffer(w)
	screen.DrawCell(top, 0, "┌", frame)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(top, i, "─", frame)
	}
	screen.DrawCell(top, w-1, "┐", frame)
	screen.DrawStr(top, 2, " Themes — live preview ", frame)
	p.WriteLine(0, 0, w, 1, top)

	// Body rows.
	rows := h - 3 // top + two footer rows
	visStart := 0
	if p.cursor >= rows {
		visStart = p.cursor - rows + 1
	}
	for r := 0; r < rows; r++ {
		row := screen.MakeDrawBuffer(w)
		idx := visStart + r
		c := normal
		if idx == p.cursor {
			c = sel
		}
		screen.DrawCell(row, 0, "│", frame)
		for x := 1; x < w-1; x++ {
			screen.DrawCell(row, x, " ", c)
		}
		screen.DrawCell(row, w-1, "│", frame)
		if idx >= 0 && idx < len(p.themes) {
			t := p.themes[idx]
			label := t.Name
			if t.Tagline != "" && len(label)+3+len(t.Tagline) < w-3 {
				label = t.Name + " — " + t.Tagline
			}
			if len(label) > w-4 {
				label = label[:w-4]
			}
			screen.DrawStr(row, 2, label, c)
		}
		p.WriteLine(0, 1+r, w, 1, row)
	}

	// Footer hint.
	hint := screen.MakeDrawBuffer(w)
	screen.DrawCell(hint, 0, "│", frame)
	for x := 1; x < w-1; x++ {
		screen.DrawCell(hint, x, " ", normal)
	}
	screen.DrawCell(hint, w-1, "│", frame)
	screen.DrawStr(hint, 2, "↑↓ preview · Enter accept · Esc revert", normal)
	p.WriteLine(0, h-2, w, 1, hint)

	// Bottom border.
	bot := screen.MakeDrawBuffer(w)
	screen.DrawCell(bot, 0, "└", frame)
	for i := 1; i < w-1; i++ {
		screen.DrawCell(bot, i, "─", frame)
	}
	screen.DrawCell(bot, w-1, "┘", frame)
	p.WriteLine(0, h-1, w, 1, bot)
}
