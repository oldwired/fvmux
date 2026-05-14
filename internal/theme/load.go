package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	fvtheme "github.com/oldwired/fv-go/pkg/fv/theme"
)

// fileSchema is what users put in ~/.config/fvmux/themes/*.toml. The
// nested [palette] table mirrors a subset of fvtheme.Palette fields —
// the ones users typically tweak. Anything missing falls back to the
// fv-go default, so a theme can override only what it wants to change.
type fileSchema struct {
	Name    string         `toml:"name"`
	Tagline string         `toml:"tagline"`
	Palette paletteOverlay `toml:"palette"`
}

type paletteOverlay struct {
	FrameNormal       *uint16 `toml:"frame_normal"`
	FrameActive       *uint16 `toml:"frame_active"`
	FrameIcons        *uint16 `toml:"frame_icons"`
	WindowBackground  *uint16 `toml:"window_background"`
	WindowShadow      *uint16 `toml:"window_shadow"`
	DesktopBackground *uint16 `toml:"desktop_background"`
	SplitterBar       *uint16 `toml:"splitter_bar"`
	SplitterHandle    *uint16 `toml:"splitter_handle"`
	StatusBarNormal   *uint16 `toml:"status_bar_normal"`
	StatusBarHot      *uint16 `toml:"status_bar_hot"`
	MenuBarNormal     *uint16 `toml:"menu_bar_normal"`
	MenuBarHot        *uint16 `toml:"menu_bar_hot"`
}

// LoadFromTOML parses one theme file. Returns the assembled Theme
// (with overlay applied on top of fvtheme.Default) or an error.
func LoadFromTOML(data []byte) (*Theme, error) {
	var f fileSchema
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Name == "" {
		return nil, errors.New("theme: missing 'name' key")
	}
	p := clonePalette(fvtheme.Default)
	o := f.Palette
	if o.FrameNormal != nil {
		p.FrameNormal = *o.FrameNormal
	}
	if o.FrameActive != nil {
		p.FrameActive = *o.FrameActive
	}
	if o.FrameIcons != nil {
		p.FrameIcons = *o.FrameIcons
	}
	if o.WindowBackground != nil {
		p.WindowBackground = *o.WindowBackground
	}
	if o.WindowShadow != nil {
		p.WindowShadow = *o.WindowShadow
	}
	if o.DesktopBackground != nil {
		p.DesktopBackground = *o.DesktopBackground
	}
	if o.SplitterBar != nil {
		p.SplitterBar = *o.SplitterBar
	}
	if o.SplitterHandle != nil {
		p.SplitterHandle = *o.SplitterHandle
	}
	if o.StatusBarNormal != nil {
		p.StatusBarNormal = *o.StatusBarNormal
	}
	if o.StatusBarHot != nil {
		p.StatusBarHot = *o.StatusBarHot
	}
	if o.MenuBarNormal != nil {
		p.MenuBarNormal = *o.MenuBarNormal
	}
	if o.MenuBarHot != nil {
		p.MenuBarHot = *o.MenuBarHot
	}
	return &Theme{Name: f.Name, Tagline: f.Tagline, Palette: p}, nil
}

// LoadDir reads every *.toml file in dir and returns the parsed themes
// in alphabetical filename order. Files that fail to parse are skipped
// and their error is appended into err so the caller can surface them.
// Missing dir ⇒ nil, nil (no error).
func LoadDir(dir string) ([]*Theme, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".toml") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)

	var (
		out  []*Theme
		errs []string
	)
	for _, n := range names {
		data, rerr := os.ReadFile(filepath.Join(dir, n))
		if rerr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", n, rerr))
			continue
		}
		t, terr := LoadFromTOML(data)
		if terr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", n, terr))
			continue
		}
		out = append(out, t)
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("themes: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// All returns Builtins() ∪ LoadDir(dir). When a disk theme shares a
// name with a builtin, the disk version wins (users override builtins
// by saving a theme with the same name).
func All(dir string) []*Theme {
	out := Builtins()
	disk, _ := LoadDir(dir)
	for _, d := range disk {
		// Replace any builtin with the same name.
		replaced := false
		for i, b := range out {
			if b.Name == d.Name {
				out[i] = d
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, d)
		}
	}
	return out
}
