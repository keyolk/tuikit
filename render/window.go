package render

import "fmt"

// Window returns the slice [start, end) of a list of total rows to draw in
// height rows so that cursor stays visible, and the offset to keep for the
// next frame. Scrolling is minimal: the window moves only when the cursor
// would leave it, so the list does not jump on every keypress. Out-of-range
// cursor and offset values are clamped.
//
// Callers that draw More lines must pass the height left after reserving
// them.
func Window(total, height, cursor, offset int) (start, end, newOffset int) {
	if total <= 0 || height <= 0 {
		return 0, 0, 0
	}
	cursor = max(0, min(cursor, total-1))
	offset = max(0, min(offset, total-1))
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+height {
		offset = cursor - height + 1
	}
	offset = max(0, min(offset, total-height))
	return offset, min(total, offset+height), offset
}

// More returns the `↑ N more` / `↓ N more` lines for a window with above rows
// hidden before it and below after it; an empty string means nothing is
// hidden on that side. When height (the rows the list has, including these
// lines) is under 3 there is no room for two markers, so both counts collapse
// into bottom and top is empty.
//
// The spec named this More(above, below); height was added because the
// collapse rule needs it.
func More(above, below, height int) (top, bottom string) {
	if above > 0 {
		top = fmt.Sprintf("↑ %d more", above)
	}
	if below > 0 {
		bottom = fmt.Sprintf("↓ %d more", below)
	}
	if height < 3 && above > 0 {
		top, bottom = "", fmt.Sprintf("↑ %d more", above)
		if below > 0 {
			bottom = fmt.Sprintf("↑ %d ↓ %d more", above, below)
		}
	}
	return top, bottom
}
