package ui

import "github.com/oldwired/fv-go/pkg/fv/geom"

// CenterRect clamps a desired w×h box so it stays at least `margin`
// cells narrower/shorter than the desk-sized area (a ~margin/2-cell
// gutter on each side once centred) and centres it, so a dialog shrinks
// gracefully on tiny terminals instead of overflowing.
//
// desk is the desktop size (a view's Base.Size). The returned rect's
// Width()/Height() report the possibly-clamped size — pass those to
// child layout — and A is the centred top-left corner. Sizes are never
// negative and the origin is never off the top-left edge, so a zero or
// absurdly small desk yields a benign empty rect rather than a panic.
func CenterRect(desk geom.Point, w, h, margin int) geom.Rect {
	if maxW := desk.X - margin; w > maxW {
		w = maxW
	}
	if maxH := desk.Y - margin; h > maxH {
		h = maxH
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	x := (desk.X - w) / 2
	y := (desk.Y - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return geom.NewRect(x, y, x+w, y+h)
}
