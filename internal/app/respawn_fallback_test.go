package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/profile"
	"github.com/oldwired/fvmux/internal/sshmgr"
)

// TestResolveProfileFallback is the regression for finding #9: a respawn (or
// session restore) of a pane whose Profile name isn't a registered profile
// must resolve an SSH host alias to a real `ssh <alias>` spawn instead of
// silently falling back to a local shell. Non-alias names still fall back to
// the default shell profile.
func TestResolveProfileFallback(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{Root: dir, StateRoot: filepath.Join(dir, "state")}

	// hosts.toml with a single alias. resolveProfileFallback reads this via
	// sshmgr.Load(paths.HostsFile()).
	hosts := "[[host]]\n" +
		"alias = \"web1\"\n" +
		"host = \"web1.example.com\"\n" +
		"user = \"deploy\"\n" +
		"port = 22\n"
	if err := os.WriteFile(paths.HostsFile(), []byte(hosts), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Mux{}
	m.Opts.Paths = paths
	// A non-nil pool is required — Acquire only records intent and returns a
	// socket path (no ssh process is spawned).
	m.sshPool = sshmgr.NewPool(paths.ControlSocket)

	// Known alias → synthesized ssh profile.
	got := m.resolveProfileFallback("web1", m.hostByAlias)
	if got == nil {
		t.Fatal("resolveProfileFallback(web1) = nil")
	}
	if got.Command != "ssh" {
		t.Errorf("Command = %q; want ssh", got.Command)
	}
	if !slices.Contains(got.Args, "web1") {
		t.Errorf("Args = %v; want to contain the alias \"web1\"", got.Args)
	}

	// Unknown name → default shell profile (never an ssh spawn). Use an
	// improbable alias so a real ~/.ssh/config on the test host can't match.
	def := profile.Defaults()[0]
	fallback := m.resolveProfileFallback("fvmux-nosuch-host-zzz9999", m.hostByAlias)
	if fallback == nil {
		t.Fatal("resolveProfileFallback(unknown) = nil")
	}
	if fallback.Command != def.Command {
		t.Errorf("unknown alias Command = %q; want the default shell %q",
			fallback.Command, def.Command)
	}
	if fallback.Name != def.Name {
		t.Errorf("unknown alias Name = %q; want %q", fallback.Name, def.Name)
	}
}
