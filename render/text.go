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
func Width(s string) int { return ansi.StringWidth(s) }

// Truncate cuts s to at most w cells, ending in `…` when anything was cut.
// Styled input is safe: sequences are never split and the trailing reset is
// kept. A wide character that would straddle the edge is dropped whole.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// Pad returns s as exactly w cells: padded with spaces on the right, or
// truncated with `…` when longer. Columns built with it line up whatever
// script the cell holds (fmt's %-20s pads by runes, so Hangul shifts the next
// column; lipgloss Style.Width wraps instead of padding).
func Pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = Truncate(s, w)
	if d := w - ansi.StringWidth(s); d > 0 {
		s += strings.Repeat(" ", d)
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
