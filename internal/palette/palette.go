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

// Options bundles palette inputs that aren't covered by the basic
// fuzzy-find behaviour. The original Show takes the same arguments
// it always did; ShowWithOptions exposes MRU + persist callback.
type Options struct {
	Pos        Position
	MRU        []uint16                // recently-used command IDs, newest first.
	Persist    func(newMRU []uint16)   // called after a successful pick.
	Special    []SpecialEntry          // optional synthetic entries (`:`, `@`, `#`, `?`).
	OnDisabled func(*commands.Command) // optional explanation surface.
}

// SpecialEntry is a top-of-list pseudo-command. Selecting one runs
// Action(ctx); these don't go through commands.Registry.
type SpecialEntry struct {
	Label  string
	Action func(*commands.Ctx)
}

// Show is the original convenience entry — MRU disabled. Used by
// callers that don't have access to state.toml.
func Show(a *fvapp.Application, reg *commands.Registry, ctx *commands.Ctx, pos Position) {
	ShowWithOptions(a, reg, ctx, Options{Pos: pos})
}

// ShowWithOptions opens the palette with full control over MRU + the
// synthetic-entry strip. The user picks; if it's a regular command,
// reg.ByID dispatches; if it's a synthetic entry, its Action runs.
func ShowWithOptions(a *fvapp.Application, reg *commands.Registry, ctx *commands.Ctx, opts Options) {
	cmds := orderCommands(visibleCommands(reg), opts.MRU)
	if len(cmds) == 0 && len(opts.Special) == 0 {
		return
	}

	// Build the on-screen items: synthetic entries first, commands after.
	items := make([]string, 0, len(opts.Special)+len(cmds))
	for _, s := range opts.Special {
		items = append(items, s.Label)
	}
	for _, c := range cmds {
		items = append(items, formatRow(c, ctx))
	}

	desk := a.Desktop.BaseView()
	cols, rows := desk.Size.X, desk.Size.Y
	w, h := 70, 16
	if w > cols-4 {
		w = cols - 4
	}
	if h > rows-2 {
		h = rows - 2
	}
	x, y := placement(opts.Pos, cols, rows, w, h)
	bounds := geom.NewRect(x, y, x+w, y+h)

	ff := fuzzyfinder.New(bounds, items)
	idx := ff.Run(&a.Desktop.Group)
	if idx < 0 {
		return
	}
	if idx < len(opts.Special) {
		if a := opts.Special[idx].Action; a != nil {
			a(ctx)
		}
		return
	}
	cmd := cmds[idx-len(opts.Special)]
	if cmd.Enabled != nil && !cmd.Enabled(ctx) {
		if opts.OnDisabled != nil {
			opts.OnDisabled(cmd)
		}
		return
	}
	if cmd.Action != nil {
		cmd.Action(ctx)
	}
	if opts.Persist != nil {
		opts.Persist(MRUBump(opts.MRU, cmd.ID))
	}
}

// orderCommands returns visible commands with MRU entries moved to
// the front (in MRU order, newest first), followed by the remaining
// commands in their original order.
func orderCommands(all []*commands.Command, mru []uint16) []*commands.Command {
	if len(mru) == 0 {
		return all
	}
	byID := make(map[uint16]*commands.Command, len(all))
	for _, c := range all {
		byID[c.ID] = c
	}
	out := make([]*commands.Command, 0, len(all))
	seen := make(map[uint16]bool, len(all))
	for _, id := range mru {
		if c, ok := byID[id]; ok && !seen[id] {
			out = append(out, c)
			seen[id] = true
		}
	}
	for _, c := range all {
		if !seen[c.ID] {
			out = append(out, c)
		}
	}
	return out
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

// formatRow renders one command row. Commands whose Enabled predicate
// vetoes them are tagged "[disabled]" so the user sees they exist but
// can't pick them right now — picking is then a no-op above.
func formatRow(c *commands.Command, ctx *commands.Ctx) string {
	var b strings.Builder
	prefix := "          "
	if c.Enabled != nil && !c.Enabled(ctx) {
		prefix = "[disabled]"
	}
	fmt.Fprintf(&b, "%s [%-12s] %-32s", prefix, c.Category, c.Name)
	if c.Chord != "" {
		b.WriteString("  ")
		b.WriteString(c.Chord)
	}
	return b.String()
}
