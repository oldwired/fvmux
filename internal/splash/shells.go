package splash

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"

	"github.com/oldwired/fvmux/internal/ui"
)

// pickShell asks the user to choose a default shell. Behaviour:
//
//   - Unix: parses /etc/shells, keeps only entries that exist + are
//     executable, then adds "Keep $SHELL" + "Custom path…" rows.
//   - Windows: offers a fixed list (PowerShell, cmd) since /etc/shells
//     isn't a thing there.
//
// currentShell is the value currently in config.toml (Terminal.Shell);
// empty means "honour $SHELL". The picker preselects nothing — the user
// makes an explicit choice or hits Esc to keep current.
//
// Returns the chosen path. "" means "keep current" (no config write).
func pickShell(a *fvapp.Application, currentShell string) string {
	shells := enumerateShells()

	// Build the popup rows: shells, then sentinels.
	items := make([]string, 0, len(shells)+2)
	for _, s := range shells {
		mark := "  "
		if s == currentShell {
			mark = "* "
		}
		items = append(items, mark+s)
	}
	const (
		labelKeep   = "  Keep current ($SHELL)"
		labelCustom = "  Custom path…"
	)
	items = append(items, labelKeep, labelCustom)

	desk := a.Desktop.BaseView()
	origin := ui.CenterRect(desk.Size, 40, 8, 0).A
	idx := popupmenu.New(origin, items, 50).Run(&a.Desktop.Group)
	switch {
	case idx < 0:
		return "" // Esc/cancel.
	case idx < len(shells):
		return shells[idx]
	case items[idx] == labelKeep:
		return ""
	case items[idx] == labelCustom:
		if got, ok := promptShellPath(a, currentShell); ok {
			return got
		}
		return ""
	}
	return ""
}

// enumerateShells returns absolute paths of available shells. Unix
// reads /etc/shells and filters to existing executable entries.
// Windows probes a known-good list (modern pwsh first, then Windows
// PowerShell, cmd, Git Bash, WSL) filtered to those actually
// installed.
func enumerateShells() []string {
	if runtime.GOOS == "windows" {
		// Order matters — powerusers want pwsh ahead of legacy
		// Windows PowerShell. Anything not installed gets filtered.
		return filterExisting([]string{
			`C:\Program Files\PowerShell\7\pwsh.exe`,
			`C:\Program Files\PowerShell\6\pwsh.exe`,
			`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
			`C:\Windows\System32\cmd.exe`,
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Windows\System32\wsl.exe`,
		})
	}
	f, err := os.Open("/etc/shells")
	if err != nil {
		// Fall back to a sensible POSIX list.
		return filterExisting([]string{
			"/bin/bash", "/bin/zsh", "/bin/sh",
			"/usr/bin/bash", "/usr/bin/zsh", "/usr/bin/fish",
			"/opt/homebrew/bin/fish", "/opt/homebrew/bin/zsh",
		})
	}
	defer func() { _ = f.Close() }()

	var out []string
	seen := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !filepath.IsAbs(line) {
			continue
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return filterExisting(out)
}

// filterExisting keeps only paths that stat as regular files. Cheap
// guard against /etc/shells listing uninstalled shells (the file is
// rarely groomed when packages are removed).
func filterExisting(paths []string) []string {
	out := paths[:0]
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		out = append(out, p)
	}
	return out
}

// promptShellPath opens a centred input dialog asking for an arbitrary
// path. Pre-seeded with currentShell (when set). Returns (path, true)
// on OK, ("", false) on Cancel/Esc.
func promptShellPath(a *fvapp.Application, initial string) (string, bool) {
	desk := a.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 60, 8, 2)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "Custom shell path")

	il := dialogs.NewInputLine(geom.NewRect(2, 4, w-3, 5), 1024)
	il.SetText(initial)
	d.Insert(dialogs.NewLabel(
		geom.NewRect(2, 2, w-3, 3), "Path to shell binary:", il))
	d.Insert(il)

	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-12, h-3, w/2-2, h-2),
		"O~K~", consts.CmOK, dialogs.BfDefault,
	))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2+2, h-3, w/2+12, h-2),
		"~C~ancel", consts.CmCancel, 0,
	))

	if a.Desktop.ExecView(d) != consts.CmOK {
		return "", false
	}
	return strings.TrimSpace(il.Text()), true
}
