package headless

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/menus"
	"github.com/oldwired/fvmux/internal/statusbar"
)

// TestMenuBarRender draws the default menu bar to a 80×1 headless
// surface and snapshots the top row. Asserts the eight top-level
// labels are present, in order, separated by whitespace.
func TestMenuBarRender(t *testing.T) {
	h := term.NewHeadless(80, 1)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	reg := commands.Defaults()
	mb := menus.Build(geom.NewRect(0, 0, 80, 1), reg)
	mb.State |= consts.SfExposed | consts.SfVisible
	mb.Draw()
	_ = h.Flush()

	assertGolden(t, "menubar", h.Snapshot())
}

// TestStatusBarRender draws the bottom row with a representative snapshot
// (session name + two windows + clock-suppressed). Captures the layout
// we promise users; future regressions to formatLeft / formatRight fail
// this test rather than only surfacing in manual smoke.
func TestStatusBarRender(t *testing.T) {
	h := term.NewHeadless(80, 1)
	views.SetRootBackend(h)
	defer views.SetRootBackend(nil)

	bar := statusbar.Build(geom.NewRect(0, 0, 80, 1), "dev", "15:04")
	bar.Refresh(statusbar.Snapshot{
		SessionName: "dev",
		HideClock:   true, // golden must be stable across runs.
		Windows: []statusbar.WindowEntry{
			{Number: 1, Title: "shell", Focused: true},
			{Number: 2, Title: "logs", Activity: true},
		},
		FocusedTitle: "shell",
	})
	bar.Line.State |= consts.SfExposed | consts.SfVisible
	bar.Line.Draw()
	_ = h.Flush()

	assertGolden(t, "statusbar", h.Snapshot())
}
