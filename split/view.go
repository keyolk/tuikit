package split

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/render"
)

// Divider glyphs. The heavy pair marks a drag in progress, so the highlight
// survives NO_COLOR, where st's colors are dropped.
const (
	lineV, lineH         = "│", "─"
	lineVDrag, lineHDrag = "┃", "━"
)

// Divider draws the divider rect div for a split of orientation o: exactly
// div.W cells by div.H rows. Width 1 is `│`, 2 is `│` and a pad, 3 is ` │ `
// (the line sits at (n-1)/2), and the same across rows for Vertical. st
// styles the line only, never the pad; dragging draws it heavy and bold.
func Divider(o Orientation, div mouse.Rect, st lipgloss.Style, dragging bool) string {
	if div.W <= 0 || div.H <= 0 {
		return ""
	}
	v, h := lineV, lineH
	if dragging {
		v, h = lineVDrag, lineHDrag
		st = st.Bold(true)
	}
	rows := make([]string, div.H)
	if o == Vertical {
		mid := (div.H - 1) / 2
		for i := range rows {
			if i == mid {
				rows[i] = st.Render(strings.Repeat(h, div.W))
			} else {
				rows[i] = strings.Repeat(" ", div.W)
			}
		}
		return strings.Join(rows, "\n")
	}
	mid := (div.W - 1) / 2
	row := strings.Repeat(" ", mid) + st.Render(v) + strings.Repeat(" ", div.W-mid-1)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

// Look is how DrawDivider draws the divider.
type Look int

const (
	// LookLine draws the line always (Divider), heavy while dragging.
	LookLine Look = iota
	// LookBlank draws spaces while idle and the heavy line only while
	// dragging: a blank column between bordered panes (ccproxy) that
	// still shows where the drag is.
	LookBlank
)

// DrawDivider is Divider for m's orientation and drag state, in m's Look.
func (m Model) DrawDivider(div mouse.Rect, st lipgloss.Style) string {
	if m.Look == LookBlank && !m.dragging {
		return blank(div)
	}
	return Divider(m.Orientation, div, st, m.dragging)
}

// blank is div.H rows of div.W spaces.
func blank(div mouse.Rect) string {
	if div.W <= 0 || div.H <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", div.W)+"\n", div.H), "\n")
}

// Join lays a, the divider and b out in r by Layout(r): each is cut and
// padded to exactly its rect (render.FitLines, then padded), so every row
// of the result is r.W cells and there are r.H rows, whatever the panes
// drew. A hidden pane's text is dropped. The panes' own View code should
// still size to the rects; Join is the guarantee, not the layout.
func (m Model) Join(r mouse.Rect, a, div, b string) string {
	ra, rd, rb := m.Layout(r)
	if r.W <= 0 || r.H <= 0 {
		return ""
	}
	la, ld, lb := fit(a, ra), fit(div, rd), fit(b, rb)
	if m.Orientation == Vertical {
		return strings.Join(append(append(la, ld...), lb...), "\n")
	}
	rows := make([]string, r.H)
	for i := range rows {
		rows[i] = at(la, i) + at(ld, i) + at(lb, i)
	}
	return strings.Join(rows, "\n")
}

func fit(s string, r mouse.Rect) []string {
	if r.W <= 0 || r.H <= 0 {
		return nil
	}
	lines := strings.Split(render.FitLines(s, r.W, r.H), "\n")
	for i, l := range lines {
		lines[i] = render.PadRight(l, r.W)
	}
	return lines
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}
