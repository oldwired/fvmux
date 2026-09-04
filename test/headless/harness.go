package headless

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGoldens is set by `go test ./test/headless/ -update=true` to
// rewrite every golden snapshot file rather than comparing. Mirrors
// fv-go's golden_test.go convention.
var updateGoldens = flag.Bool("update", false, "rewrite golden snapshots in test/headless/golden/")

// goldenPath returns the on-disk location for name's golden file.
func goldenPath(name string) string {
	return filepath.Join("golden", name+".txt")
}

// assertGolden compares actual against the on-disk golden for name.
// When -update is set the golden file is rewritten and the test
// passes. Trailing whitespace inside lines is preserved; leading +
// trailing blank lines are trimmed before compare so the on-disk
// files stay tidy.
func assertGolden(t *testing.T, name, actual string) {
	t.Helper()
	actual = strings.ReplaceAll(actual, "\r\n", "\n")
	actual = strings.TrimSuffix(actual, "\n")
	path := goldenPath(name)
	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(path, []byte(actual+"\n"), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	wantStr := strings.ReplaceAll(string(want), "\r\n", "\n")
	wantStr = strings.TrimSuffix(wantStr, "\n")
	if actual != wantStr {
		t.Errorf("golden %s mismatch:\n--- got ---\n%s\n--- want ---\n%s",
			name, actual, wantStr)
	}
}
