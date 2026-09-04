package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidSessionName(t *testing.T) {
	valid := []string{"work", "client-a", "a b", "work.1", "prod_2"}
	for _, name := range valid {
		if err := ValidSessionName(name); err != nil {
			t.Errorf("ValidSessionName(%q) = %v; want nil", name, err)
		}
	}
	invalid := []string{"", "  ", "../config", "..", ".", "a/b", `a\b`, "sessions/../../x"}
	for _, name := range invalid {
		if err := ValidSessionName(name); err == nil {
			t.Errorf("ValidSessionName(%q) = nil; want error", name)
		}
	}
}

func TestSessionFile_HostileNameStaysInsideSessionsDir(t *testing.T) {
	p := Paths{Root: "/cfg", StateRoot: "/state"}
	sessionsDir := filepath.Join(p.Root, "sessions")

	// Every name that ValidSessionName rejects must still resolve to a
	// path strictly inside the sessions dir (it gets hashed), never an
	// escape like /cfg/config.toml.
	for _, name := range []string{"../config", "..", ".", "a/b", `a\b`, "sessions/../../x", ""} {
		got := p.SessionFile(name)
		if dir := filepath.Dir(got); dir != sessionsDir {
			t.Errorf("SessionFile(%q) = %q escaped sessions dir (dir %q, want %q)",
				name, got, dir, sessionsDir)
		}
		if !strings.HasSuffix(got, ".toml") {
			t.Errorf("SessionFile(%q) = %q; want a .toml file", name, got)
		}
	}

	// A valid name maps to the obvious plain path (no hashing).
	if got := p.SessionFile("work"); got != filepath.Join(sessionsDir, "work.toml") {
		t.Errorf("SessionFile(\"work\") = %q; want plain path", got)
	}
}

// TestWithRoot_MovesStateRootUnderRoot pins review finding #40: WithRoot
// now relocates StateRoot under the new config root, so -config actually
// isolates an instance — state.toml and the ControlMaster socket dir land
// beside config.toml rather than at the shared global XDG state location.
// WithRoot("") stays a no-op.
func TestWithRoot_MovesStateRootUnderRoot(t *testing.T) {
	p := Default().WithRoot("/tmp/x")
	if p.Root != "/tmp/x" {
		t.Errorf("Root = %q, want /tmp/x", p.Root)
	}
	if want := filepath.Join("/tmp/x", "state"); p.StateRoot != want {
		t.Errorf("StateRoot = %q, want %q", p.StateRoot, want)
	}
	if got, want := p.ControlSocketDir(), filepath.Join("/tmp/x", "state", "cm"); got != want {
		t.Errorf("ControlSocketDir = %q, want %q", got, want)
	}
	if got, want := p.StateFile(), filepath.Join("/tmp/x", "state", "state.toml"); got != want {
		t.Errorf("StateFile = %q, want %q", got, want)
	}
	// A control socket for an alias resolves under the moved cm dir.
	if dir := filepath.Dir(p.ControlSocket("prod")); dir != p.ControlSocketDir() {
		t.Errorf("ControlSocket dir = %q, want %q", dir, p.ControlSocketDir())
	}

	// WithRoot("") leaves the receiver's defaults intact.
	base := Default()
	if got := base.WithRoot(""); got != base {
		t.Errorf("WithRoot(\"\") = %+v, want unchanged %+v", got, base)
	}
}

func TestControlSocket_HashedNameIsSafeAndBounded(t *testing.T) {
	p := Paths{Root: "/cfg", StateRoot: "/state"}

	// An alias with path separators / traversal must not escape the cm dir.
	got := p.ControlSocket("../../etc/evil")
	if dir := filepath.Dir(got); dir != p.ControlSocketDir() {
		t.Fatalf("socket escaped cm dir: %q (dir %q)", got, dir)
	}
	base := filepath.Base(got)
	if strings.ContainsAny(base, "/.\\") && !strings.HasSuffix(base, ".sock") {
		t.Fatalf("socket basename has unsafe chars: %q", base)
	}

	// Deterministic: same alias → same socket (computed independently).
	first := p.ControlSocket("prod")
	second := Paths{Root: "/cfg", StateRoot: "/state"}.ControlSocket("prod")
	if first != second {
		t.Fatalf("ControlSocket not deterministic: %q vs %q", first, second)
	}
	// Distinct aliases → distinct sockets.
	if p.ControlSocket("prod") == p.ControlSocket("staging") {
		t.Fatal("distinct aliases collided")
	}

	// Even a very long alias under an ordinary state root stays below the
	// conservative Unix socket path limit.
	long := strings.Repeat("very-long-alias-segment.", 20)
	deep := Paths{StateRoot: "/home/somebody/.local/state/fvmux"}
	if n := len(deep.ControlSocket(long)); n > controlSocketPathLimit {
		t.Fatalf("socket path too long: %d bytes", n)
	}
}

func TestControlSocket_DeepStateRootUsesPrivateShortFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix-domain ControlMaster socket limits")
	}
	rootA := t.TempDir()
	rootB := t.TempDir()
	deepA := Paths{Root: rootA, StateRoot: filepath.Join(rootA, strings.Repeat("deep-segment", 12), "state-a")}
	deepB := Paths{Root: rootB, StateRoot: filepath.Join(rootB, strings.Repeat("deep-segment", 12), "state-b")}

	dirA := deepA.ControlSocketDir()
	if filepath.Dir(dirA) != "/tmp" {
		t.Fatalf("ControlSocketDir = %q, want a direct /tmp fallback", dirA)
	}
	if dirA == deepB.ControlSocketDir() {
		t.Fatal("different state roots must not share a fallback socket directory")
	}
	if got := len(deepA.ControlSocket(strings.Repeat("alias", 100))); got > controlSocketPathLimit {
		t.Fatalf("fallback socket path is %d bytes, limit is %d", got, controlSocketPathLimit)
	}

	// EnsureDirs must create the computed directory, not the unusably deep
	// preferred one, and it must be private under the shared /tmp parent.
	if err := deepA.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(dirA) })
	info, err := os.Stat(dirA)
	if err != nil {
		t.Fatalf("stat fallback dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("fallback mode = %#o, want 0700", got)
	}
}
