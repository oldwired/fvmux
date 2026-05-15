package sshmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHostsTOML_MissingFileIsNotError(t *testing.T) {
	hosts, err := loadHostsTOML(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should yield (nil, nil); got err=%v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(hosts))
	}
}

func TestLoadHostsTOML_EmptyPathReturnsNil(t *testing.T) {
	hosts, err := loadHostsTOML("")
	if err != nil || hosts != nil {
		t.Fatalf("empty path should yield (nil, nil); got %v / err=%v", hosts, err)
	}
}

func TestLoadHostsTOML_ParsesEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.toml")
	body := `
[[host]]
alias = "prod"
user  = "deploy"
host  = "prod.example"
port  = 2222
tags  = ["primary", "asia"]
notes = "main API box"

[[host]]
alias = "default-port"
host  = "elsewhere.example"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	hosts, err := loadHostsTOML(path)
	if err != nil {
		t.Fatalf("loadHostsTOML: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	prod := findHost(hosts, "prod")
	if prod == nil {
		t.Fatal("missing 'prod' entry")
	}
	if prod.User != "deploy" || prod.Hostname != "prod.example" || prod.Port != "2222" {
		t.Errorf("prod fields wrong: %+v", prod)
	}
	if prod.Source != "hosts.toml" {
		t.Errorf("source: got %q want hosts.toml", prod.Source)
	}
	if len(prod.Tags) != 2 || prod.Tags[0] != "primary" {
		t.Errorf("tags: %+v", prod.Tags)
	}

	def := findHost(hosts, "default-port")
	if def == nil || def.Port != "22" {
		t.Fatalf("default-port should default to 22, got %+v", def)
	}
}

func TestPool_ReleaseRefcount(t *testing.T) {
	p := NewPool(func(alias string) string { return "/tmp/socket-" + alias })

	// Inject a master directly — Acquire would otherwise fork ssh.
	p.mu.Lock()
	p.masters["host1"] = &master{alias: "host1", sockPath: "/tmp/socket-host1", refcount: 3}
	p.mu.Unlock()

	p.Release("host1")
	p.Release("host1")
	if got := p.masters["host1"].refcount; got != 1 {
		t.Fatalf("refcount after 2 releases: got %d want 1", got)
	}

	// Releasing past zero is a no-op, not a panic / negative count.
	p.Release("host1")
	p.Release("host1")
	p.Release("host1")
	if got := p.masters["host1"].refcount; got < 0 {
		t.Fatalf("refcount went negative: %d", got)
	}
}

func TestPool_ReleaseUnknownIsNoop(t *testing.T) {
	p := NewPool(func(alias string) string { return "" })
	p.Release("doesnt-exist") // must not panic
}

func TestPool_SnapshotMatchesMasters(t *testing.T) {
	p := NewPool(func(alias string) string { return "/tmp/socket-" + alias })
	p.mu.Lock()
	p.masters["a"] = &master{alias: "a", sockPath: "/tmp/socket-a", refcount: 1}
	p.masters["b"] = &master{alias: "b", sockPath: "/tmp/socket-b", refcount: 2}
	p.mu.Unlock()

	snap := p.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot length: got %d want 2", len(snap))
	}
	byAlias := map[string]ActiveConn{}
	for _, c := range snap {
		byAlias[c.Alias] = c
	}
	if byAlias["a"].Refs != 1 || byAlias["b"].Refs != 2 {
		t.Fatalf("snapshot refs wrong: %+v", byAlias)
	}
}

func findHost(hosts []*Host, alias string) *Host {
	for _, h := range hosts {
		if h.Alias == alias {
			return h
		}
	}
	return nil
}
