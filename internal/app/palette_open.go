package app

import (
	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/palette"
)

// openPalette is the Ctrl-G P entry-point that wraps palette.ShowWithOptions
// with persisted MRU + the four synthetic prefix entries (`:`, `@`,
// `#`, `?`). On a successful pick it persists the new MRU back to
// state.toml so subsequent opens bubble recently-used commands.
func (m *Mux) openPalette() {
	pos := palette.ParsePosition(m.Opts.Config.Appearance.PalettePosition)
	state, _ := config.LoadState(m.Opts.Paths.StateFile())
	mru := append([]uint16(nil), state.PaletteMRU...)
	persist := func(newMRU []uint16) {
		_ = config.WithStateLock(m.Opts.Paths.StateFile(), func() error {
			st, _ := config.LoadState(m.Opts.Paths.StateFile())
			st.PaletteMRU = newMRU
			return config.SaveState(m.Opts.Paths.StateFile(), st)
		})
	}
	specials := []palette.SpecialEntry{
		{
			Label:  ": run command…           (free-form shell command in a new window)",
			Action: func(*commands.Ctx) { m.runDialog() },
		},
		{
			Label:  "@ jump to window…        (window list)",
			Action: func(*commands.Ctx) { m.showWindowList() },
		},
		{
			Label:  "# open session…          (load saved layout)",
			Action: func(*commands.Ctx) { m.openSessionPicker() },
		},
		{
			Label:  "? show cheatsheet        (every chord, grouped by category)",
			Action: func(*commands.Ctx) { m.ShowCheatsheet() },
		},
	}
	palette.ShowWithOptions(m.App, m.Reg, &commands.Ctx{App: m.App}, palette.Options{
		Pos:     pos,
		MRU:     mru,
		Persist: persist,
		Special: specials,
	})
}
