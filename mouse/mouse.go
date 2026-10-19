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
	c := Click{Index: -1}
	if !r.Contains(msg.X, msg.Y) {
		return c
	}
	c.Visible = true
	start, _ := l.Window()
	local := msg.Y - r.Y
	if local < 0 || local >= r.Rows() {
		return c
	}
	idx := start + local
	if idx < 0 || idx >= l.Len() {
		return c
	}
	c.Index = idx
	c.HasItem = true
	return c
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
