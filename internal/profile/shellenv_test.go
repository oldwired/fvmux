package profile

import (
	"runtime"
	"strings"
	"testing"
)

// TestFallbackShellAndShellCommand pins review finding #4: the shell
// resolution chain is now exposed via FallbackShell / ShellCommand and no
// longer hardcodes "/bin/sh" scattered through callers. Windows values are
// covered by cross-compile + review, so these assert the non-Windows branch.
func TestFallbackShellAndShellCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows shell values only")
	}
	if got := FallbackShell(); got != "/bin/sh" {
		t.Errorf("FallbackShell() = %q, want /bin/sh", got)
	}
	name, args := ShellCommand("x y")
	if name != "/bin/sh" {
		t.Errorf("ShellCommand name = %q, want /bin/sh", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "x y" {
		t.Errorf("ShellCommand args = %v, want [-c x y]", args)
	}
}

// TestDefaults_EmptyShellFallsBackToBinSh pins the tail of #4: with $SHELL
// unset, the default profile's Command must resolve to a non-empty shell
// (/bin/sh on non-Windows) rather than being left blank.
func TestDefaults_EmptyShellFallsBackToBinSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows shell values only")
	}
	t.Setenv("SHELL", "")
	d := Defaults()
	if len(d) == 0 {
		t.Fatal("Defaults() returned no profiles")
	}
	if d[0].Command == "" {
		t.Fatal("Defaults()[0].Command is empty with SHELL unset")
	}
	if d[0].Command != "/bin/sh" {
		t.Errorf("Defaults()[0].Command = %q, want /bin/sh", d[0].Command)
	}
}

// TestSpawnEnv_NilOrEmptyReturnsNil pins review finding #18: a profile with
// no env vars must yield a nil child environment so fv-go applies its own
// TERM patch (a non-nil slice would suppress it).
func TestSpawnEnv_NilOrEmptyReturnsNil(t *testing.T) {
	if env := spawnEnv(nil); env != nil {
		t.Errorf("spawnEnv(nil) = %v, want nil", env)
	}
	if env := spawnEnv(map[string]string{}); env != nil {
		t.Errorf("spawnEnv(empty) = %v, want nil", env)
	}
}

// TestSpawnEnv_ReappliesTermPatch pins #18: when a profile supplies any env
// var, spawnEnv must re-append the xterm-256color TERM patch (fv-go skips it
// for a non-nil env) AFTER the inherited outer TERM, and keep the profile's
// own vars.
func TestSpawnEnv_ReappliesTermPatch(t *testing.T) {
	t.Setenv("TERM", "tmux-256color")

	env := spawnEnv(map[string]string{"AWS_PROFILE": "dev"})

	inherited := indexOf(env, "TERM=tmux-256color")
	patched := indexOf(env, "TERM=xterm-256color")
	if inherited < 0 {
		t.Fatalf("inherited TERM=tmux-256color not present: %v", env)
	}
	if patched < 0 {
		t.Fatalf("patch TERM=xterm-256color not present: %v", env)
	}
	if patched <= inherited {
		t.Errorf("TERM patch at index %d must come after inherited TERM at %d", patched, inherited)
	}
	if indexOf(env, "AWS_PROFILE=dev") < 0 {
		t.Errorf("profile var AWS_PROFILE=dev not present: %v", env)
	}
}

// TestSpawnEnv_ProfileTermOverridesPatch pins #18: exec dedups a duplicated
// key keeping the LAST occurrence, so an explicit TERM in the profile must be
// the final TERM= entry, winning over the appended patch.
func TestSpawnEnv_ProfileTermOverridesPatch(t *testing.T) {
	t.Setenv("TERM", "tmux-256color")

	env := spawnEnv(map[string]string{"TERM": "screen"})

	last := ""
	for _, e := range env {
		if strings.HasPrefix(e, "TERM=") {
			last = e
		}
	}
	if last != "TERM=screen" {
		t.Errorf("last TERM entry = %q, want TERM=screen (env=%v)", last, env)
	}
}

func indexOf(env []string, want string) int {
	for i, e := range env {
		if e == want {
			return i
		}
	}
	return -1
}
