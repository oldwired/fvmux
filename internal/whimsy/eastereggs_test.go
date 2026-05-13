package whimsy

import (
	"testing"
	"time"
)

func TestFridayAfterFive(t *testing.T) {
	// Friday 17:00.
	friday5pm := time.Date(2026, 5, 15, 17, 0, 0, 0, time.UTC)
	if !FridayAfterFive(friday5pm) {
		t.Error("Friday 17:00 should match")
	}
	friday11am := time.Date(2026, 5, 15, 11, 0, 0, 0, time.UTC)
	if FridayAfterFive(friday11am) {
		t.Error("Friday 11:00 should not match")
	}
	thursday5pm := time.Date(2026, 5, 14, 17, 0, 0, 0, time.UTC)
	if FridayAfterFive(thursday5pm) {
		t.Error("Thursday 17:00 should not match")
	}
}

func TestHomeGlyph(t *testing.T) {
	if HomeGlyphFor("home") == "" {
		t.Error("home should get a glyph")
	}
	if HomeGlyphFor("Home") == "" {
		t.Error("home-case-insensitive should get a glyph")
	}
	if HomeGlyphFor("homework") != "" {
		t.Error("homework should not get a glyph")
	}
	if HomeGlyphFor("") != "" {
		t.Error("empty title should not get a glyph")
	}
}

func TestRot13(t *testing.T) {
	in := []byte("Hello, World! 123")
	want := "Uryyb, Jbeyq! 123"
	got := string(Rot13(in))
	if got != want {
		t.Errorf("Rot13 = %q want %q", got, want)
	}
	// Double-rot13 is identity.
	round := string(Rot13(Rot13(in)))
	if round != string(in) {
		t.Errorf("double Rot13 = %q want %q", round, string(in))
	}
}
