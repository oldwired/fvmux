package app

import (
	"fmt"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/widgets/notification"

	"github.com/oldwired/fvmux/internal/config"
)

// MaybeShowVersionBump fires a top-right notification when the running
// version differs from the last one persisted to state.toml. No-op on
// first-run (LastVersion == ""), and quietly persists the new version
// so future launches don't re-notify until another bump happens.
//
// Called from main.go after the wizard path doesn't run — keeping it
// off the first-run path means new users aren't double-notified.
func (m *Mux) MaybeShowVersionBump() {
	state, _ := config.LoadState(m.Opts.Paths.StateFile())
	if state.LastVersion == "" {
		// First-run already handles this; just record the version.
		state.LastVersion = m.Opts.Version
		_ = config.SaveState(m.Opts.Paths.StateFile(), state)
		return
	}
	if state.LastVersion == m.Opts.Version {
		return
	}
	prev := state.LastVersion
	state.LastVersion = m.Opts.Version
	_ = config.SaveState(m.Opts.Paths.StateFile(), state)

	body := fmt.Sprintf("Upgraded %s → %s\nCtrl-G ? opens the cheatsheet.",
		prev, m.Opts.Version)
	n := notification.New(
		&m.App.Desktop.Group,
		"fvmux updated",
		body,
		notification.PosTopRight,
		36,
		8*time.Second,
	)
	m.App.Desktop.Insert(n)
}
