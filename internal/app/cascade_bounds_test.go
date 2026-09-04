package app

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func TestCascadedRectAlwaysFitsDesktop(t *testing.T) {
	tests := []struct {
		name string
		size geom.Point
		w, h int
	}{
		{name: "normal", size: geom.Point{X: 120, Y: 40}, w: 80, h: 24},
		{name: "exact", size: geom.Point{X: 80, Y: 24}, w: 80, h: 24},
		{name: "oversized", size: geom.Point{X: 50, Y: 15}, w: 500, h: 200},
		{name: "tiny", size: geom.Point{X: 9, Y: 4}, w: 80, h: 24},
		{name: "single cell", size: geom.Point{X: 1, Y: 1}, w: 80, h: 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for n := 0; n <= 100; n++ {
				r := cascadedRect(tt.size, n, tt.w, tt.h)
				if r.A.X < 0 || r.A.Y < 0 || r.B.X > tt.size.X || r.B.Y > tt.size.Y {
					t.Fatalf("n=%d rect=%v escapes desktop %v", n, r, tt.size)
				}
				if r.Width() <= 0 || r.Height() <= 0 {
					t.Fatalf("n=%d rect=%v is empty", n, r)
				}
			}
		})
	}
}

func TestCascadedRectEmptyDesktop(t *testing.T) {
	for _, size := range []geom.Point{{}, {X: 80}, {Y: 24}} {
		if got := cascadedRect(size, 10, 80, 24); got.Width() != 0 || got.Height() != 0 {
			t.Fatalf("size=%v returned non-empty rect %v", size, got)
		}
	}
}

func TestCascadedRectWrapsWithStableSize(t *testing.T) {
	size := geom.Point{X: 100, Y: 30}
	first := cascadedRect(size, 0, 60, 18)
	moved := false
	for n := 1; n <= 100; n++ {
		r := cascadedRect(size, n, 60, 18)
		if r.Width() != first.Width() || r.Height() != first.Height() {
			t.Fatalf("n=%d changed window size from %dx%d to %dx%d", n,
				first.Width(), first.Height(), r.Width(), r.Height())
		}
		if r.A != first.A {
			moved = true
		}
	}
	if !moved {
		t.Error("cascade never offset a window despite available space")
	}
}
