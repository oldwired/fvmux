package app

import (
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/layout"
	"github.com/oldwired/fvmux/internal/session"
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

func TestFilesFollowIsPaneScopedNotAliasScoped(t *testing.T) {
	paneA := &session.Pane{ID: session.NewPaneID(), SSHAlias: "prod"}
	paneB := &session.Pane{ID: session.NewPaneID(), SSHAlias: "prod"}
	followA := &fileWindowState{Alias: "prod", OriginPaneID: paneA.ID, FollowTerminal: true, RemoteCWD: "/old-a"}
	followA2 := &fileWindowState{Alias: "prod", OriginPaneID: paneA.ID, FollowTerminal: true, RemoteCWD: "/other-a"}
	followB := &fileWindowState{Alias: "prod", OriginPaneID: paneB.ID, FollowTerminal: true, RemoteCWD: "/old-b"}
	unlinked := &fileWindowState{Alias: "prod", OriginPaneID: paneA.ID, RemoteCWD: "/manual"}
	m := &Mux{fileWindows: map[views.View]*fileWindowState{
		views.NewBackground(geom.Rect{}, 'a'): followA,
		views.NewBackground(geom.Rect{}, 'd'): followA2,
		views.NewBackground(geom.Rect{}, 'b'): followB,
		views.NewBackground(geom.Rect{}, 'c'): unlinked,
	}}

	m.followFilesForPane(paneA, "/srv/a")

	if followA.RemoteCWD != "/srv/a" {
		t.Fatalf("matching Files cwd=%q, want /srv/a", followA.RemoteCWD)
	}
	if followA2.RemoteCWD != "/srv/a" {
		t.Fatalf("second Files window from same pane cwd=%q, want /srv/a", followA2.RemoteCWD)
	}
	if followB.RemoteCWD != "/old-b" {
		t.Fatalf("same-alias different-pane Files moved to %q", followB.RemoteCWD)
	}
	if unlinked.RemoteCWD != "/manual" {
		t.Fatalf("follow-disabled Files moved to %q", unlinked.RemoteCWD)
	}
}

func TestFilesLinksDetachOnPaneRemovalAndRebindOnRespawn(t *testing.T) {
	oldPane := &session.Pane{ID: session.NewPaneID()}
	replacement := &session.Pane{ID: session.NewPaneID()}
	following := &fileWindowState{OriginPaneID: oldPane.ID, FollowTerminal: true}
	manual := &fileWindowState{OriginPaneID: oldPane.ID}
	m := &Mux{fileWindows: map[views.View]*fileWindowState{
		views.NewBackground(geom.Rect{}, 'a'): following,
		views.NewBackground(geom.Rect{}, 'b'): manual,
	}}

	m.rebindFilesFromPane(oldPane.ID, replacement)
	if following.OriginPaneID != replacement.ID || manual.OriginPaneID != replacement.ID {
		t.Fatalf("respawn did not preserve origins: following=%d manual=%d want=%d",
			following.OriginPaneID, manual.OriginPaneID, replacement.ID)
	}
	if !following.FollowTerminal {
		t.Fatal("respawn unexpectedly disabled an active Files link")
	}

	m.detachFilesFromPane(replacement.ID)
	if following.OriginPaneID != 0 || following.FollowTerminal {
		t.Fatalf("removed source left stale link: origin=%d follow=%v",
			following.OriginPaneID, following.FollowTerminal)
	}
	if manual.OriginPaneID != 0 {
		t.Fatalf("removed source left stale manual origin=%d", manual.OriginPaneID)
	}
}

func TestToggleFilesFollowUsesOriginTerminalCWD(t *testing.T) {
	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 120, 40))
	terminalFrame := views.NewWindow(geom.NewRect(0, 0, 60, 20), "terminal", 1)
	filesFrame := views.NewWindow(geom.NewRect(5, 3, 105, 35), "files", 2)
	pane := &session.Pane{ID: session.NewPaneID(), SSHAlias: "prod", CWD: "/srv/app"}
	leaf := layout.Leaf(pane)
	ws := &windowState{Frame: terminalFrame, Root: leaf, Focus: leaf, Number: 1}
	fw := &fileWindowState{Frame: filesFrame, Alias: "prod", Number: 2, OriginPaneID: pane.ID, RemoteCWD: "/home/me", State: filesReady}
	m := &Mux{
		App:         &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		windows:     map[views.View]*windowState{terminalFrame.Self(): ws},
		fileWindows: map[views.View]*fileWindowState{filesFrame.Self(): fw},
	}
	desk.InsertWindow(terminalFrame)
	desk.InsertWindow(filesFrame)
	desk.Focus(filesFrame)

	m.toggleFilesFollowTerminal()

	if !fw.FollowTerminal || fw.RemoteCWD != "/srv/app" {
		t.Fatalf("follow=%v cwd=%q, want enabled at originating pane cwd", fw.FollowTerminal, fw.RemoteCWD)
	}
	if got := fw.displayTitle(); got != "[prod] Files ↔ Terminal — /srv/app" {
		t.Fatalf("linked title=%q", got)
	}

	m.toggleFilesFollowTerminal()
	if fw.FollowTerminal {
		t.Fatal("second toggle did not disable directory follow")
	}
}
