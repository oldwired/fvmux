package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"

	"github.com/oldwired/fvmux/internal/config"
	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/sshmgr"
)

// muxWithHost builds a headless Mux whose hosts.toml declares one "web1"
// alias and whose real sshPool resolves sockets into a sandboxed temp
// dir. No PTYs or ssh subprocesses are spawned — Acquire only records
// intent — so these tests exercise the refcount pairing (#26/#50) without
// a network.
func muxWithHost(t *testing.T) *Mux {
	t.Helper()
	dir := t.TempDir()
	paths := config.Paths{Root: dir, StateRoot: filepath.Join(dir, "state")}
	hosts := "[[host]]\n" +
		"alias = \"web1\"\n" +
		"host = \"web1.internal\"\n" +
		"user = \"deploy\"\n" +
		"port = 2022\n"
	if err := os.WriteFile(paths.HostsFile(), []byte(hosts), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Mux{}
	m.Opts.Paths = paths
	m.Opts.Config = config.Defaults() // instantiateProfile reads Terminal defaults.
	m.sshPool = sshmgr.NewPool(paths.ControlSocket)
	return m
}

// poolRefs is the current refcount the pool tracks for alias (0 when the
// alias was never acquired or has been fully released).
func poolRefs(m *Mux, alias string) int {
	for _, c := range m.sshPool.Snapshot() {
		if c.Alias == alias {
			return c.Refs
		}
	}
	return 0
}

// TestSSHRefcountPairing pins the Acquire/Release pairing that #26/#50
// fixed: sshProfile acquires exactly one ref and hands ownership to the
// spawned pane via SSHAlias; stopPane releases it exactly once (even when
// the pane's Term is nil), and a repeat stopPane is a no-op.
func TestSSHRefcountPairing(t *testing.T) {
	m := muxWithHost(t)

	host := m.hostByAlias("web1")
	if host == nil {
		t.Fatal("web1 not resolved from hosts.toml")
	}
	prof := m.sshProfile(host, "web1")

	if got := poolRefs(m, "web1"); got != 1 {
		t.Fatalf("after sshProfile, web1 refs = %d, want 1", got)
	}
	if prof.SSHAlias != "web1" {
		t.Errorf("prof.SSHAlias = %q, want web1", prof.SSHAlias)
	}
	if !slices.Contains(prof.Args, "web1") {
		t.Errorf("prof.Args = %v, want to contain the alias web1", prof.Args)
	}
	// The hosts.toml connection fields ride along as -o overrides so a
	// standalone host is actually dial-able.
	for _, want := range []string{"HostName=web1.internal", "User=deploy", "Port=2022"} {
		if !slices.Contains(prof.Args, want) {
			t.Errorf("prof.Args = %v, want to contain %q", prof.Args, want)
		}
	}

	// The spawned pane owns the ref. A nil Term must still release (the
	// pool bookkeeping is independent of whether the PTY ever came up).
	pane := &session.Pane{SSHAlias: "web1"}
	m.stopPane(pane)
	if got := poolRefs(m, "web1"); got != 0 {
		t.Fatalf("after stopPane, web1 refs = %d, want 0", got)
	}
	if pane.SSHAlias != "" {
		t.Errorf("stopPane left SSHAlias = %q, want cleared", pane.SSHAlias)
	}

	// stopPane can legitimately run twice for the same pane (doClose then
	// cleanupWindow's leaf walk). The second call must not double-release.
	m.stopPane(pane)
	if got := poolRefs(m, "web1"); got != 0 {
		t.Fatalf("second stopPane drove web1 refs to %d, want 0 (no double release / negative)", got)
	}
}

// TestInstantiateProfileReleasesOnFailure covers the spawn-failure leg of
// the pairing: when the terminal fails to start, instantiateProfile must
// release the ref sshProfile acquired instead of leaking it.
func TestInstantiateProfileReleasesOnFailure(t *testing.T) {
	m := muxWithHost(t)
	host := m.hostByAlias("web1")
	prof := m.sshProfile(host, "web1")
	if got := poolRefs(m, "web1"); got != 1 {
		t.Fatalf("precondition: web1 refs = %d, want 1", got)
	}

	// Force terminal.Start to fail — a non-existent command can't exec.
	prof.Command = "/nonexistent/binary-xyz"
	pane, err := m.instantiateProfile(prof, geom.NewRect(0, 0, 80, 24))
	if err == nil {
		if pane != nil && pane.Term != nil {
			pane.Term.Stop()
		}
		t.Fatal("instantiateProfile spawned a non-existent binary without error")
	}
	if got := poolRefs(m, "web1"); got != 0 {
		t.Fatalf("after failed spawn, web1 refs = %d, want 0 (released on failure)", got)
	}
}

// TestResolveProfileFallback_SetsSSHAlias confirms the restore/respawn
// fallback threads through sshProfile: an aliased fallback carries
// SSHAlias and acquires one ref, so the spawned pane can later release it.
func TestResolveProfileFallback_SetsSSHAlias(t *testing.T) {
	m := muxWithHost(t)

	prof := m.resolveProfileFallback("web1", m.hostByAlias)
	if prof == nil {
		t.Fatal("resolveProfileFallback(web1) = nil")
	}
	if prof.SSHAlias != "web1" {
		t.Errorf("prof.SSHAlias = %q, want web1", prof.SSHAlias)
	}
	if got := poolRefs(m, "web1"); got != 1 {
		t.Fatalf("resolveProfileFallback should Acquire once; web1 refs = %d, want 1", got)
	}

	// Release through the pane the fallback profile would spawn, leaving
	// the pool clean (mirrors the real teardown path).
	m.stopPane(&session.Pane{SSHAlias: "web1"})
	if got := poolRefs(m, "web1"); got != 0 {
		t.Fatalf("after release, web1 refs = %d, want 0", got)
	}
}
