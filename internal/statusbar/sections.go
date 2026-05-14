package statusbar

import "strings"

// sparkChars renders [0,1] floats as Unicode block "spark" cells. Eight
// levels — '▁' (lowest) through '█' (highest); empty slots get a space.
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// renderSparkline renders history as up to width cells. Older samples
// are right-aligned (most recent at the rightmost cell) and missing
// samples (history shorter than width) become spaces on the left so
// the line settles into a stable position as samples accumulate.
func renderSparkline(history []float64, width int) string {
	if width <= 0 {
		return ""
	}
	var sb strings.Builder
	pad := width - len(history)
	for i := 0; i < pad; i++ {
		sb.WriteByte(' ')
	}
	for _, v := range history {
		idx := int(v * float64(len(sparkChars)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		sb.WriteRune(sparkChars[idx])
	}
	return sb.String()
}

// renderBar renders a [0,1] fraction as a left-aligned filled bar.
// Width counts both filled and empty cells.
func renderBar(v float64, width int) string {
	if width <= 0 {
		return ""
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	fill := int(v * float64(width))
	if fill > width {
		fill = width
	}
	var sb strings.Builder
	for i := 0; i < fill; i++ {
		sb.WriteRune('▓')
	}
	for i := fill; i < width; i++ {
		sb.WriteRune('░')
	}
	return sb.String()
}
