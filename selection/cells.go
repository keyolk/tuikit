package selection

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/kitty"
)

// wordChar is the engine's word class: letters, digits and the characters
// paths and flags are made of, so a double click takes a whole path.
var wordChar = regexp.MustCompile(`^[\p{L}\p{N}_/.\-+~\\]$`)

// wordAt returns the cell columns of the word covering column col.
func wordAt(line string, col int) (lo, hi int, ok bool) {
	type cell struct {
		r      string
		lo, hi int
	}
	var cells []cell
	x := 0
	for _, r := range line {
		w := max(ansi.StringWidth(string(r)), 1)
		cells = append(cells, cell{string(r), x, x + w - 1})
		x += w
	}
	at := -1
	for i, c := range cells {
		if col >= c.lo && col <= c.hi {
			at = i
			break
		}
	}
	if at < 0 || !wordChar.MatchString(cells[at].r) {
		return 0, 0, false
	}
	first, last := at, at
	for first > 0 && wordChar.MatchString(cells[first-1].r) {
		first--
	}
	for last < len(cells)-1 && wordChar.MatchString(cells[last+1].r) {
		last++
	}
	return cells[first].lo, cells[last].hi, true
}

// lineSpan is the cells a line pick takes: the line without its leading
// indent, which in a pane is layout ("  @ Room 7F"), not text.
func lineSpan(line string) (lo, hi int, ok bool) {
	body := strings.TrimLeft(line, " ")
	if strings.TrimRight(body, " ") == "" {
		return 0, 0, false
	}
	lo = len(line) - len(body) // spaces are one byte and one cell each
	return lo, ansi.StringWidth(strings.TrimRight(line, " ")) - 1, true
}

// textRun is a stretch of cells with no kitty placeholder in it, x cells
// from the start of the string it came from.
type textRun struct {
	x    int
	text string
}

// textRuns splits plain cells around kitty placeholder cells (U+10EEEE and
// the zero-width diacritics after it that carry the row and column).
func textRuns(cells string) []textRun {
	var runs []textRun
	var cur strings.Builder
	start, x, inPlaceholder := 0, 0, false
	flush := func() {
		if cur.Len() > 0 {
			runs = append(runs, textRun{x: start, text: cur.String()})
			cur.Reset()
		}
	}
	for _, r := range cells {
		w := ansi.StringWidth(string(r))
		switch {
		case r == kitty.PlaceholderRune:
			flush()
			inPlaceholder = true
		case w == 0 && inPlaceholder:
			// a diacritic of the placeholder before it
		default:
			if inPlaceholder || cur.Len() == 0 {
				flush()
				start = x
			}
			inPlaceholder = false
			cur.WriteRune(r)
		}
		x += w
	}
	flush()
	return runs
}

// stripPlaceholders drops kitty placeholder cells from copied text: they are
// a picture, and pasted they are U+10EEEE plus combining marks.
func stripPlaceholders(s string) string {
	if !strings.ContainsRune(s, kitty.PlaceholderRune) {
		return s
	}
	var b strings.Builder
	for _, run := range textRuns(s) {
		b.WriteString(run.text)
	}
	return b.String()
}

// plainLine is row of p without styles, as cells of the pane only.
func plainLine(p Pane, row int, frame *lazyFrame) string {
	if p.Lines != nil {
		if row < 0 || row >= len(p.Lines) {
			return ""
		}
		return ansi.Cut(ansi.Strip(p.Lines[row]), p.LineX, p.LineX+p.Rect.W)
	}
	rows := frame.lines()
	y := p.Rect.Y + row
	if y < 0 || y >= len(rows) {
		return ""
	}
	return ansi.Strip(ansi.Cut(rows[y], p.Rect.X, p.Rect.X+p.Rect.W))
}

// pointIn maps a screen cell to a point of p, clamped to the pane: columns
// never leave it (the engine's scope), rows stay within its lines. A press
// below the last line of a short pane lands on its last line; a drag above a
// pane that cannot scroll lands on row 0, at the press column's clamp.
func pointIn(p Pane, x, y int) Point {
	col := min(max(x-p.Rect.X, 0), max(p.Rect.W-1, 0))
	row := y - p.Rect.Y
	if p.Lines != nil {
		row = min(max(row+p.Offset, 0), max(len(p.Lines)-1, 0))
	} else {
		row = min(max(row, 0), max(p.Rect.H-1, 0))
	}
	return Point{Row: row, Col: col}
}

func paneAt(panes []Pane, x, y int) (Pane, bool) {
	for i := len(panes) - 1; i >= 0; i-- {
		if panes[i].Rect.Contains(x, y) {
			return panes[i], true
		}
	}
	return Pane{}, false
}

func paneByID(panes []Pane, id string) (Pane, bool) {
	for _, p := range panes {
		if p.ID == id {
			return p, true
		}
	}
	return Pane{}, false
}
