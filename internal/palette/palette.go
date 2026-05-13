// Package palette implements fvmux's Ctrl-G P command palette: a
// modal fuzzy-search popup over every registered, non-hidden command.
package palette

import (
	"fmt"
	"strings"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"

	"github.com/oldwired/fvmux/internal/commands"
)

// Position names the supported palette placements.
type Position string

const (
	PosCenter    Position = "center"
	PosTopLeft   Position = "top-left"
	PosTopCenter Position = "top-center"
	PosTopRight  Position = "top-right"
)

// ParsePosition is forgiving: empty / unknown → PosCenter.
func ParsePosition(s string) Position {
	switch Position(s) {
	case PosTopLeft, PosTopCenter, PosTopRight:
		return Position(s)
	}
	return PosCenter
}

// Show opens the palette as a modal picker over the visible commands
// in reg, blocking until the user picks one or cancels. If the chosen
// command is enabled, its Action(ctx) fires synchronously. Position
// controls where the popup lands (see [appearance] palette_position).
func Show(a *fvapp.Application, reg *commands.Registry, ctx *commands.Ctx, pos Position) {
	cmds := visibleCommands(reg)
	if len(cmds) == 0 {
		return
	}
	items := make([]string, len(cmds))
	for i, c := range cmds {
		items[i] = formatRow(c)
	}

	desk := a.Desktop.BaseView()
	cols, rows := desk.Size.X, desk.Size.Y
	w, h := 64, 14
	if w > cols-4 {
		w = cols - 4
	}
	if h > rows-2 {
		h = rows - 2
	}
	x, y := placement(pos, cols, rows, w, h)
	bounds := geom.NewRect(x, y, x+w, y+h)

	ff := fuzzyfinder.New(bounds, items)
	idx := ff.Run(&a.Desktop.Group)
	if idx < 0 || idx >= len(cmds) {
		return
	}
	c := cmds[idx]
	if c.Enabled != nil && !c.Enabled(ctx) {
		return
	}
	if c.Action != nil {
		c.Action(ctx)
	}
}

// placement returns the top-left corner for a popup of (w, h) on a
// desktop of (cols, rows), honouring pos. Top variants pin near the
// menu bar (y=1) with a small inset; center stays in the middle.
func placement(pos Position, cols, rows, w, h int) (int, int) {
	const inset = 2
	var x, y int
	switch pos {
	case PosTopLeft:
		x, y = inset, 1
	case PosTopCenter:
		x, y = (cols-w)/2, 1
	case PosTopRight:
		x, y = cols-w-inset, 1
	default: // PosCenter
		x, y = (cols-w)/2, (rows-h)/2
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+w > cols {
		x = cols - w
	}
	if y+h > rows {
		y = rows - h
	}
	return x, y
}

// VisibleCommands returns the non-Hidden commands from reg in
// stable-ID order — the exact set Show displays.
func VisibleCommands(reg *commands.Registry) []*commands.Command {
	all := reg.All()
	out := make([]*commands.Command, 0, len(all))
	for _, c := range all {
		if c.Hidden {
			continue
		}
		out = append(out, c)
	}
	return out
}

func visibleCommands(reg *commands.Registry) []*commands.Command {
	return VisibleCommands(reg)
}

func formatRow(c *commands.Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%-12s] %-32s", c.Category, c.Name)
	if c.Chord != "" {
		b.WriteString("  ")
		b.WriteString(c.Chord)
	}
	return b.String()
}
