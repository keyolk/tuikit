package tuitest

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Reversed returns the text drawn in reverse video in frame, one entry per
// run, in reading order, with escapes stripped. A run ends at the end of a
// line, so a highlight over three rows is three entries.
//
// It is the assertion a selection or cursor test needs: "these cells are
// highlighted, and nothing else". Reverse is SGR 7; SGR 0 or 27 ends it. A
// 7 inside an extended color (38;5;7) is a color, not reverse.
func Reversed(frame string) []string {
	var runs []string
	for _, sp := range ReversedSpans(frame) {
		runs = append(runs, sp.Text)
	}
	return runs
}

// Span is one reverse-video run: the frame line it is on, its cells
// [X0, X1) counted the way lipgloss counts them, and its text.
type Span struct {
	Row, X0, X1 int
	Text        string
}

// ReversedSpans is Reversed with positions, for a test that must say where
// the highlight is, not only what it covers: a cursor on the second of two
// identical rows, or a selection that must start at a pane's left edge.
func ReversedSpans(frame string) []Span {
	var spans []Span
	var cur, line strings.Builder // the open run; the line's plain text so far
	on := false
	row, x0 := 0, 0
	flush := func() {
		if cur.Len() > 0 {
			spans = append(spans, Span{Row: row, X0: x0, X1: ansi.StringWidth(line.String()), Text: cur.String()})
			cur.Reset()
		}
	}
	for i := 0; i < len(frame); {
		c := frame[i]
		switch {
		case c == 0x1b && i+1 < len(frame) && frame[i+1] == '[':
			j := i + 2
			for j < len(frame) && (frame[j] < 0x40 || frame[j] > 0x7e) {
				j++
			}
			if j < len(frame) && frame[j] == 'm' {
				if next := sgrReverse(frame[i+2:j], on); next != on {
					if next {
						x0 = ansi.StringWidth(line.String())
					} else {
						flush()
					}
					on = next
				}
			}
			i = j + 1
		case c == 0x1b && i+1 < len(frame) && frame[i+1] == ']':
			// OSC (hyperlinks, titles): ends at BEL or ESC \.
			j := i + 2
			for j < len(frame) && frame[j] != 0x07 && !(frame[j] == 0x1b && j+1 < len(frame) && frame[j+1] == '\\') {
				j++
			}
			if j < len(frame) && frame[j] == 0x1b {
				j++
			}
			i = j + 1
		case c == 0x1b:
			i += 2
		case c == '\n':
			flush()
			row++
			line.Reset()
			x0 = 0
			i++
		default:
			if on {
				cur.WriteByte(c)
			}
			line.WriteByte(c)
			i++
		}
	}
	flush()
	return spans
}

// sgrReverse applies one SGR parameter list to the reverse state.
func sgrReverse(params string, on bool) bool {
	if params == "" {
		return false
	}
	ps := strings.Split(params, ";")
	for k := 0; k < len(ps); k++ {
		switch ps[k] {
		case "", "0", "27":
			on = false
		case "7":
			on = true
		case "38", "48", "58":
			if k+1 < len(ps) && ps[k+1] == "5" {
				k += 2
			} else if k+1 < len(ps) && ps[k+1] == "2" {
				k += 4
			}
		}
	}
	return on
}
