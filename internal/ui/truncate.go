// Package ui holds small presentation helpers shared across fvmux's UI
// surfaces (status bar, SFTP browser, dialogs). They exist so the same
// width-safe truncation and modal-prompt logic isn't re-derived — and
// re-broken — in each package.
package ui

import (
	fvutf8 "github.com/oldwired/fv-go/pkg/fv/utf8"
)

// TruncRight returns s shortened to occupy at most max terminal CELLS,
// appending "…" as the final cell when it has to drop anything. Width
// is measured with fv-go's renderer model (uniseg grapheme clusters,
// UAX #11 wide runes count 2), so a CJK- or 🏠-prefixed label can't
// overflow the column the caller budgeted — a rune count would let
// twelve CJK characters "fit" a 12-cell slot while painting 24 cells.
// Grapheme clusters are never split. max <= 0 yields "".
func TruncRight(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if fvutf8.StringDisplayWidth(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return fvutf8.CopyDisplayCells(s, 0, max-1) + "…"
}

// TruncLeftPath returns path shortened to at most max terminal CELLS by
// dropping leading cells, so the tail (the most specific path segments)
// stays visible. When anything is dropped it prefixes "…"; for
// max <= 3 there isn't room for the ellipsis, so it just keeps the last
// max cells. Same cell-width model as TruncRight; a wide cluster that
// would straddle the cut is dropped whole, so the result never exceeds
// the budget. max <= 0 yields "".
func TruncLeftPath(path string, max int) string {
	if max <= 0 {
		return ""
	}
	w := fvutf8.StringDisplayWidth(path)
	if w <= max {
		return path
	}
	if max <= 3 {
		return fvutf8.CopyDisplayCells(path, w-max, max)
	}
	return "…" + fvutf8.CopyDisplayCells(path, w-(max-1), max-1)
}
