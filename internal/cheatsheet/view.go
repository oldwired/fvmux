package cheatsheet

import (
	"strings"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/ui"
)

// window is a non-modal utility window. Keeping it as its own concrete type
// prevents fvmux's terminal-window arrangement commands from tiling it.
type window struct {
	views.Window
	content *markdown.MarkdownView
	source  string
}

func newWindow(bounds geom.Rect, reg *commands.Registry) *window {
	w := &window{}
	views.InitWindow(&w.Window, bounds, "Cheatsheet (/: search, Esc: close)", 0)
	w.SetSelf(w)
	w.SetSizeLimits(geom.Point{X: 40, Y: 10}, geom.Point{})

	width, height := w.Size.X, w.Size.Y
	vbar := views.NewScrollBar(geom.NewRect(width-2, 1, width-1, height-1))
	w.Insert(vbar)

	w.content = markdown.New(geom.NewRect(1, 1, width-2, height-1), vbar)
	w.Insert(w.content)
	w.setMarkdown(Generate(reg))
	return w
}

func (w *window) GetTypeID() string { return "fvmux-cheatsheet" }

func (w *window) HandleEvent(ev *drivers.Event) {
	w.Window.HandleEvent(ev)
	if ev.What != consts.EvKeyDown {
		return
	}
	if ev.KeyCode == consts.KbEsc {
		w.Close()
		w.ClearEvent(ev)
		return
	}
	if ev.UnicodeChar == '/' {
		w.ClearEvent(ev)
		w.search()
	}
}

func (w *window) setMarkdown(source string) {
	w.source = source
	w.content.SetMarkdown(source)
}

func (w *window) search() {
	candidates := searchCandidates(w.source)
	if len(candidates) == 0 {
		return
	}

	width := w.Size.X - 4
	if width > 66 {
		width = 66
	}
	height := w.Size.Y - 4
	if height > 14 {
		height = 14
	}
	if width < 16 || height < 4 {
		return
	}
	x := (w.Size.X - width) / 2
	y := (w.Size.Y - height) / 2
	items := make([]string, len(candidates))
	for i, candidate := range candidates {
		items[i] = candidate.label
	}

	finder := fuzzyfinder.New(geom.NewRect(x, y, x+width, y+height), items)
	if selected := finder.Run(&w.Group); selected >= 0 {
		w.scrollToLine(candidates[selected].line)
	}
	w.Focus(w.content)
}

func (w *window) scrollToLine(line int) {
	top := line - 2 // Keep the selected row's section heading in view.
	if top < 0 {
		top = 0
	}
	lineCount := len(strings.Split(w.source, "\n"))
	maxTop := lineCount - w.content.Size.Y
	if maxTop < 0 {
		maxTop = 0
	}
	if top > maxTop {
		top = maxTop
	}
	w.content.Top = top
	if w.content.VScroll != nil {
		w.content.VScroll.SetValue(top)
	}
	w.content.Draw()
}

type searchCandidate struct {
	line  int
	label string
}

// searchCandidates indexes every bullet in the generated reference, not only
// registry commands. Prefixing each result with its closest heading makes
// searches such as "copy mode" or "dialog escape" useful too.
func searchCandidates(source string) []searchCandidate {
	var candidates []searchCandidate
	section := ""
	for line, raw := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			section = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		case strings.HasPrefix(trimmed, "- "):
			label := strings.TrimPrefix(trimmed, "- ")
			label = strings.ReplaceAll(label, "**", "")
			label = strings.ReplaceAll(label, "`", "")
			if section != "" {
				label = "[" + section + "] " + label
			}
			candidates = append(candidates, searchCandidate{line: line, label: label})
		}
	}
	return candidates
}

// Show opens or raises a centred, non-modal window containing the
// auto-generated cheatsheet. Esc or the title-bar close box dismisses it.
func Show(a *fvapp.Application, reg *commands.Registry) {
	for _, child := range a.Desktop.Children {
		if existing, ok := child.(*window); ok {
			existing.setMarkdown(Generate(reg))
			a.Desktop.MakeFirst(existing)
			a.Desktop.Focus(existing)
			return
		}
	}

	desk := a.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 70, 22, 4)
	a.Desktop.InsertWindow(newWindow(r, reg))
}
