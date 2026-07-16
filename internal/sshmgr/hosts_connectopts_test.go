package sshmgr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Regression tests for finding #17: hosts.toml entries' user/hostname/
// port are honoured on every connect path via Host.ConnectOpts, while
// ssh_config-sourced hosts contribute nothing (ssh applies their fields).

func TestConnectOpts_NilHostIsNil(t *testing.T) {
	var h *Host
	if got := h.ConnectOpts(); got != nil {
		t.Errorf("nil *Host ConnectOpts = %v, want nil", got)
	}
}

func TestConnectOpts_SSHConfigSourceReturnsNil(t *testing.T) {
	// Even fully populated, an ssh_config host must yield nil — re-passing
	// its fields could fight Match blocks, and ssh applies them itself.
	h := &Host{
		Source:   "ssh_config",
		Hostname: "cfg.example",
		User:     "root",
		Port:     "2200",
	}
	if got := h.ConnectOpts(); got != nil {
		t.Errorf("ssh_config-sourced ConnectOpts = %v, want nil", got)
	}
}

func TestConnectOpts_HostsTOMLFullSet(t *testing.T) {
	h := &Host{
		Source:   "hosts.toml",
		Hostname: "prod-1.internal",
		User:     "deploy",
		Port:     "22",
	}
	want := []string{
		"-o", "HostName=prod-1.internal",
		"-o", "User=deploy",
		"-o", "Port=22",
	}
	if got := h.ConnectOpts(); !reflect.DeepEqual(got, want) {
		t.Errorf("ConnectOpts = %v, want %v", got, want)
	}
}

func TestConnectOpts_OmitsEmptyAndZeroFields(t *testing.T) {
	// Only Hostname set → only HostName emitted.
	if got, want := (&Host{Source: "hosts.toml", Hostname: "only-host.example"}).ConnectOpts(),
		[]string{"-o", "HostName=only-host.example"}; !reflect.DeepEqual(got, want) {
		t.Errorf("hostname-only ConnectOpts = %v, want %v", got, want)
	}
	// Port "0" and "" are both omitted (a bogus Port=0 would break ssh).
	for _, port := range []string{"0", ""} {
		got := (&Host{Source: "hosts.toml", User: "u", Port: port}).ConnectOpts()
		want := []string{"-o", "User=u"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("port %q ConnectOpts = %v, want %v (port omitted)", port, got, want)
		}
	}
	// Everything empty → nil.
	if got := (&Host{Source: "hosts.toml"}).ConnectOpts(); got != nil {
		t.Errorf("all-empty hosts.toml ConnectOpts = %v, want nil", got)
	}
}

// TestConnectOpts_FromLoadedHostsTOML is the integration leg: parse a
// hosts.toml through the package loader (README example shape) and assert
// the resulting Host's ConnectOpts carries the file's HostName/User/Port.
func TestConnectOpts_FromLoadedHostsTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.toml")
	body := `
[[host]]
alias = "prod-1"
user  = "deploy"
host  = "prod-1.internal"
port  = 22
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	hosts, err := loadHostsTOML(path)
	if err != nil {
		t.Fatalf("loadHostsTOML: %v", err)
	}
	h := findHost(hosts, "prod-1")
	if h == nil {
		t.Fatal("prod-1 not parsed from hosts.toml")
	}
	want := []string{
		"-o", "HostName=prod-1.internal",
		"-o", "User=deploy",
		"-o", "Port=22",
	}
	if got := h.ConnectOpts(); !reflect.DeepEqual(got, want) {
		t.Errorf("loaded prod-1 ConnectOpts = %v, want %v", got, want)
	}
}
