package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// BoxContent is the content size of a w×h box drawn with st: what is left
// after st's border, padding and margin. Size the body with it before
// rendering (RenderHelp, a list), so the caller never hard-codes "the border
// adds 4 columns and 2 rows" (dots). Both are at least 0.
func BoxContent(w, h int, st lipgloss.Style) (cw, ch int) {
	return max(0, w-st.GetHorizontalFrameSize()), max(0, h-st.GetVerticalFrameSize())
}

// Box draws body in a box of exactly w×h cells with st's border, padding and
// margin, and title spliced into the top border (`╭─ title ──╮`). body is
// fitted to the content size (BoxContent) first: extra lines and cells are
// cut, missing ones padded, so the box never grows with its body.
//
// st's own Width/Height/MaxWidth/MaxHeight are ignored: lipgloss counts
// padding but not the border in Width, and Box owns the outer size. A title
// is drawn only when st has a top border. When w or h is smaller than the
// frame, the result is still w×h: blank lines of w spaces.
func Box(title, body string, w, h int, st lipgloss.Style) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	cw, ch := BoxContent(w, h, st)
	if cw == 0 || ch == 0 || cw+st.GetHorizontalFrameSize() != w || ch+st.GetVerticalFrameSize() != h {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", w)+"\n", h), "\n")
	}
	lines := strings.Split(FitLines(body, cw, ch), "\n")
	for i, l := range lines {
		lines[i] = PadRight(l, cw)
	}
	st = st.UnsetWidth().UnsetHeight().UnsetMaxWidth().UnsetMaxHeight()
	out := strings.Split(st.Render(strings.Join(lines, "\n")), "\n")
	if title != "" && st.GetBorderTop() {
		top := st.GetMarginTop()
		x := st.GetMarginLeft() + 2 // "╭─ title ─╮": one border cell after the corner
		// Up to one border cell before the right corner.
		if avail := w - st.GetMarginRight() - 2 - x; top < len(out) && avail >= 3 {
			t := " " + Truncate(title, avail-2) + " "
			out[top] = splice(out[top], t, x, w)
		}
	}
	return strings.Join(out, "\n")
}
