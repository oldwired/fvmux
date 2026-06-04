package app

import (
	"testing"
	"time"
)

func TestSetFlash_PriorityProtectsActiveHigherFlash(t *testing.T) {
	m := &Mux{}

	// Empty slot: anything sets.
	m.setFlash("[1] a  [2] b", 1500*time.Millisecond, flashPrioNumbers)
	if m.flashText != "[1] a  [2] b" {
		t.Fatalf("numbers flash not set: %q", m.flashText)
	}

	// Higher-priority "ship it" overwrites the active lower-priority list.
	m.setFlash("ship it", 4*time.Second, flashPrioShipIt)
	if m.flashText != "ship it" {
		t.Fatalf("ship-it should overwrite numbers; got %q", m.flashText)
	}

	// Lower-priority numbers must NOT clobber the still-active ship-it.
	m.setFlash("[1] x", 1500*time.Millisecond, flashPrioNumbers)
	if m.flashText != "ship it" {
		t.Fatalf("numbers must not cut short active ship-it; got %q", m.flashText)
	}
}

func TestSetFlash_ExpiredSlotIsReplaceable(t *testing.T) {
	m := &Mux{}
	m.setFlash("ship it", 4*time.Second, flashPrioShipIt)
	// Force the high-priority flash to have already elapsed.
	m.flashUntil = time.Now().Add(-time.Second)

	m.setFlash("[1] x", 1500*time.Millisecond, flashPrioNumbers)
	if m.flashText != "[1] x" {
		t.Fatalf("expired flash should be replaceable by lower priority; got %q", m.flashText)
	}
}
