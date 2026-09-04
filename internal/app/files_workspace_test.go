package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

func TestFilesForAliasReturnsMostRecentMatchingWorkspaceWindow(t *testing.T) {
	a := views.NewWindow(geom.NewRect(0, 0, 80, 24), "a", 1)
	b := views.NewWindow(geom.NewRect(1, 1, 81, 25), "b", 2)
	c := views.NewWindow(geom.NewRect(2, 2, 82, 26), "c", 3)
	fa := &fileWindowState{Frame: a, Alias: "prod", Number: 1}
	fb := &fileWindowState{Frame: b, Alias: "other", Number: 2}
	fc := &fileWindowState{Frame: c, Alias: "prod", Number: 3}
	m := &Mux{
		windows:     map[views.View]*windowState{},
		fileWindows: map[views.View]*fileWindowState{a.Self(): fa, b.Self(): fb, c.Self(): fc},
		windowOrder: []views.View{a.Self(), b.Self(), c.Self()},
	}
	if got := m.filesForAlias("prod"); got != fc {
		t.Fatalf("got %p, want newest %p", got, fc)
	}
	if got := m.filesForAlias("missing"); got != nil {
		t.Fatalf("got %p for missing alias", got)
	}
}

func TestSnapshotPreservesEachFilesWindowAndActiveIdentity(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 140, 45))
	app := &fvapp.Application{Program: &fvapp.Program{Desktop: desk}}
	a := views.NewWindow(geom.NewRect(3, 2, 103, 32), "", 2)
	b := views.NewWindow(geom.NewRect(7, 5, 127, 40), "", 5)
	fa := &fileWindowState{ID: 11, Frame: a, Alias: "prod", Number: 2, RemoteCWD: "/srv/a", LocalCWD: "/tmp/a", FocusSide: "remote", State: filesReady}
	fb := &fileWindowState{ID: 12, Frame: b, Alias: "prod", Number: 5, RemoteCWD: "/srv/b", LocalCWD: "/tmp/b", FocusSide: "local", State: filesFailed}
	m := &Mux{App: app, windows: map[views.View]*windowState{}, fileWindows: map[views.View]*fileWindowState{
		a.Self(): fa, b.Self(): fb,
	}, windowOrder: []views.View{a.Self(), b.Self()}}
	desk.InsertWindow(a)
	desk.InsertWindow(b)
	desk.Focus(b)

	snap := m.buildSnapshot()
	if snap.Active != 1 || len(snap.Windows) != 2 {
		t.Fatalf("active=%d windows=%d", snap.Active, len(snap.Windows))
	}
	if got := snap.Windows[0]; got.Kind != "files" || got.Number != 2 || got.Alias != "prod" || got.RemoteCWD != "/srv/a" || got.LocalCWD != "/tmp/a" || got.FocusSide != "remote" {
		t.Fatalf("first files snapshot=%#v", got)
	}
	if got := snap.Windows[1]; got.Kind != "files" || got.Number != 5 || got.RemoteCWD != "/srv/b" || got.FocusSide != "local" {
		t.Fatalf("second files snapshot=%#v", got)
	}
}

func TestFilesWindowParticipatesInNumberLookup(t *testing.T) {
	w := views.NewWindow(geom.NewRect(0, 0, 80, 24), "files", 4)
	m := &Mux{windows: map[views.View]*windowState{}, fileWindows: map[views.View]*fileWindowState{
		w.Self(): {Frame: w, Alias: "prod", Number: 4},
	}}
	if !m.windowNumberInUse(4) {
		t.Fatal("files window number was not reserved")
	}
	if got := m.nextWindowNumber(); got != 1 {
		t.Fatalf("next number=%d, want 1", got)
	}
}
