package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oldwired/fvmux/internal/config"
)

func isolatedCheckPaths(t *testing.T) config.Paths {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	return config.Paths{Root: root, StateRoot: filepath.Join(root, "state")}
}

func TestCheckConfigAcceptsMissingOptionalFiles(t *testing.T) {
	paths := isolatedCheckPaths(t)
	var out bytes.Buffer
	if err := checkConfig(paths, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "configuration valid" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCheckConfigReportsBindingErrors(t *testing.T) {
	paths := isolatedCheckPaths(t)
	body := `
[[binding]]
chord = "<prefix> HyperDrive"
command = "Missing command"
`
	if err := os.WriteFile(paths.KeybindingsFile(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := checkConfig(paths, &out)
	if err == nil {
		t.Fatal("invalid binding should make --check-config fail")
	}
	if got := out.String(); !strings.Contains(got, "ERROR keybindings.toml") || !strings.Contains(got, "unknown key atom") {
		t.Fatalf("diagnostic output = %q", got)
	}
}

func TestCheckConfigReportsUnknownCommandAfterSyntaxValidation(t *testing.T) {
	paths := isolatedCheckPaths(t)
	body := `
[[binding]]
chord = "<prefix> a"
command = "Missing command"
`
	if err := os.WriteFile(paths.KeybindingsFile(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := checkConfig(paths, &out); err == nil {
		t.Fatal("unknown command should make --check-config fail")
	}
	if !strings.Contains(out.String(), "unknown command name") {
		t.Fatalf("diagnostic output = %q", out.String())
	}
}
