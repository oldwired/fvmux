package app

import (
	"strings"

	"github.com/oldwired/fvmux/internal/profile"
)

// runDialog prompts the user for a one-shot command and spawns it in a
// new window via `sh -c`. Lets users invoke ad-hoc commands (`nano …`,
// `htop`, `ssh staging-3`) without defining a profile first.
func (m *Mux) runDialog() {
	text, ok := promptString(m.App, "Run Command",
		"Command (run via /bin/sh -c):", "")
	if !ok {
		return
	}
	cmd := strings.TrimSpace(text)
	if cmd == "" {
		return
	}
	prof := &profile.Profile{
		Name:    "run",
		Command: "/bin/sh",
		Args:    []string{"-c", cmd},
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
