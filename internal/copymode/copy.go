// Package copymode implements fvmux's copy mode: a keyboard-driven
// selection layer over the focused pane's scrollback.
//
// Flow:
//   - Enter copy mode → terminal cursor parks at the visible region's
//     bottom-right (fv-go's Terminal.EnterCopyMode does this).
//   - Arrows / PgUp / PgDn / Home / End move the copy cursor.
//   - Space toggles a selection anchor.
//   - Enter copies the selection to the OS clipboard and exits.
//   - `/` re-enters fv-go's scrollback search.
//   - Esc exits without copying.
package copymode

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal"

	"github.com/oldwired/fvmux/internal/clipboard"
)

// Show enters copy mode on t and installs an OfPreProcess key
// driver on a.Desktop. The driver consumes navigation + Space + Enter
// + Esc / `/` and forwards everything else through to the terminal.
// On Enter the selection (if any) is copied to the OS clipboard; on
// Esc we exit without copying. Driver self-removes on exit.
//
// The returned Driver is the lifecycle handle: the caller must Close()
// it when the pane it drives goes away (pane kill, window close),
// otherwise the driver stays installed desktop-wide, eating keys for a
// stopped terminal. OnDone fires on every exit path.
func Show(a *fvapp.Application, t *terminal.Terminal) *Driver {
	if t == nil {
		return nil
	}
	t.EnterCopyMode()
	drv := &Driver{
		Base: views.NewBase(geom.NewRect(0, 0, 0, 0)),
		app:  a,
		term: t,
	}
	drv.Options |= consts.OfPreProcess
	drv.SetSelf(drv)
	a.Desktop.Insert(drv)
	return drv
}

// Paste reads the OS clipboard and writes it to t. When the pane has
// bracketed-paste mode enabled, the text is wrapped in the bracketed
// markers; otherwise it's sent raw.
func Paste(t *terminal.Terminal) error {
	if t == nil {
		return nil
	}
	text, err := clipboard.Get()
	if err != nil || text == "" {
		return err
	}
	return t.Paste(text)
}

// Driver is the OfPreProcess key listener that powers an active copy
// mode session. It removes itself from the desktop on exit.
type Driver struct {
	views.Base

	app  *fvapp.Application
	term *terminal.Terminal
	gone bool

	// OnDone fires once, on any exit path (Enter, Esc, `/`, Close).
	// The Mux uses it to un-suspend the prefix listener.
	OnDone func()
}

// Active reports whether the driver is still installed. Nil-safe.
func (d *Driver) Active() bool { return d != nil && !d.gone }

// Term returns the terminal the driver is attached to. Nil-safe.
func (d *Driver) Term() *terminal.Terminal {
	if d == nil {
		return nil
	}
	return d.term
}

// Close tears the driver down from outside (the pane it drives is
// being stopped). Idempotent and nil-safe.
func (d *Driver) Close() {
	if d == nil {
		return
	}
	d.exit()
}

// GetTypeID for serial registry.
func (d *Driver) GetTypeID() string { return "copymode-driver" }

// Draw is a no-op — the driver is invisible.
func (d *Driver) Draw() {}

// HandleEvent translates copy-mode key bindings. Non-key events fall
// through unchanged. Key events that we consume have ev.What cleared.
func (d *Driver) HandleEvent(ev *drivers.Event) {
	if d.gone || ev.What != consts.EvKeyDown {
		return
	}
	switch ev.KeyCode {
	case consts.KbLeft:
		d.term.MoveCopyCursor(-1, 0)
	case consts.KbRight:
		d.term.MoveCopyCursor(1, 0)
	case consts.KbUp:
		d.term.MoveCopyCursor(0, -1)
	case consts.KbDown:
		d.term.MoveCopyCursor(0, 1)
	case consts.KbPgUp:
		d.term.MoveCopyCursor(0, -10)
	case consts.KbPgDn:
		d.term.MoveCopyCursor(0, 10)
	case consts.KbHome:
		d.term.MoveCopyCursor(-1<<14, 0)
	case consts.KbEnd:
		d.term.MoveCopyCursor(1<<14, 0)
	case consts.KbSpaceBar:
		d.term.ToggleCopyAnchor()
	case consts.KbEnter:
		if sel, ok := d.term.CopySelection(); ok && sel != "" {
			_ = clipboard.Set(sel)
		}
		d.exit()
	case consts.KbEsc:
		d.exit()
	default:
		if ev.UnicodeChar == '/' {
			d.exit()
			d.term.StartScrollbackSearch()
		} else {
			return // pass through.
		}
	}
	ev.Clear()
	views.MarkDirty()
}

func (d *Driver) exit() {
	if d.gone {
		return
	}
	d.gone = true
	d.term.ExitCopyMode()
	d.app.Desktop.Delete(d)
	if d.OnDone != nil {
		d.OnDone()
	}
}
