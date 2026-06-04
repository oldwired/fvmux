package copymode

import "testing"

// The copy-mode driver is interactive glue over fv-go's terminal widget
// (every key branch delegates to a concrete *terminal.Terminal method
// with no injectable seam), so its behaviour is best exercised through
// the headless integration suite rather than brittle unit tests here.
// What we can pin down cheaply are the nil guards on the exported entry
// points, which must never panic.

func TestPaste_NilTerminal(t *testing.T) {
	if err := Paste(nil); err != nil {
		t.Fatalf("Paste(nil) = %v, want nil", err)
	}
}

func TestShow_NilTerminal(t *testing.T) {
	// Show(nil-app would deref, but a nil *terminal* must early-return
	// before touching the app, so passing nil for both is safe.
	Show(nil, nil)
}
