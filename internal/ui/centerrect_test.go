package ui

import (
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
)

func TestCenterRect(t *testing.T) {
	tests := []struct {
		name         string
		desk         geom.Point
		w, h, margin int
		wantX, wantY int
		wantW, wantH int
	}{
		{
			// Even desktop, even box: perfectly centred, no clamp.
			name: "even/even no clamp",
			desk: geom.Point{X: 100, Y: 40}, w: 40, h: 20, margin: 0,
			wantX: 30, wantY: 10, wantW: 40, wantH: 20,
		},
		{
			// Odd leftover cells floor (integer division) toward the
			// top-left corner, matching the old (desk-w)/2 arithmetic.
			name: "odd desk floors origin",
			desk: geom.Point{X: 101, Y: 41}, w: 40, h: 20, margin: 0,
			wantX: 30, wantY: 10, wantW: 40, wantH: 20,
		},
		{
			name: "odd box floors origin",
			desk: geom.Point{X: 11, Y: 5}, w: 4, h: 2, margin: 0,
			wantX: 3, wantY: 1, wantW: 4, wantH: 2,
		},
		{
			// Both dimensions overflow: clamp each to desk-margin, then
			// the leftover margin splits evenly to centre.
			name: "clamp both dims",
			desk: geom.Point{X: 20, Y: 10}, w: 40, h: 20, margin: 2,
			wantX: 1, wantY: 1, wantW: 18, wantH: 8,
		},
		{
			// Only width overflows; height is left untouched.
			name: "clamp width only",
			desk: geom.Point{X: 20, Y: 40}, w: 40, h: 20, margin: 2,
			wantX: 1, wantY: 10, wantW: 18, wantH: 20,
		},
		{
			// Margin is honored: a giant box shrinks to desk-margin in
			// both axes, leaving margin/2 gutter per side.
			name: "margin honored",
			desk: geom.Point{X: 100, Y: 40}, w: 200, h: 100, margin: 4,
			wantX: 2, wantY: 2, wantW: 96, wantH: 36,
		},
		{
			// Exactly at the clamp boundary: w == desk-margin stays put.
			name: "at clamp boundary",
			desk: geom.Point{X: 50, Y: 30}, w: 48, h: 28, margin: 2,
			wantX: 1, wantY: 1, wantW: 48, wantH: 28,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CenterRect(tc.desk, tc.w, tc.h, tc.margin)
			if got.A.X != tc.wantX || got.A.Y != tc.wantY {
				t.Errorf("origin = (%d,%d), want (%d,%d)",
					got.A.X, got.A.Y, tc.wantX, tc.wantY)
			}
			if got.Width() != tc.wantW || got.Height() != tc.wantH {
				t.Errorf("size = %dx%d, want %dx%d",
					got.Width(), got.Height(), tc.wantW, tc.wantH)
			}
		})
	}
}

// TestCenterRectDegenerate guards the pathological desktops the helper
// exists to tame: a zero or negative size must never panic or emit a
// negative-size / off-screen rect.
func TestCenterRectDegenerate(t *testing.T) {
	cases := []geom.Point{
		{X: 0, Y: 0},
		{X: -5, Y: -5},
		{X: 1, Y: 1},
		{X: 3, Y: 3}, // smaller than the margin below.
	}
	for _, desk := range cases {
		r := CenterRect(desk, 40, 20, 4)
		if r.Width() < 0 || r.Height() < 0 {
			t.Errorf("desk %+v: negative size %dx%d", desk, r.Width(), r.Height())
		}
		if r.A.X < 0 || r.A.Y < 0 {
			t.Errorf("desk %+v: off-screen origin (%d,%d)", desk, r.A.X, r.A.Y)
		}
	}
}

// TestCenterRectMatchesLegacy pins that, on a comfortably large desktop
// where nothing clamps, CenterRect reproduces the exact (desk-w)/2
// arithmetic every migrated call site used to inline — so the migration
// is byte-identical in the common case.
func TestCenterRectMatchesLegacy(t *testing.T) {
	desk := geom.Point{X: 200, Y: 60}
	for _, m := range []int{0, 2, 4} {
		for _, wh := range [][2]int{{40, 8}, {54, 12}, {70, 22}, {110, 32}} {
			w, h := wh[0], wh[1]
			wantX := (desk.X - w) / 2
			wantY := (desk.Y - h) / 2
			r := CenterRect(desk, w, h, m)
			if r.A.X != wantX || r.A.Y != wantY || r.Width() != w || r.Height() != h {
				t.Errorf("margin %d box %dx%d: got %+v, want origin (%d,%d) size %dx%d",
					m, w, h, r, wantX, wantY, w, h)
			}
		}
	}
}
