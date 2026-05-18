package glue

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// expectedCount is the number of files Install writes on the host OS:
// fvmuxa + fvmux.tmux.conf everywhere; fvmuxa.cmd on Windows too.
func expectedCount() int {
	if runtime.GOOS == "windows" {
		return 3
	}
	return 2
}

func tmpLocs(t *testing.T) Locations {
	t.Helper()
	tmp := t.TempDir()
	return Locations{
		BinDir:  filepath.Join(tmp, "bin"),
		ConfDir: filepath.Join(tmp, "conf"),
	}
}

func TestInstall_WritesMissingFiles(t *testing.T) {
	locs := tmpLocs(t)
	wrote, err := locs.Install()
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got, want := len(wrote), expectedCount(); got != want {
		t.Errorf("wrote %d files (%v), want %d", got, wrote, want)
	}

	fi, err := os.Stat(filepath.Join(locs.BinDir, "fvmuxa"))
	if err != nil {
		t.Fatalf("fvmuxa not installed: %v", err)
	}
	// Permission bits don't round-trip through Windows file systems, so
	// only assert the executable bit elsewhere.
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755 {
		t.Errorf("fvmuxa perm = %o, want 0755", fi.Mode().Perm())
	}

	cfg, err := os.ReadFile(filepath.Join(locs.ConfDir, "fvmux.tmux.conf"))
	if err != nil {
		t.Fatalf("fvmux.tmux.conf not installed: %v", err)
	}
	if len(cfg) == 0 {
		t.Errorf("fvmux.tmux.conf is empty")
	}
}

// Install must leave hand-edited files alone. fvmux.tmux.conf is the
// likely target for user tweaks (status line, prefix, scrollback), so
// re-running the wizard cannot clobber those edits.
func TestInstall_LeavesExistingFilesAlone(t *testing.T) {
	locs := tmpLocs(t)
	if _, err := locs.Install(); err != nil {
		t.Fatalf("first install: %v", err)
	}

	conf := filepath.Join(locs.ConfDir, "fvmux.tmux.conf")
	const userEdit = "# user-edited contents\n"
	if err := os.WriteFile(conf, []byte(userEdit), 0o644); err != nil {
		t.Fatalf("simulating user edit: %v", err)
	}

	wrote, err := locs.Install()
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if len(wrote) != 0 {
		t.Errorf("second install wrote %v, want nothing", wrote)
	}

	got, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != userEdit {
		t.Errorf("Install clobbered user edits:\n got: %q\nwant: %q", got, userEdit)
	}
}

func TestMissing_AllAbsent(t *testing.T) {
	locs := tmpLocs(t)
	if got, want := len(locs.Missing()), expectedCount(); got != want {
		t.Errorf("Missing pre-install: %d, want %d", got, want)
	}
}

func TestMissing_NoneAfterInstall(t *testing.T) {
	locs := tmpLocs(t)
	if _, err := locs.Install(); err != nil {
		t.Fatal(err)
	}
	if got := locs.Missing(); len(got) != 0 {
		t.Errorf("Missing post-install: %v, want empty", got)
	}
}
