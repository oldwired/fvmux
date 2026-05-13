// Package theme wraps fv-go's theme.Palette with fvmux's named-theme
// registry. A Theme is one palette plus a name and tagline.
package theme

import (
	fvtheme "github.com/oldwired/fv-go/pkg/fv/theme"
	"github.com/oldwired/fv-go/pkg/fv/types"
)

// Theme bundles a Palette with the metadata fvmux's Ctrl-G T picker
// uses to label its rows.
type Theme struct {
	Name    string
	Tagline string
	Palette *fvtheme.Palette
}

// Apply installs t's palette as fv-go's active theme.
func (t *Theme) Apply() {
	if t == nil || t.Palette == nil {
		return
	}
	fvtheme.Set(t.Palette)
}

// Builtins returns fvmux's three baked-in themes (slate, tokyonight-ish,
// solarbeach). Each clones fvtheme.Default and tweaks a handful of
// accent colours so the picker actually shows a visible difference.
func Builtins() []*Theme {
	return []*Theme{
		slate(),
		tokyonightIsh(),
		solarbeach(),
	}
}

// Find returns the theme with the given name from list, or nil.
func Find(list []*Theme, name string) *Theme {
	for _, t := range list {
		if t.Name == name {
			return t
		}
	}
	return nil
}

func slate() *Theme {
	p := clonePalette(fvtheme.Default)
	return &Theme{
		Name:    "slate",
		Tagline: "the one you'll forget you chose, in a good way.",
		Palette: p,
	}
}

func tokyonightIsh() *Theme {
	p := clonePalette(fvtheme.Default)
	// Cyan splitters, magenta active frame as a visual cue.
	p.FrameActive = types.MakeAttr(0x0D, 0x01) // bright magenta on dark blue
	p.FrameNormal = types.MakeAttr(0x07, 0x01) // gray on dark blue
	p.WindowBackground = types.MakeAttr(0x07, 0x01)
	p.SplitterBar = types.MakeAttr(0x0B, 0x01)    // bright cyan
	p.SplitterHandle = types.MakeAttr(0x0E, 0x01) // yellow
	p.DesktopBackground = types.MakeAttr(0x08, 0x00)
	return &Theme{
		Name:    "tokyonight-ish",
		Tagline: "borrowed warmth from a city that doesn't sleep.",
		Palette: p,
	}
}

func solarbeach() *Theme {
	p := clonePalette(fvtheme.Default)
	// Cream-ish bg, terracotta accents.
	p.WindowBackground = types.MakeAttr(0x00, 0x0F) // black on bright white
	p.FrameNormal = types.MakeAttr(0x06, 0x0F)      // brown on white
	p.FrameActive = types.MakeAttr(0x04, 0x0F)      // red on white
	p.SplitterBar = types.MakeAttr(0x06, 0x0F)
	p.SplitterHandle = types.MakeAttr(0x04, 0x0F)
	p.DesktopBackground = types.MakeAttr(0x06, 0x0E) // brown on yellow
	return &Theme{
		Name:    "solarbeach",
		Tagline: "solarized went on vacation and came back tan.",
		Palette: p,
	}
}

func clonePalette(src *fvtheme.Palette) *fvtheme.Palette {
	cp := *src
	return &cp
}
