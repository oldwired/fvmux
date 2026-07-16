// Package ui holds small presentation helpers shared across fvmux's UI
// surfaces (status bar, SFTP browser, dialogs). They exist so the same
// rune-safe truncation and modal-prompt logic isn't re-derived — and
// re-broken — in each package.
package ui

// TruncRight returns s shortened to at most max runes, appending "…" as
// the final rune when it has to drop anything. It is rune-safe: a
// multi-byte rune is never split, so 🏠- or CJK-prefixed strings never
// produce mojibake. max <= 0 yields "".
func TruncRight(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// TruncLeftPath returns path shortened to at most max runes by dropping
// leading runes, so the tail (the most specific path segments) stays
// visible. When anything is dropped it prefixes "…"; for max <= 3 there
// isn't room for the ellipsis, so it just keeps the last max runes.
// Rune-safe. max <= 0 yields "".
func TruncLeftPath(path string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(path)
	if len(r) <= max {
		return path
	}
	if max <= 3 {
		return string(r[len(r)-max:])
	}
	return "…" + string(r[len(r)-(max-1):])
}
