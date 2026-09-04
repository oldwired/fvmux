package headless

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func repositoryFile(t *testing.T, rel string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	// These tests inspect workflow meaning, not the checkout's native newline
	// convention. Normalize CRLF so exact indentation checks are portable.
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func TestWorkflowActionsArePinnedToCommits(t *testing.T) {
	t.Parallel()
	uses := regexp.MustCompile(`uses:\s+[^@\s]+@([^\s#]+)`)
	commit := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, workflow := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml"} {
		body := repositoryFile(t, workflow)
		matches := uses.FindAllSubmatch(body, -1)
		if len(matches) == 0 {
			t.Fatalf("%s has no action references", workflow)
		}
		for _, match := range matches {
			if !commit.Match(match[1]) {
				t.Errorf("%s has mutable action reference %q", workflow, match[1])
			}
		}
	}
}

func TestReleaseTokenIsWriteScopedOnlyToAssembly(t *testing.T) {
	t.Parallel()
	body := string(repositoryFile(t, ".github/workflows/release.yml"))
	if !strings.Contains(body, "permissions:\n  contents: read") {
		t.Fatal("release workflow does not default to contents: read")
	}
	if strings.Count(body, "contents: write") != 1 {
		t.Fatalf("release workflow has %d contents: write grants, want 1", strings.Count(body, "contents: write"))
	}
	assemblyStart := strings.Index(body, "  assemble-release:")
	if assemblyStart < 0 {
		t.Fatal("release workflow has no assemble-release job")
	}
	assembly := body[assemblyStart:]
	if !strings.Contains(assembly, "    permissions:\n      contents: write") {
		t.Fatal("assemble-release job lacks its required job-local write grant")
	}
}

func TestReleaseBinariesEmbedTagVersion(t *testing.T) {
	t.Parallel()
	body := string(repositoryFile(t, ".github/workflows/release.yml"))
	if !strings.Contains(body, "FVMUX_VERSION: ${{ github.ref_name }}") {
		t.Fatal("release build does not source its version from the pushed tag")
	}
	if !strings.Contains(body, `-ldflags "-X main.Version=${FVMUX_VERSION}"`) {
		t.Fatal("release build does not embed the tag in main.Version")
	}
}

func TestSmokeSSHIsLoopbackKeyOnlyAndDigestPinned(t *testing.T) {
	t.Parallel()
	body := string(repositoryFile(t, "test/smoke/docker-compose.yml"))
	checks := []string{
		`127.0.0.1:2222:2222`,
		`PASSWORD_ACCESS: "false"`,
		`FVMUX_SMOKE_PUBLIC_KEY`,
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("smoke compose missing %q", want)
		}
	}
	if strings.Contains(body, "USER_PASSWORD") || strings.Contains(body, ":latest") {
		t.Fatal("smoke compose contains a fixed password or mutable latest tag")
	}
	pinnedImage := regexp.MustCompile(`image:\s+\S+@sha256:[0-9a-f]{64}\s`)
	if !pinnedImage.MatchString(body + "\n") {
		t.Fatal("smoke image is not pinned to a full SHA-256 digest")
	}
}
