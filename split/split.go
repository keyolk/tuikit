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
//     The split's press clears the selection (Result.Pressed).
//   - Keys are `>` grow, `<` shrink, `=` reset (Bindings); a double click on
//     the divider resets too.
//   - A split nobody has moved is unset (IsSet false): it follows Default,
//     or DefaultFor of the area, and State persists nothing for it. Reset
//     returns it to unset, so "follow the width rule until moved" needs no
//     flag on the app's side.
//
// The zero Model is usable: two panes 50/50, each at least one cell, no
// divider cell and so no mouse target (Grow, Shrink and Reset still work).
// Give it a Divider, or GrabA/GrabB, for a drag.
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
	// MaxA and MaxB cap each pane's size in cells; zero is no cap. A cap
	// holds in Layout and in every write that knows the area (a list never
	// wider than 70 columns, ccproxy). The minimums win over a cap, and when
	// MaxA and MaxB cannot both hold, MaxA does and B takes the rest.
	MaxA, MaxB int
	// Grab widens the divider's press target by this many cells on each
	// side, taking them from the panes for presses only (ccx grabbed ±1).
	// Zero, the default, keeps every pane cell the pane's.
	Grab int
	// GrabA and GrabB widen the press target into A and into B only. When
	// either is set, both are used and Grab is ignored: a zero-width
	// divider (gcl's bottom split) is dragged by A's last row with GrabA 1
	// and leaves B's first row to B. Grab n is GrabA n, GrabB n.
	GrabA, GrabB int
	// Default is the ratio Reset returns to (DefaultRatio when zero).
	Default float64
	// DefaultFor, when set, gives the ratio of an unset split laid out in
	// an area, in place of Default: a ratio derived from the width ("a
	// third, at most 52 columns", narwhal). It is clamped like any ratio,
	// and an answer not strictly between 0 and 1 falls back to Default.
	// Ratio, which has no area, still reports Default; use RatioIn.
	//
	// It is an interface so Model stays comparable with == (gcl and kkt
	// compare against split.Model{}). A struct Defaulter keeps == working
	// between two Models; DefaultFunc, a func, makes == between two Models
	// that both hold one panic, so compare those with IsZero or by field.
	DefaultFor Defaulter
	// Lo and Hi bound the ratio (DefaultLo and DefaultHi when zero).
	Lo, Hi float64
	// Look is how DrawDivider draws the divider (LookLine when zero).
	Look Look

	ratio    float64 // 0 means unset: Default, or DefaultFor(area)
	dragging bool
	grabOff  int // pointer offset inside the divider when the drag began
	clicks   mouse.Clicks
}

// Result says what the split did with an event or an action.
type Result struct {
	// Consumed: the event was the split's. Skip selection and the app's
	// own click handling for it.
	Consumed bool
	// Changed: the ratio, or whether it is set, changed. Save prefs (State).
	Changed bool
	// Pressed: the event was a left press the split took (a drag start or
	// a double click). The selection never sees it, so clear it here:
	// `if r.Pressed { m.sel.Clear() }`; otherwise its highlight stays over
	// cells the resize is about to move.
	Pressed bool
	// Reset: the split was reset, by a double click on the divider or by
	// ActionReset.
	Reset bool
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

// Defaulter derives an unset split's ratio from the area it is laid out in.
type Defaulter interface {
	DefaultRatio(area mouse.Rect) float64
}

// DefaultFunc is a func as a Defaulter:
//
//	DefaultFor: split.DefaultFunc(func(r mouse.Rect) float64 { return 1.0 / 3 })
type DefaultFunc func(area mouse.Rect) float64

// DefaultRatio calls f.
func (f DefaultFunc) DefaultRatio(area mouse.Rect) float64 { return f(area) }

// defIn is the unset ratio in area: DefaultFor's when it gives a fraction.
func (m Model) defIn(area mouse.Rect) float64 {
	if m.DefaultFor != nil && area.W > 0 && area.H > 0 {
		if d := m.DefaultFor.DefaultRatio(area); d > 0 && d < 1 {
			return m.clampBounds(d)
		}
	}
	return m.def()
}

// Ratio is the fraction of the space after the divider that A gets. For an
// unset split it is Default; RatioIn also applies DefaultFunc.
func (m Model) Ratio() float64 {
	if m.ratio == 0 {
		return m.def()
	}
	return m.clampBounds(m.ratio)
}

// RatioIn is Ratio for a split laid out in area: an unset split's ratio
// comes from DefaultFor when there is one.
func (m Model) RatioIn(area mouse.Rect) float64 {
	if m.ratio == 0 {
		return m.defIn(area)
	}
	return m.clampBounds(m.ratio)
}

// IsSet reports whether the ratio was chosen (a key, a drag, Set, Restore)
// rather than left at the default. Reset clears it.
func (m Model) IsSet() bool { return m.ratio != 0 }

// IsZero reports whether m is the zero Model: no field set and no state.
// An app whose models are built by a constructor can swap a struct-literal
// zero split (a test model) for the constructor's.
func (m Model) IsZero() bool {
	// DefaultFor may hold a func, which == would panic on.
	return m.Orientation == 0 && m.MinA == 0 && m.MinB == 0 && m.Divider == 0 &&
		m.Collapse == 0 && m.Primary == 0 && m.Step == 0 && m.MaxA == 0 && m.MaxB == 0 &&
		m.Grab == 0 && m.GrabA == 0 && m.GrabB == 0 && m.Default == 0 && m.DefaultFor == nil &&
		m.Lo == 0 && m.Hi == 0 && m.Look == 0 &&
		m.ratio == 0 && !m.dragging && m.grabOff == 0 && m.clicks == (mouse.Clicks{})
}

// Dragging reports whether a divider drag is in progress (draw the divider
// highlighted).
func (m Model) Dragging() bool { return m.dragging }

// CancelDrag ends a drag in progress, keeping the ratio it reached: what a
// release would do, for an app that loses the pointer (a focus change, an
// overlay opening) and has no release to feed.
func (m Model) CancelDrag() Model {
	m.dragging = false
	return m
}

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
	ok = r.W > 0 && r.H > 0 && lo <= hi
	if !ok {
		return lo, hi, avail, false
	}
	// Caps narrow the range but never past the minimums, so they cannot
	// collapse a split. When MaxA and MaxB cannot both hold, lo ends above
	// hi; every caller clamps as min(max(x, lo), hi), so MaxA wins.
	if m.MaxB > 0 {
		lo = max(lo, avail-m.MaxB)
	}
	if m.MaxA > 0 {
		hi = max(min(hi, m.MaxA), max(m.MinA, 1))
	}
	return lo, hi, avail, true
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
	return min(max(int(math.Round(m.RatioIn(r)*float64(avail))), lo), hi)
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

// Reset returns the split to unset: Default, or DefaultFunc of the area it
// is laid out in, and nothing for State to persist. area is unused; it is
// kept for the callers written when Reset stored the default.
func (m Model) Reset(area mouse.Rect) Model {
	m.ratio = 0
	return m
}

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
	n := m.Set(float64(next)/float64(avail), area)
	if n.sizeA(area) == cur {
		// At a limit: leave an unset split unset rather than pin it to
		// the ratio it already shows.
		return m
	}
	return n
}

// state is what Changed compares: whether the ratio is set, and its value.
type state struct {
	set   bool
	ratio float64
}

func (m Model) stateIn(area mouse.Rect) state { return state{m.IsSet(), m.RatioIn(area)} }
