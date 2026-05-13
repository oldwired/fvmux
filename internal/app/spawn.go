// Package app holds fvmux's runtime glue: the Mux struct that owns the
// fv-go Application instance, the dispatch helpers that connect the
// commands.Registry to UI actions, and the per-pane spawn helpers.
package app

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/session"
)

// SpawnPane creates a fresh session.Pane (Terminal + ID + Title) and
// starts the named shell in it. The Terminal is constructed at `bounds`
// (window-local coordinates); layout.Materialize will reposition it via
// the SplitGroup tree as needed.
//
// Passing shell == "" defaults to /bin/sh. Passing env == nil lets the
// terminal widget append os.Environ() plus TERM=xterm-256color, which
// is what every shell expects.
func SpawnPane(bounds geom.Rect, shell string, args []string) (*session.Pane, error) {
	t := terminal.New(bounds)
	if shell == "" {
		shell = "/bin/sh"
	}
	if err := t.Start(shell, args, nil); err != nil {
		return nil, err
	}
	return &session.Pane{
		ID:    session.NewPaneID(),
		Term:  t,
		Title: shell,
	}, nil
}
