// Package mouse gives tuikit adopters the shared half of mouse support:
// terminal capture lifecycle, click-to-row hit testing against a list.List,
// wheel deltas, and a simple Rect type for multi-pane hit regions.
//
// It does not own the model. Each app still decides what a click means —
// select, mark, activate — because that depends on the app's layout and
// selection semantics (E2: selection is an ID, not an index). What it does
// remove is the fifth copy of "is this point inside my list, and if so which
// row, accounting for scroll offset" that every adopter would write by hand.
//
// Keyboard remains primary. Mouse is augmentation: the app's key dispatch,
// footer, help and binding table stay the source of truth. A click reaches
// the same model commands a key would.
package mouse

import (
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/keyolk/tuikit/list"
)

// Rect is a screen rectangle in terminal cells. The zero value is an empty
// rect that Contains nothing, so an unused pane (e.g. detail below 90 cols)
// safely rejects every click.
type Rect struct {
	X, Y, W, H int
}

// Contains reports whether the cell (x, y) is inside r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Rows is the number of drawable rows in r, clamped to non-negative.
func (r Rect) Rows() int { return max(0, r.H) }

// WheelRows is how many list rows one wheel notch moves. Three keeps the
// cursor under visual control without overshooting a short list.
const WheelRows = 3

// isWheel reports whether the mouse event is a wheel event.
func isWheel(msg tea.MouseMsg) bool {
	b := msg.Button
	return b == tea.MouseButtonWheelUp || b == tea.MouseButtonWheelDown ||
		b == tea.MouseButtonWheelLeft || b == tea.MouseButtonWheelRight
}

// ProgramOptions enables mouse capture for a Bubble Tea program. Pass the
// result to tea.NewProgram along with WithAltScreen; the program restores
// the terminal (including mouse mode) on exit.
//
// Cell motion (button events + drag) is enough for click and wheel. All-mode
// motion (MouseActionMotion with no button) is not enabled by default: it
// floods the event queue while the pointer is over the terminal and forces a
// redraw on every move, which breaks the 0-fps idle rule. Apps that need
// hover highlighting can append tea.WithMouseAllMotion themselves.
func ProgramOptions() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithMouseCellMotion()}
}

// Delta converts a wheel event to a signed row delta. Non-wheel events return
// 0, so a handler can call Delta for every MouseMsg and ignore the rest.
func Delta(msg tea.MouseMsg) int {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return -WheelRows
	case tea.MouseButtonWheelDown:
		return WheelRows
	default:
		return 0
	}
}

// IsPress reports whether msg is a button press (not release, motion, or
// wheel). Use it to gate click handlers so a drag does not act like a click
// on every cell it passes.
func IsPress(msg tea.MouseMsg) bool {
	return msg.Action == tea.MouseActionPress && !isWheel(msg)
}

// IsLeftPress reports whether msg is a left-button press.
func IsLeftPress(msg tea.MouseMsg) bool {
	return IsPress(msg) && msg.Button == tea.MouseButtonLeft
}

// IsRightPress reports whether msg is a right-button press.
func IsRightPress(msg tea.MouseMsg) bool {
	return IsPress(msg) && msg.Button == tea.MouseButtonRight
}

// IsMiddlePress reports whether msg is a middle-button press.
func IsMiddlePress(msg tea.MouseMsg) bool {
	return IsPress(msg) && msg.Button == tea.MouseButtonMiddle
}

// Click is the result of hit-testing a mouse event against a list region.
// Index is the item row in the underlying slice, not the visible row.
type Click struct {
	Index   int
	Visible bool // the point was inside the list's screen rect
	HasItem bool // the row maps to a real item (inside the list bounds)
}

// ListHit maps a mouse event to a row in a list.List drawn inside screen rect
// r. It accounts for the list's scroll offset so a click on the third visible
// row of a scrolled list hits the third item in the window, not the third
// item overall.
//
// The returned Click.Index is clamped to [0, l.Len()-1] when the point is
// inside the rect but past the last item (a short list with empty rows below);
// HasItem is false in that case, so the caller can ignore the click rather
// than selecting a phantom row.
func ListHit[T list.Item](l list.List[T], r Rect, msg tea.MouseMsg) Click {
	start, _ := l.Window()
	return RowHit(r, start, l.Len(), msg)
}

// RowHit maps a mouse event to an item in a one-line-per-item list drawn in
// r whose first visible item is offset, for apps that keep their own cursor
// instead of a list.List.
func RowHit(r Rect, offset, total int, msg tea.MouseMsg) Click {
	c := Click{Index: -1}
	if !r.Contains(msg.X, msg.Y) {
		return c
	}
	c.Visible = true
	idx := offset + msg.Y - r.Y
	if idx < 0 || idx >= total {
		return c
	}
	c.Index = idx
	c.HasItem = true
	return c
}

// BlockHit maps a mouse event to an item in a list whose items take several
// lines each — cards, entries with a group header — drawn in r starting at
// item first. heights[i] is the line count of item i as drawn. A click on a
// card's second line selects that card, not whatever item a one-line-per-row
// count would land on.
func BlockHit(r Rect, first int, heights []int, msg tea.MouseMsg) Click {
	c := Click{Index: -1}
	if !r.Contains(msg.X, msg.Y) {
		return c
	}
	c.Visible = true
	line := msg.Y - r.Y
	for i := max(0, first); i < len(heights); i++ {
		if line < heights[i] {
			c.Index = i
			c.HasItem = true
			return c
		}
		line -= heights[i]
	}
	return c
}

// SplitColumns divides r into n equal-width columns separated by gap cells,
// the arithmetic a board view uses to lay its columns out. The last column
// takes any remainder so the columns cover r exactly.
func SplitColumns(r Rect, n, gap int) []Rect {
	if n <= 0 {
		return nil
	}
	w := (r.W - gap*(n-1)) / n
	out := make([]Rect, n)
	x := r.X
	for i := range out {
		cw := w
		if i == n-1 {
			cw = r.X + r.W - x
		}
		out[i] = Rect{X: x, Y: r.Y, W: max(0, cw), H: r.H}
		x += w + gap
	}
	return out
}

// Column returns the index of the rect in cols containing (x, y), or -1.
func Column(cols []Rect, x, y int) int {
	for i, c := range cols {
		if c.Contains(x, y) {
			return i
		}
	}
	return -1
}

// DoubleClickWindow is how close two presses on the same target must be to
// count as a double click.
const DoubleClickWindow = 400 * time.Millisecond

// Clicks detects a double click on the same target. It is a value: keep it on
// the model and replace it with what Press returns, so Update stays pure.
//
// The fleet convention is that one click selects and a second click on the
// selected target activates — the mouse form of Enter. Activating on a single
// click made the first click on a row do something the user had not chosen.
type Clicks struct {
	key  string
	at   time.Time
	done bool
}

// Press records a press on target key at now and reports whether it completes
// a double click. A completed double click is not reused by a third press.
func (c Clicks) Press(key string, now time.Time) (Clicks, bool) {
	double := !c.done && key != "" && key == c.key && now.Sub(c.at) <= DoubleClickWindow
	return Clicks{key: key, at: now, done: double}, double
}

// SelectByClick returns a new list with the cursor moved to the clicked row,
// when the click hit a real item. The list is unchanged when the click missed
// (outside the rect or on an empty row below a short list).
func SelectByClick[T list.Item](l list.List[T], r Rect, msg tea.MouseMsg) (list.List[T], bool) {
	hit := ListHit(l, r, msg)
	if !hit.HasItem {
		return l, false
	}
	return l.SelectIndex(hit.Index), true
}

// MoveByWheel returns a new list moved by the wheel delta, or the list
// unchanged when the event is not a wheel event.
func MoveByWheel[T list.Item](l list.List[T], msg tea.MouseMsg) (list.List[T], bool) {
	d := Delta(msg)
	if d == 0 {
		return l, false
	}
	return l.Move(d), true
}
