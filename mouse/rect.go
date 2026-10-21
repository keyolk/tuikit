package mouse

import "github.com/charmbracelet/lipgloss"

// Inset is r shrunk by the given cells on each side, never negative. Use it
// for the content of a framed pane instead of adding "+1 for the border" by
// hand, which drifts from what View draws.
func (r Rect) Inset(top, right, bottom, left int) Rect {
	out := Rect{X: r.X + left, Y: r.Y + top, W: r.W - left - right, H: r.H - top - bottom}
	if out.W <= 0 || out.H <= 0 {
		return Rect{X: out.X, Y: out.Y}
	}
	return out
}

// InsetStyle is the content rect of a box drawn over r with st: r minus
// st's margin, border and padding, the same frame render.Box and
// render.BoxContent subtract.
func (r Rect) InsetStyle(st lipgloss.Style) Rect {
	top := st.GetMarginTop() + st.GetBorderTopSize() + st.GetPaddingTop()
	right := st.GetMarginRight() + st.GetBorderRightSize() + st.GetPaddingRight()
	bottom := st.GetMarginBottom() + st.GetBorderBottomSize() + st.GetPaddingBottom()
	left := st.GetMarginLeft() + st.GetBorderLeftSize() + st.GetPaddingLeft()
	return r.Inset(top, right, bottom, left)
}

// Offset is r moved by dx columns and dy rows: a rect computed in a pane's
// own coordinates placed on the screen (a body under a banner row).
func (r Rect) Offset(dx, dy int) Rect {
	return Rect{X: r.X + dx, Y: r.Y + dy, W: r.W, H: r.H}
}

// SplitTop cuts the first n rows off r: chrome (a banner, a tab bar) and the
// body under it. n is clamped to r's height.
func (r Rect) SplitTop(n int) (top, rest Rect) {
	n = min(max(n, 0), max(r.H, 0))
	return Rect{X: r.X, Y: r.Y, W: r.W, H: n}, Rect{X: r.X, Y: r.Y + n, W: r.W, H: r.H - n}
}

// SplitBottom cuts the last n rows off r: a footer and the body above it.
// n is clamped to r's height.
func (r Rect) SplitBottom(n int) (rest, bottom Rect) {
	n = min(max(n, 0), max(r.H, 0))
	return Rect{X: r.X, Y: r.Y, W: r.W, H: r.H - n}, Rect{X: r.X, Y: r.Y + r.H - n, W: r.W, H: n}
}
