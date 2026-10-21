// Package split is a resizable two-pane layout: the geometry, the ratio and
// the gestures that change it, with no rendering of pane content.
//
// Three fleet apps resized panes three ways before this package: ghx kept a
// clamped percentage, ccx a ratio plus a mouse drag that grabbed cells of
// both panes, and narwhal an unclamped column delta that grew a dead zone
// (presses that changed nothing visible) and saved it. The contract here,
// meant to be ported as is to the Rust apps:
//
//   - The ratio is a fraction of the space left after the divider, not a
//     cell count, so it survives a terminal resize.
//   - Every write clamps it (Grow, Shrink, Set, Reset, a drag, Restore):
//     to [Lo, Hi], and, when the area is known, to what the minimum pane
//     sizes allow at that size. A key press always moves the divider by at
//     least one cell, or changes nothing and says so.
//   - Layout keeps the divider's cells out of both pane rects, so hit tests
//     and text selection on the panes never see a divider press.
//   - Mouse events go to the split before selection.Update: a press on the
//     divider is the split's (Result.Consumed), a press in a pane never is.
//   - Keys are `>` grow, `<` shrink, `=` reset (Bindings); a double click on
//     the divider resets too.
package split

import (
	"math"

	"github.com/keyolk/tuikit/mouse"
)

// Orientation is the direction the two panes sit in.
type Orientation int

const (
	// Horizontal puts A left of B; sizes are columns.
	Horizontal Orientation = iota
	// Vertical puts A above B; sizes are rows.
	Vertical
)

// Side names one pane.
type Side int

const (
	A Side = iota // left or top
	B             // right or bottom
)

// Defaults for the zero fields of Model.
const (
	DefaultRatio = 0.5
	DefaultStep  = 0.05
	DefaultLo    = 0.1
	DefaultHi    = 0.9
)

// Model is the split's configuration and state, a value kept on the app's
// model. Set the exported fields in a literal; the ratio and the drag are
// changed only through methods, which clamp.
type Model struct {
	Orientation Orientation
	// MinA and MinB are the smallest sizes, in cells, of each pane. A pane
	// is never laid out narrower than 1 cell, whatever these say.
	MinA, MinB int
	// Divider is the divider's size in cells (0 to 3 in the fleet). Its
	// cells belong to neither pane.
	Divider int
	// Collapse is the size below which only Primary is shown, full size.
	// Zero never collapses by size; a split whose minimums do not fit
	// collapses anyway.
	Collapse int
	// Primary is the pane shown when the split collapses: the focused one.
	Primary Side
	// Step is the fraction one key press moves the divider (DefaultStep
	// when zero).
	Step float64
	// Grab widens the divider's press target by this many cells on each
	// side, taking them from the panes for presses only (ccx grabbed ±1).
	// Zero, the default, keeps every pane cell the pane's.
	Grab int
	// Default is the ratio Reset returns to (DefaultRatio when zero).
	Default float64
	// Lo and Hi bound the ratio (DefaultLo and DefaultHi when zero).
	Lo, Hi float64

	ratio    float64 // 0 means Default
	dragging bool
	grabOff  int // pointer offset inside the divider when the drag began
	clicks   mouse.Clicks
}

// Result says what the split did with an event or an action.
type Result struct {
	// Consumed: the event was the split's. Skip selection and the app's
	// own click handling for it.
	Consumed bool
	// Changed: the ratio changed. Save prefs (State).
	Changed bool
}

func (m Model) bounds() (lo, hi float64) {
	lo, hi = m.Lo, m.Hi
	if lo <= 0 || lo >= 1 {
		lo = DefaultLo
	}
	if hi <= 0 || hi >= 1 {
		hi = DefaultHi
	}
	return lo, max(lo, hi)
}

func (m Model) clampBounds(r float64) float64 {
	lo, hi := m.bounds()
	return min(max(r, lo), hi)
}

func (m Model) def() float64 {
	d := m.Default
	if d <= 0 || d >= 1 || math.IsNaN(d) {
		d = DefaultRatio
	}
	return m.clampBounds(d)
}

// Ratio is the fraction of the space after the divider that A gets.
func (m Model) Ratio() float64 {
	if m.ratio == 0 {
		return m.def()
	}
	return m.clampBounds(m.ratio)
}

// Dragging reports whether a divider drag is in progress (draw the divider
// highlighted).
func (m Model) Dragging() bool { return m.dragging }

func (m Model) size(r mouse.Rect) int {
	if m.Orientation == Vertical {
		return r.H
	}
	return r.W
}

// cells is the range of A's size that leaves both panes their minimum, and
// the space the two panes share. ok is false when they do not both fit.
func (m Model) cells(r mouse.Rect) (lo, hi, avail int, ok bool) {
	avail = m.size(r) - max(m.Divider, 0)
	lo, hi = max(m.MinA, 1), avail-max(m.MinB, 1)
	return lo, hi, avail, r.W > 0 && r.H > 0 && lo <= hi
}

// Collapsed reports whether r shows only Primary: r is under Collapse, or
// too small for both minimums and the divider.
func (m Model) Collapsed(r mouse.Rect) bool {
	_, _, _, ok := m.cells(r)
	return !ok || (m.Collapse > 0 && m.size(r) < m.Collapse)
}

// clampFor clamps v to the bounds and, when r fits both panes, to the
// ratios whose layout respects the minimums at r: a stored ratio is then
// always one the screen shows, so the next key press moves the divider.
func (m Model) clampFor(v float64, r mouse.Rect) float64 {
	v = m.clampBounds(v)
	if lo, hi, avail, ok := m.cells(r); ok && !m.Collapsed(r) {
		v = min(max(v, float64(lo)/float64(avail)), float64(hi)/float64(avail))
	}
	return v
}

// sizeA is A's size in cells in r, for a split that is not collapsed.
func (m Model) sizeA(r mouse.Rect) int {
	lo, hi, avail, _ := m.cells(r)
	return min(max(int(math.Round(m.Ratio()*float64(avail))), lo), hi)
}

// Layout cuts r into pane A, the divider and pane B. The divider's cells are
// in neither pane. A hidden pane (collapsed split) is the zero Rect, which
// contains no point; so is the divider then. A zero-size divider is a rect
// of zero width (or height) at the seam, for Grab.
func (m Model) Layout(r mouse.Rect) (a, div, b mouse.Rect) {
	if r.W <= 0 || r.H <= 0 {
		return mouse.Rect{}, mouse.Rect{}, mouse.Rect{}
	}
	if m.Collapsed(r) {
		if m.Primary == B {
			return mouse.Rect{}, mouse.Rect{}, r
		}
		return r, mouse.Rect{}, mouse.Rect{}
	}
	_, _, avail, _ := m.cells(r)
	na, d := m.sizeA(r), max(m.Divider, 0)
	if m.Orientation == Vertical {
		return mouse.Rect{X: r.X, Y: r.Y, W: r.W, H: na},
			mouse.Rect{X: r.X, Y: r.Y + na, W: r.W, H: d},
			mouse.Rect{X: r.X, Y: r.Y + na + d, W: r.W, H: avail - na}
	}
	return mouse.Rect{X: r.X, Y: r.Y, W: na, H: r.H},
		mouse.Rect{X: r.X + na, Y: r.Y, W: d, H: r.H},
		mouse.Rect{X: r.X + na + d, Y: r.Y, W: avail - na, H: r.H}
}

// Set stores ratio v, clamped for area (pass the zero Rect when the size is
// unknown: the bounds still apply). NaN and infinities are ignored.
func (m Model) Set(v float64, area mouse.Rect) Model {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return m
	}
	m.ratio = m.clampFor(v, area)
	return m
}

// Reset returns to Default, clamped for area.
func (m Model) Reset(area mouse.Rect) Model { return m.Set(m.def(), area) }

// Grow moves the divider one Step toward B, at least one cell.
func (m Model) Grow(area mouse.Rect) Model { return m.nudge(+1, area) }

// Shrink moves the divider one Step toward A, at least one cell.
func (m Model) Shrink(area mouse.Rect) Model { return m.nudge(-1, area) }

func (m Model) nudge(dir int, area mouse.Rect) Model {
	step := m.Step
	if step <= 0 || math.IsNaN(step) {
		step = DefaultStep
	}
	_, _, avail, ok := m.cells(area)
	if !ok || m.Collapsed(area) {
		return m.Set(m.Ratio()+float64(dir)*step, mouse.Rect{})
	}
	// Step from what the screen shows, not from the stored ratio, and by
	// at least one cell: a step smaller than a cell on a narrow terminal
	// would otherwise be a press that does nothing.
	cur := m.sizeA(area)
	next := int(math.Round((float64(cur)/float64(avail) + float64(dir)*step) * float64(avail)))
	if next == cur {
		next = cur + dir
	}
	return m.Set(float64(next)/float64(avail), area)
}
