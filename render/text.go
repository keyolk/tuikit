// Package render holds cell-width-aware, ANSI-aware string helpers for View
// code: honest truncation (E7), overlays that paint over the frame instead of
// replacing it (E8), and per-pane panic isolation (E4).
//
// Every width here is in terminal cells, never bytes or runes: len() and rune
// counts both misplace columns after a Hangul or emoji name. Every cut goes
// through charmbracelet/x/ansi, because rune-slicing a styled string can split
// an SGR sequence and bleed color into the next line (okx truncate).
package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Width is the display width of s in cells, ignoring escape sequences.
//
// Plain printable ASCII is measured by its length without the grapheme
// walk: narwhal kept a local truncate for that fast path, worth 34ms per
// frame on a long list.
func Width(s string) int {
	if printableASCII(s) {
		return len(s)
	}
	return ansi.StringWidth(s)
}

// printableASCII reports whether every byte of s is in 0x20..0x7e, so one
// byte is one cell. ESC and other controls take the slow path.
func printableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// Truncate cuts s to at most w cells, ending in the default marker
// (Ellipsis) when anything was cut. Styled input is safe: sequences are never
// split and the trailing reset is kept. A wide character that would straddle
// the edge is dropped whole.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	return cut(s, w, Ellipsis())
}

// TruncateMarker is Truncate with an explicit marker ("~", "...", or "" for a
// hard cut). A marker wider than w is dropped, so the result still shows as
// much of s as fits rather than nothing.
func TruncateMarker(s string, w int, marker string) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	return cut(s, w, marker)
}

// cut assumes Width(s) > w > 0.
func cut(s string, w int, marker string) string {
	mw := Width(marker)
	if mw > w {
		marker, mw = "", 0
	}
	if printableASCII(s) {
		return s[:w-mw] + marker
	}
	return ansi.Truncate(s, w, marker)
}

// Pad returns s as exactly w cells: padded with spaces on the right, or
// truncated with the default marker when longer. Columns built with it line
// up whatever script the cell holds (fmt's %-20s pads by runes, so Hangul
// shifts the next column; lipgloss Style.Width wraps instead of padding).
func Pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return PadRight(Truncate(s, w), w)
}

// PadRight pads s with spaces on the right to w cells and never truncates: a
// longer s comes back unchanged. Use it where cutting would lose meaning (a
// value column the caller already sized) or after TruncateMarker.
func PadRight(s string, w int) string {
	if d := w - Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// PadLeft right-aligns s in w cells (numbers, counts) and never truncates.
func PadLeft(s string, w int) string {
	if d := w - Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// FitLines returns s as exactly h lines, each at most w cells: extra lines are
// dropped, missing lines are empty. Use it on a pane's output so a pane that
// misjudges its own size cannot push the footer off screen.
func FitLines(s string, w, h int) string {
	if h <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = Truncate(lines[i], w)
		}
	}
	return strings.Join(out, "\n")
}

// Dim strips styling from s and renders every non-empty line faint, for the
// frame under a modal: the box then dominates, and the plain text underneath
// is safe to cut by cells. Faint is an attribute, not a color, so it still
// works under NO_COLOR. Empty lines stay empty so no stray escapes pad them.
func Dim(s string) string {
	lines := strings.Split(ansi.Strip(s), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "\x1b[2m" + l + "\x1b[0m"
		}
	}
	return strings.Join(lines, "\n")
}
