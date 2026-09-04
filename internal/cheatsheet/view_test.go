package cheatsheet

import (
	"strings"
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
)

func TestShowCreatesOneNonModalWindow(t *testing.T) {
	desktop := fvapp.NewDesktop(geom.NewRect(0, 0, 100, 30))
	a := &fvapp.Application{Program: &fvapp.Program{Desktop: desktop}}

	Show(a, commands.Defaults())
	sheet := findWindow(t, desktop)
	if sheet.GetState(consts.SfModal) {
		t.Fatal("cheatsheet window is modal")
	}
	if desktop.Current() != sheet {
		t.Fatal("new cheatsheet window did not receive focus")
	}
	for _, child := range sheet.Children {
		if _, ok := child.(*dialogs.Button); ok {
			t.Fatal("non-modal cheatsheet still has a redundant close button")
		}
	}
	if got, want := sheet.content.Origin, (geom.Point{X: 1, Y: 1}); got != want {
		t.Errorf("content origin = %v, want full-interior origin %v", got, want)
	}
	if got, want := sheet.content.Size.Y, sheet.Size.Y-2; got != want {
		t.Errorf("content height = %d, want full interior height %d", got, want)
	}

	Show(a, commands.Defaults())
	if got := countWindows(desktop); got != 1 {
		t.Fatalf("opening cheatsheet twice created %d windows, want 1", got)
	}

	ev := drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEsc}
	sheet.HandleEvent(&ev)
	if sheet.Owner != nil {
		t.Fatal("Esc did not close non-modal cheatsheet window")
	}
}

func TestSearchCandidatesCoverCommandsAndContextualHelp(t *testing.T) {
	candidates := searchCandidates(GenerateBaked(commands.Defaults()))
	for _, want := range []string{
		"[Window] C-g X — Kill Window",
		"[Copy mode] Space — Toggle the selection anchor.",
		"[Dialogs and pickers] Esc — Cancel or close.",
	} {
		found := false
		for _, candidate := range candidates {
			if candidate.label == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("search index missing %q", want)
		}
	}
	for _, candidate := range candidates {
		if strings.Contains(candidate.label, "**") || strings.Contains(candidate.label, "`") {
			t.Errorf("search label contains Markdown markers: %q", candidate.label)
		}
	}
}

func TestScrollToLineClampsAtDocumentEnd(t *testing.T) {
	sheet := newWindow(geom.NewRect(0, 0, 70, 12), commands.Defaults())
	sheet.scrollToLine(1 << 20)
	lineCount := len(strings.Split(sheet.source, "\n"))
	want := lineCount - sheet.content.Size.Y
	if want < 0 {
		want = 0
	}
	if sheet.content.Top != want {
		t.Errorf("content Top = %d, want end-clamped %d", sheet.content.Top, want)
	}
	if sheet.content.VScroll.Value != want {
		t.Errorf("scrollbar value = %d, want %d", sheet.content.VScroll.Value, want)
	}
}

func TestSlashSearchScrollsToChosenEntry(t *testing.T) {
	desktop := fvapp.NewDesktop(geom.NewRect(0, 0, 100, 30))
	a := &fvapp.Application{Program: &fvapp.Program{Desktop: desktop}}
	Show(a, commands.Defaults())
	sheet := findWindow(t, desktop)

	queue := drivers.NewQueue()
	views.SetEventQueue(queue)
	defer views.SetEventQueue(nil)
	for _, r := range "killwindow" {
		queue.Put(drivers.Event{What: consts.EvKeyDown, UnicodeChar: r})
	}
	queue.Put(drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEnter})

	ev := drivers.Event{What: consts.EvKeyDown, UnicodeChar: '/'}
	sheet.HandleEvent(&ev)
	if sheet.content.Top == 0 {
		t.Fatal("selecting a fuzzy result did not scroll the cheatsheet")
	}
	if sheet.Owner == nil {
		t.Fatal("closing the search picker also closed the non-modal cheatsheet")
	}
}

func findWindow(t *testing.T, desktop *fvapp.Desktop) *window {
	t.Helper()
	for _, child := range desktop.Children {
		if sheet, ok := child.(*window); ok {
			return sheet
		}
	}
	t.Fatal("cheatsheet window was not inserted")
	return nil
}

func countWindows(desktop *fvapp.Desktop) int {
	count := 0
	for _, child := range desktop.Children {
		if _, ok := child.(*window); ok {
			count++
		}
	}
	return count
}
