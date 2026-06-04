package sshmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSSHConfig_FollowsInclude(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "config")
	mainBody := "Host main-box\n  HostName main.example\n\nInclude conf.d/*.conf\n"
	if err := os.WriteFile(main, []byte(mainBody), 0o644); err != nil {
		t.Fatal(err)
	}
	incBody := "Host included-box\n  HostName inc.example\n  User deploy\n"
	if err := os.WriteFile(filepath.Join(dir, "conf.d", "extra.conf"), []byte(incBody), 0o644); err != nil {
		t.Fatal(err)
	}

	hosts, err := loadSSHConfig(main)
	if err != nil {
		t.Fatalf("loadSSHConfig: %v", err)
	}
	if findHost(hosts, "main-box") == nil {
		t.Error("main host missing")
	}
	inc := findHost(hosts, "included-box")
	if inc == nil {
		t.Fatal("included host missing — Include not followed")
		return
	}
	if inc.Hostname != "inc.example" || inc.User != "deploy" {
		t.Errorf("included host fields wrong: %+v", inc)
	}
}

func TestLoadSSHConfig_IncludeLoopTerminates(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "config")
	b := filepath.Join(dir, "other")
	// a includes b, b includes a — must not hang or stack-overflow.
	if err := os.WriteFile(a, []byte("Host abox\n  HostName a\nInclude other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("Host bbox\n  HostName b\nInclude config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hosts, err := loadSSHConfig(a)
	if err != nil {
		t.Fatalf("loadSSHConfig: %v", err)
	}
	if findHost(hosts, "abox") == nil || findHost(hosts, "bbox") == nil {
		t.Errorf("expected both hosts, got %d", len(hosts))
	}
}

func TestLoadHostsTOML_SkipsEmptyAliasAndClampsPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.toml")
	body := `
[[host]]
alias = ""
host  = "noalias.example"

[[host]]
alias = "badport"
host  = "bp.example"
port  = 99999

[[host]]
alias = "dup"
host  = "first.example"

[[host]]
alias = "dup"
host  = "second.example"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	hosts, err := loadHostsTOML(path)
	if err != nil {
		t.Fatal(err)
	}
	if findHost(hosts, "") != nil {
		t.Error("empty-alias entry should have been skipped")
	}
	bp := findHost(hosts, "badport")
	if bp == nil || bp.Port != "22" {
		t.Errorf("out-of-range port should clamp to 22, got %+v", bp)
	}
	dup := findHost(hosts, "dup")
	if dup == nil || dup.Hostname != "first.example" {
		t.Errorf("intra-file duplicate should keep first definition, got %+v", dup)
	}
}
