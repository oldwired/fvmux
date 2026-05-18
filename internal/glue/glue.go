// Package glue installs the shell wrapper that lets users run fvmux
// inside a private tmux session for detach/reattach.
//
// The wrapper script (fvmuxa) and its tmux config (fvmux.tmux.conf)
// are embedded into the fvmux binary so the first-run wizard can
// drop them onto disk without any external assets. The Makefile
// target `make install-glue` performs the same install non-interactively
// from a source checkout; both code paths read the same files.
//
// Layout when installed (with the defaults from DefaultLocations):
//
//	~/.local/bin/fvmuxa                    -- bash wrapper (unix)
//	~/.local/bin/fvmuxa.cmd                -- cmd wrapper  (Windows)
//	<confDir>/fvmux.tmux.conf              -- tmux config the wrappers load
//
// Install is idempotent: existing files are left alone so user edits
// to fvmux.tmux.conf survive re-runs of the wizard.
package glue

import (
	"embed"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/oldwired/fvmux/internal/atomicfile"
)

//go:embed fvmuxa fvmux.tmux.conf fvmuxa.cmd
var files embed.FS

// Locations names where each glue file should live on disk. BinDir
// holds the wrapper(s); ConfDir holds fvmux.tmux.conf and is typically
// fvmux's own config root so the wrapper's default lookup path works
// without any env vars set.
type Locations struct {
	BinDir  string
	ConfDir string
}

// DefaultLocations returns ~/.local/bin for the wrapper and the
// supplied confDir for fvmux.tmux.conf. confDir is normally
// config.Paths.Root, so the conf ends up next to config.toml.
func DefaultLocations(confDir string) Locations {
	home, _ := os.UserHomeDir()
	return Locations{
		BinDir:  filepath.Join(home, ".local", "bin"),
		ConfDir: confDir,
	}
}

type target struct {
	src  string
	dst  string
	perm os.FileMode
}

// targets resolves the (embedded source, destination, perm) tuples
// for the host OS. fvmuxa.cmd is only written on Windows; it would
// just be inert clutter on unix where fvmuxa itself is the entry
// point.
func (l Locations) targets() []target {
	ts := []target{
		{src: "fvmuxa", dst: filepath.Join(l.BinDir, "fvmuxa"), perm: 0o755},
		{src: "fvmux.tmux.conf", dst: filepath.Join(l.ConfDir, "fvmux.tmux.conf"), perm: 0o644},
	}
	if runtime.GOOS == "windows" {
		ts = append(ts, target{
			src:  "fvmuxa.cmd",
			dst:  filepath.Join(l.BinDir, "fvmuxa.cmd"),
			perm: 0o755,
		})
	}
	return ts
}

// Missing returns destination paths that don't yet exist on disk.
// Empty slice ⇒ glue is fully installed; the wizard skips its prompt
// in that case.
func (l Locations) Missing() []string {
	var missing []string
	for _, t := range l.targets() {
		if _, err := os.Stat(t.dst); errors.Is(err, os.ErrNotExist) {
			missing = append(missing, t.dst)
		}
	}
	return missing
}

// Install writes every missing glue file to its destination. Existing
// files are left untouched on purpose — overwriting would clobber any
// hand-edits the user made to fvmux.tmux.conf. Returns the list of
// files actually written.
func (l Locations) Install() ([]string, error) {
	var wrote []string
	for _, t := range l.targets() {
		if _, err := os.Stat(t.dst); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return wrote, err
		}
		data, err := files.ReadFile(t.src)
		if err != nil {
			return wrote, err
		}
		if err := os.MkdirAll(filepath.Dir(t.dst), 0o755); err != nil {
			return wrote, err
		}
		if err := atomicfile.Write(t.dst, data, t.perm); err != nil {
			return wrote, err
		}
		wrote = append(wrote, t.dst)
	}
	return wrote, nil
}

// BinDirOnPath reports whether BinDir appears in $PATH. The wizard
// uses this to decide whether a post-install hint about updating the
// shell rc is worth showing.
func (l Locations) BinDirOnPath() bool {
	want, err := filepath.Abs(l.BinDir)
	if err != nil {
		want = l.BinDir
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		got, err := filepath.Abs(dir)
		if err != nil {
			got = dir
		}
		if got == want {
			return true
		}
	}
	return false
}
