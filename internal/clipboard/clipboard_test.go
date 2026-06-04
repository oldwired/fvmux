package clipboard

import "testing"

// TestSupported just exercises the predicate — it returns whatever
// atotto detected at init for this environment (true on a desktop, false
// on a headless box without xclip/wl-copy). Either is valid.
func TestSupported(t *testing.T) {
	_ = Supported()
}

// TestRoundTrip verifies Set then Get returns the same text, but only
// where the OS clipboard is actually reachable — otherwise atotto's Get
// errors and the check is meaningless. The original clipboard contents
// are restored so running the suite doesn't clobber the user's clipboard.
func TestRoundTrip(t *testing.T) {
	if !Supported() {
		t.Skip("no OS clipboard in this environment")
	}
	orig, err := Get()
	if err != nil {
		t.Skipf("clipboard read unavailable: %v", err)
	}
	t.Cleanup(func() { _ = Set(orig) })

	const want = "fvmux-clipboard-test"
	if err := Set(want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %q, want %q", got, want)
	}
}
