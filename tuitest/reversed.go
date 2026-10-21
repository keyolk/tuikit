package tuitest

import (
	"strings"
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
	var cur strings.Builder
	on := false
	flush := func() {
		if cur.Len() > 0 {
			runs = append(runs, cur.String())
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
					if !next {
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
			i++
		default:
			if on {
				cur.WriteByte(c)
			}
			i++
		}
	}
	flush()
	return runs
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
