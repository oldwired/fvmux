package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	fvtheme "github.com/oldwired/fv-go/pkg/fv/theme"
)

// fileSchema is what users put in ~/.config/fvmux/themes/*.toml. The
// nested [palette] table accepts the snake_case form of *any*
// fvtheme.Palette field (frame_normal, menu_box_selected_hot, …), each
// a uint16 attribute code. Anything omitted falls back to the fv-go
// default, so a theme overrides only what it wants. Unknown keys are
// reported as errors so a typo'd field name doesn't silently no-op.
type fileSchema struct {
	Name    string           `toml:"name"`
	Tagline string           `toml:"tagline"`
	Palette map[string]int64 `toml:"palette"`
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
	if err := applyOverlay(p, f.Palette); err != nil {
		return nil, fmt.Errorf("theme %q: %w", f.Name, err)
	}
	return &Theme{Name: f.Name, Tagline: f.Tagline, Palette: p}, nil
}

// applyOverlay sets each overlay key on p by matching its snake_case name
// to a uint16 field of fvtheme.Palette via reflection. Unknown keys and
// out-of-range values are collected and returned as a single error, so a
// theme file surfaces every mistake at once rather than one per reload.
func applyOverlay(p *fvtheme.Palette, overlay map[string]int64) error {
	if len(overlay) == 0 {
		return nil
	}
	rv := reflect.ValueOf(p).Elem()
	rt := rv.Type()
	index := make(map[string]int, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		if rt.Field(i).Type.Kind() == reflect.Uint16 {
			index[toSnakeCase(rt.Field(i).Name)] = i
		}
	}

	var bad []string
	for key, val := range overlay {
		i, ok := index[key]
		if !ok {
			bad = append(bad, fmt.Sprintf("unknown palette key %q", key))
			continue
		}
		if val < 0 || val > 65535 {
			bad = append(bad, fmt.Sprintf("palette %q = %d is out of range 0..65535", key, val))
			continue
		}
		rv.Field(i).SetUint(uint64(val))
	}
	if len(bad) > 0 {
		sort.Strings(bad) // deterministic ordering for stable messages/tests.
		return errors.New(strings.Join(bad, "; "))
	}
	return nil
}

// toSnakeCase converts an exported Go field name to the lower_snake_case
// form used as the TOML key (FrameNormal → frame_normal). Palette field
// names contain no acronym runs, so a simple per-capital split suffices.
func toSnakeCase(name string) string {
	var b strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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
