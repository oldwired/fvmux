package app

import (
	"path/filepath"
	"testing"

	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/commands"
	"github.com/oldwired/fvmux/internal/config"
)

// TestApplyPrefix_ListenerAndRegistryLockstep is the regression for finding
// #8: ApplyPrefix must move the armed prefix listener and every registry chord
// to the new prefix key together — never one without the other. A drift would
// leave chords firing on a key the listener no longer arms on (or vice versa).
// Asserted in both directions: C-g → C-b and back.
func TestApplyPrefix_ListenerAndRegistryLockstep(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{Root: dir, StateRoot: filepath.Join(dir, "state")}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	desk := fvapp.NewDesktop(geom.NewRect(0, 0, 80, 24))
	m := &Mux{
		App:     &fvapp.Application{Program: &fvapp.Program{Desktop: desk}},
		Reg:     commands.Defaults(),
		windows: map[views.View]*windowState{},
	}
	m.Opts.Paths = paths              // UpdateKeys writes config.toml into the sandbox
	m.Opts.Config = config.Defaults() // prefix_key "C-g"
	m.InstallPrefixListener()

	// Baseline: listener armed on C-g, registry chords in the C-g space.
	if got := m.prefix.Spec().ConfigKey; got != "C-g" {
		t.Fatalf("baseline prefix spec = %q, want C-g", got)
	}
	if m.Reg.LookupChord("C-g w") == nil {
		t.Fatal("baseline: C-g w should resolve (Window List)")
	}

	// C-g → C-b: both the listener spec and the registry follow.
	m.ApplyPrefix("C-b")
	if got := m.prefix.Spec().ConfigKey; got != "C-b" {
		t.Errorf("after ApplyPrefix(C-b) listener spec = %q, want C-b", got)
	}
	if m.Reg.LookupChord("C-b w") == nil {
		t.Error("after ApplyPrefix(C-b) registry chord C-b w missing")
	}
	if m.Reg.LookupChord("C-g w") != nil {
		t.Error("after ApplyPrefix(C-b) stale C-g w still resolves")
	}

	// C-b → C-g: the mirror. Reverting must resync a listener still armed on
	// the old prefix and sweep every chord back to the C-g space.
	m.ApplyPrefix("C-g")
	if got := m.prefix.Spec().ConfigKey; got != "C-g" {
		t.Errorf("after ApplyPrefix(C-g) listener spec = %q, want C-g", got)
	}
	if m.Reg.LookupChord("C-g w") == nil {
		t.Error("after ApplyPrefix(C-g) registry chord C-g w missing")
	}
	if m.Reg.LookupChord("C-b w") != nil {
		t.Error("after ApplyPrefix(C-g) stale C-b w still resolves")
	}
}
