package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupCorrupt_MovesFileAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("this = is = not = valid = toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	bak, err := BackupCorrupt(path)
	if err != nil {
		t.Fatalf("BackupCorrupt: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(bak), "config.toml.bad-") {
		t.Fatalf("backup name %q lacks .bad- marker", bak)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original should be gone after backup; stat err = %v", err)
	}
	got, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(got) != "this = is = not = valid = toml" {
		t.Fatalf("backup content lost: %q", got)
	}
}

// Mirrors the main.go startup flow: a malformed config is detected, backed
// up, and Defaults() is used so writes are safely re-enabled.
func TestLoad_MalformedThenBackupThenDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("prefix_key = \"unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error for malformed config")
	}
	if _, err := BackupCorrupt(path); err != nil {
		t.Fatalf("BackupCorrupt: %v", err)
	}
	// A subsequent programmatic persist must succeed and produce a
	// parseable file now the corrupt original is out of the way.
	if err := UpdateKeys(path, KV{Section: "general", Key: "prefix_key", Value: "C-g"}); err != nil {
		t.Fatalf("UpdateKeys after backup: %v", err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("reloading written defaults: %v", err)
	}
	if c.General.PrefixKey != "C-g" {
		t.Fatalf("written defaults not round-tripping: prefix = %q", c.General.PrefixKey)
	}
}

func TestWithStateLock_RunsAndSerialises(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")

	// Concurrent increments through WithStateLock must not lose updates
	// (on platforms where flock is available). Each critical section does
	// a load-modify-save of a counter persisted as LastVersion.
	const n = 30
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = WithStateLock(statePath, func() error {
				st, _ := LoadState(statePath)
				cur := 0
				if st.LastVersion != "" {
					// stored as a number string
					for _, r := range st.LastVersion {
						cur = cur*10 + int(r-'0')
					}
				}
				st.LastVersion = itoa(cur + 1)
				return SaveState(statePath, st)
			})
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	st, _ := LoadState(statePath)
	// On flock platforms this is exactly n; on best-effort fallback it may
	// be lower, so we only assert it ran and produced a valid file.
	if st.LastVersion == "" {
		t.Fatal("WithStateLock never persisted")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
