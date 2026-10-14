package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// reset closes whatever style was open at a seam, so the base's color does not
// run into the box and the box's color does not run into the base.
const reset = "\x1b[0m"

// Overlay paints box over base with its top-left corner at cell (x, y),
// keeping the base visible around it (E8). lipgloss.Place alone draws the
// modal on a blank screen, and the user loses what the modal is about.
//
// Guarantees:
//   - base keeps its line count; box rows outside base are not drawn.
//   - no line grows past the width of the widest base line: the box is
//     clipped at that edge.
//   - a wide character (Hangul, emoji) straddling either seam is replaced by a
//     space instead of shifting the rest of the line — argx found that
//     ansi.Cut keeps a wide character that starts one cell before the cut,
//     which pushed every line under a modal one cell past the terminal.
//   - styles are reset at both seams; the base's tail keeps its own style
//     because ansi.Cut carries the SGR state into the cut.
func Overlay(base, box string, x, y int) string {
	rows := strings.Split(base, "\n")
	frameW := 0
	for _, r := range rows {
		frameW = max(frameW, ansi.StringWidth(r))
	}
	x = max(0, x)
	if x >= frameW {
		return base
	}
	for i, line := range strings.Split(box, "\n") {
		r := y + i
		if r < 0 || r >= len(rows) {
			continue
		}
		rows[r] = splice(rows[r], line, x, frameW)
	}
	return strings.Join(rows, "\n")
}

// splice replaces cells [x, x+width(box)) of row with box, clipped at frameW.
func splice(row, box string, x, frameW int) string {
	box = ansi.Truncate(box, frameW-x, "")
	boxW := ansi.StringWidth(box)
	rowW := ansi.StringWidth(row)

	head := ansi.Cut(row, 0, x)
	if ansi.StringWidth(head) > x { // defensive: never let the head overrun
		head = ansi.Truncate(head, x, "")
	}
	// A wide character straddling x is dropped by Cut; the gap becomes space.
	head += strings.Repeat(" ", x-ansi.StringWidth(head))

	end := x + boxW
	tail := ""
	if rowW > end {
		want := rowW - end
		start := end
		tail = ansi.Cut(row, start, rowW)
		// Cut includes a wide character that begins one cell before start;
		// step past it and fill its visible half with a space.
		for ansi.StringWidth(tail) > want && start < rowW {
			start++
			tail = ansi.Cut(row, start, rowW)
		}
		tail = strings.Repeat(" ", want-ansi.StringWidth(tail)) + tail
	}
	return head + reset + box + reset + tail
}

// Center paints box over the middle of base, which is w cells by h lines.
// base is first fitted to exactly that size, so the result is always w×h no
// matter what the caller's frame looked like.
func Center(base, box string, w, h int) string {
	bw, bh := 0, strings.Count(box, "\n")+1
	for _, l := range strings.Split(box, "\n") {
		bw = max(bw, ansi.StringWidth(l))
	}
	base = fitPadded(base, w, h)
	return Overlay(base, box, max(0, (w-bw)/2), max(0, (h-bh)/2))
}

// fitPadded is FitLines with every line padded to w, so the overlay has a
// full-width frame to splice into.
func fitPadded(s string, w, h int) string {
	lines := strings.Split(FitLines(s, w, h), "\n")
	for i, l := range lines {
		if d := w - ansi.StringWidth(l); d > 0 {
			lines[i] = l + strings.Repeat(" ", d)
		}
	}
	return strings.Join(lines, "\n")
}
