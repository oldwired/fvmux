package app

import (
	"strings"

	"github.com/oldwired/fvmux/internal/profile"
)

// runDialog prompts the user for a one-shot command and spawns it in a
// new window via the system shell (`sh -c`; `cmd /c` on Windows). Lets
// users invoke ad-hoc commands (`nano …`, `htop`, `ssh staging-3`)
// without defining a profile first.
//
// Strings starting with ":" are interpreted as built-in fvmux easter
// eggs (`:tea`, `:konami`, `:rot13`) instead of shell commands.
func (m *Mux) runDialog() {
	text, ok := promptString(m.App, "Run Command",
		"Command (`:tea`, `:konami`, `:rot13`, or shell):", "")
	if !ok {
		return
	}
	cmd := strings.TrimSpace(text)
	if cmd == "" {
		return
	}
	if strings.HasPrefix(cmd, ":") {
		if m.handleEgg(cmd) {
			return
		}
	}
	// profile.ShellCommand, not a hardcoded /bin/sh — Run Command must
	// work on Windows too (cmd /c), same as new_window_command.
	sh, args := profile.ShellCommand(cmd)
	prof := &profile.Profile{
		Name:    "run",
		Command: sh,
		Args:    args,
		Title:   shortTitleFor(cmd),
	}
	_, _ = m.openWindowFromProfile(prof)
}

// shortTitleFor extracts the first whitespace-separated token from cmd
// for the initial window title (the shell's own title-emit will
// overwrite it shortly).
func shortTitleFor(cmd string) string {
	for i, r := range cmd {
		if r == ' ' || r == '\t' {
			if i == 0 {
				continue
			}
			return cmd[:i]
		}
	}
	return cmd
}
