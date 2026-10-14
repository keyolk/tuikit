// Package list is a cursor over items addressed by ID (engine principle E2),
// with the scroll window kept in the value.
//
// A cursor stored as a row index points at a different item after every
// refresh that inserts, removes or re-sorts rows: brd opened the wrong board
// after a sync, okx deleted the row that slid under the cursor. Every
// adopter re-wrote "remember the ID, find it again after replacing the
// items" by hand (brd, okx, argx, dots); List does it once.
//
// List is a value. The item slice is never written in place — SetItems
// copies its input — so copies of a model share it safely.
package list

import "github.com/keyolk/tuikit/render"

// Item is anything with a stable identity.
type Item interface{ ID() string }

// List is items plus a cursor held by ID and a scroll offset. The zero value
// is an empty list.
type List[T Item] struct {
	items  []T
	id     string // the selected item's ID; "" when empty
	idx    int    // its row, derived from id
	off    int    // first visible row
	height int    // rows the list draws items in
}

// New returns a list of a copy of items with the first selected.
func New[T Item](items []T) List[T] { return List[T]{}.SetItems(items) }

// SetItems replaces the items (copied) and keeps the selection by ID. When
// the selected item is gone, the cursor stays on the same row, clamped —
// the item that took its place — rather than jumping to the top. An empty
// list before selects the first item.
func (l List[T]) SetItems(items []T) List[T] {
	l.items = append([]T(nil), items...)
	if i := l.find(l.id); i >= 0 {
		return l.at(i)
	}
	return l.at(l.idx)
}

func (l List[T]) find(id string) int {
	if id == "" {
		return -1
	}
	for i, it := range l.items {
		if it.ID() == id {
			return i
		}
	}
	return -1
}

// at selects row i (clamped) and scrolls minimally to keep it visible.
func (l List[T]) at(i int) List[T] {
	if len(l.items) == 0 {
		l.id, l.idx, l.off = "", 0, 0
		return l
	}
	l.idx = max(0, min(i, len(l.items)-1))
	l.id = l.items[l.idx].ID()
	l.off = l.window()
	return l
}

func (l List[T]) window() int {
	_, _, off := render.Window(len(l.items), l.height, l.idx, l.off)
	return off
}

// Len is the number of items.
func (l List[T]) Len() int { return len(l.items) }

// At is item i; it panics when i is out of range, like a slice.
func (l List[T]) At(i int) T { return l.items[i] }

// Items returns a copy of the items.
func (l List[T]) Items() []T { return append([]T(nil), l.items...) }

// Selected is the item under the cursor; ok is false when the list is empty.
func (l List[T]) Selected() (item T, ok bool) {
	if len(l.items) == 0 {
		return item, false
	}
	return l.items[l.idx], true
}

// SelectedID is the selected item's ID, "" when empty. Persist this, not
// Index (prefs).
func (l List[T]) SelectedID() string { return l.id }

// Index is the selected row, -1 when empty.
func (l List[T]) Index() int {
	if len(l.items) == 0 {
		return -1
	}
	return l.idx
}

// Select moves the cursor to the item with id. ok is false, and l is
// unchanged, when no item has it.
func (l List[T]) Select(id string) (List[T], bool) {
	i := l.find(id)
	if i < 0 {
		return l, false
	}
	return l.at(i), true
}

// SelectIndex moves the cursor to row i, clamped.
func (l List[T]) SelectIndex(i int) List[T] { return l.at(i) }

// Move moves the cursor by delta rows, clamped at both ends (no wrap).
func (l List[T]) Move(delta int) List[T] { return l.at(l.idx + delta) }

// Top and Bottom move to the first and last row.
func (l List[T]) Top() List[T]    { return l.at(0) }
func (l List[T]) Bottom() List[T] { return l.at(len(l.items) - 1) }

// HalfDown and HalfUp move half a page (ctrl+d / ctrl+u), at least one row.
func (l List[T]) HalfDown() List[T] { return l.Move(max(1, l.height/2)) }
func (l List[T]) HalfUp() List[T]   { return l.Move(-max(1, l.height/2)) }

// PageDown and PageUp move a full page, at least one row.
func (l List[T]) PageDown() List[T] { return l.Move(max(1, l.height)) }
func (l List[T]) PageUp() List[T]   { return l.Move(-max(1, l.height)) }

// SetHeight sets how many rows the list draws items in — the pane height
// minus any header and More lines — and re-scrolls to keep the cursor
// visible. Call it from Update on tea.WindowSizeMsg and whenever the chrome
// around the list changes, so View draws with an offset Update kept (E1).
func (l List[T]) SetHeight(h int) List[T] {
	l.height = max(0, h)
	l.off = l.window()
	return l
}

// Height is the rows set by SetHeight.
func (l List[T]) Height() int { return l.height }

// Window is the rows [start, end) to draw.
func (l List[T]) Window() (start, end int) {
	start = l.window()
	return start, min(len(l.items), start+l.height)
}

// Hidden is how many items are above and below the window, for render.More
// or render.MoreLine.
func (l List[T]) Hidden() (above, below int) {
	start, end := l.Window()
	return start, len(l.items) - end
}

// MoreLine is render.MoreLine for the current window: "↑ 3 ↓ 12 more".
func (l List[T]) MoreLine() string { return render.MoreLine(l.Hidden()) }
