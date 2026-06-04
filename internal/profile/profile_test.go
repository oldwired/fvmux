package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "shell" {
		t.Fatalf("missing file = %+v, want the [shell] default", got)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.toml")
	body := `
[[profile]]
name = "top"
command = "/usr/bin/htop"
args = ["-d", "5"]
close_on_exit = true

[[profile]]
name = "logs"
command = "tail"
args = ["-f", "/var/log/syslog"]
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d profiles, want 2", len(got))
	}
	if got[0].Name != "top" || got[0].Command != "/usr/bin/htop" || !got[0].CloseOnExit {
		t.Errorf("profile[0] = %+v", got[0])
	}
	if len(got[1].Args) != 2 || got[1].Args[1] != "/var/log/syslog" {
		t.Errorf("profile[1].Args = %v", got[1].Args)
	}
}

func TestLoad_MalformedReturnsDefaultsAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("this is = = not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err == nil {
		t.Fatal("malformed TOML should return an error")
	}
	if len(got) != 1 || got[0].Name != "shell" {
		t.Fatalf("malformed file should still yield defaults, got %+v", got)
	}
}

func TestLoad_EmptyProfileListFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.toml")
	if err := os.WriteFile(path, []byte("# no profiles here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].Name != "shell" {
		t.Fatalf("empty list should fall back to defaults, got %+v", got)
	}
}

func TestDefaults_UsesShellEnv(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	d := Defaults()
	if len(d) != 1 || d[0].Command != "/bin/zsh" {
		t.Fatalf("Defaults = %+v, want command /bin/zsh", d)
	}

	t.Setenv("SHELL", "")
	if d := Defaults(); d[0].Command != "/bin/sh" {
		t.Fatalf("Defaults with empty SHELL = %q, want /bin/sh", d[0].Command)
	}
}

func TestFind(t *testing.T) {
	list := []*Profile{{Name: "a"}, {Name: "b"}}
	if got := Find(list, "b"); got == nil || got.Name != "b" {
		t.Fatalf("Find(b) = %+v", got)
	}
	if got := Find(list, "missing"); got != nil {
		t.Fatalf("Find(missing) = %+v, want nil", got)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := expandPath("~/work"); got != filepath.Join(home, "work") {
		t.Errorf("expandPath(~/work) = %q, want %q", got, filepath.Join(home, "work"))
	}

	t.Setenv("FVMUX_TEST_DIR", "/srv/data")
	if got := expandPath("$FVMUX_TEST_DIR/sub"); got != "/srv/data/sub" {
		t.Errorf("expandPath($VAR/sub) = %q, want /srv/data/sub", got)
	}
	if got := expandPath(""); got != "" {
		t.Errorf("expandPath(\"\") = %q, want empty", got)
	}
}

func TestInstantiate_NilProfile(t *testing.T) {
	if _, err := Instantiate(nil, geom.NewRect(0, 0, 80, 24), 0, ""); err == nil {
		t.Fatal("Instantiate(nil) should error")
	}
}
