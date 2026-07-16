package headless

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/term"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// TestSftpBrowserOpenFocusesDialog guards the focus contract that
// internal/sftp.buildBrowser (via ShowAsync) depends on when it pops the
// browser.
//
// The browser is a non-modal floating dialog inserted on top of whatever
// window currently holds focus. On the auth-then-retry path that window
// is the ssh pane the user just typed a passphrase into. The old
// sequence was Insert(d) + MakeFirst(d) — but MakeFirst early-returns
// when the view is already the last child, which Insert just made it, so
// it left keyboard focus on the ssh pane and the freshly-spawned browser
// opened unfocused. The fix is an explicit Desktop.Focus(d).
//
// This reproduces both halves with fv-go's public API so a regression in
// either direction is caught: (a) a "simplification" back to
// MakeFirst-only, or (b) an fv-go change to Focus/MakeFirst semantics.
func TestSftpBrowserOpenFocusesDialog(t *testing.T) {
	p := fvapp.NewProgram(term.NewHeadless(80, 24))
	p.SetDesktop(fvapp.NewDesktop(geom.NewRect(0, 1, 80, 23)))

	// Stand-in for the ssh PIN window: a selectable Window that holds
	// focus, exactly as the auth-then-retry spawn leaves things.
	sshWin := views.NewWindow(geom.NewRect(0, 0, 20, 8), "ssh", 1)
	p.Desktop.Insert(sshWin)
	p.Desktop.Focus(sshWin)
	if p.Desktop.Current() != views.View(sshWin) {
		t.Fatalf("precondition: ssh window should hold focus, got %v", p.Desktop.Current())
	}

	// Stand-in for the SFTP browser. Insert appends it as the new last
	// (topmost) desktop child, mirroring Show.
	browser := dialogs.NewDialog(geom.NewRect(2, 2, 40, 16), "SFTP")
	p.Desktop.Insert(browser)

	// The bug: MakeFirst is a no-op when the target is already last, so
	// focus stays on the ssh window.
	p.Desktop.MakeFirst(browser)
	if p.Desktop.Current() == views.View(browser) {
		t.Fatal("MakeFirst focused an already-topmost dialog; if fv-go changed " +
			"this, Show can drop its explicit Focus(d) call")
	}

	// The fix: Focus(d) takes keyboard focus unconditionally.
	p.Desktop.Focus(browser)
	if p.Desktop.Current() != views.View(browser) {
		t.Fatalf("after Focus, the browser dialog should hold focus, got %v", p.Desktop.Current())
	}
}
