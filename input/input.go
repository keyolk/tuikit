// Package input is a single-line text field as a value: typing (Korean and
// pastes included), backspace, ctrl+u/ctrl+w/ctrl+k, cursor movement by rune
// and word, and a cell-width-aware view that keeps the cursor on screen.
//
// Every adopter hand-wrote this for its filter or pattern box (dots, okx,
// brd, argx), and each got some of it wrong: byte-indexed backspace that
// split a Hangul syllable, a cursor drawn at rune count instead of cell
// count, a paste of "a\nb" that broke the one-line layout.
//
// Field is a value. Every method returns a new Field and never writes into a
// slice another copy can see, so a model copied by Bubble Tea keeps its own
// text.
package input

import (
	"reflect"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/keys"
)

// Field is a one-line text field. The zero value is an empty field, ready to
// use.
type Field struct {
	runes []rune // never written in place; edits build a new slice
	pos   int    // cursor, in runes, 0..len(runes)
	// Limit caps the length in runes; 0 is unlimited. Typing and pastes past
	// it are cut.
	Limit int
	// Placeholder is shown, in Style.Placeholder, while the field is empty.
	Placeholder string
}

// New returns a field holding value with the cursor at the end.
func New(value string) Field { return Field{}.SetValue(value) }

// Value is the text.
func (f Field) Value() string { return string(f.runes) }

// Len is the length in runes.
func (f Field) Len() int { return len(f.runes) }

// Cursor is the cursor position in runes, 0..Len.
func (f Field) Cursor() int { return f.pos }

// SetValue replaces the text (sanitized as Insert does) and puts the cursor
// at the end.
func (f Field) SetValue(s string) Field {
	f.runes, f.pos = nil, 0
	return f.Insert(s)
}

// SetCursor moves the cursor to rune i, clamped to 0..Len.
func (f Field) SetCursor(i int) Field {
	f.pos = max(0, min(i, len(f.runes)))
	return f
}

// Insert types s at the cursor. A field is one line, so a pasted newline or
// tab becomes a space and other control characters (a stray \r, escape
// bytes) are dropped.
func (f Field) Insert(s string) Field {
	var add []rune
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			add = append(add, ' ')
		case unicode.IsControl(r):
		default:
			add = append(add, r)
		}
	}
	if f.Limit > 0 {
		add = add[:max(0, min(len(add), f.Limit-len(f.runes)))]
	}
	if len(add) == 0 {
		return f
	}
	out := make([]rune, 0, len(f.runes)+len(add))
	out = append(out, f.runes[:f.pos]...)
	out = append(out, add...)
	out = append(out, f.runes[f.pos:]...)
	f.runes, f.pos = out, f.pos+len(add)
	return f
}

// remove drops runes [from, to) into a new slice and leaves the cursor at
// from.
func (f Field) remove(from, to int) Field {
	from, to = max(0, from), min(len(f.runes), to)
	if from >= to {
		return f
	}
	out := make([]rune, 0, len(f.runes)-(to-from))
	out = append(out, f.runes[:from]...)
	out = append(out, f.runes[to:]...)
	f.runes, f.pos = out, from
	return f
}

// wordLeft is where a word-left move from the cursor lands: back over
// spaces, then over the word.
func (f Field) wordLeft() int {
	i := f.pos
	for i > 0 && unicode.IsSpace(f.runes[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(f.runes[i-1]) {
		i--
	}
	return i
}

func (f Field) wordRight() int {
	i := f.pos
	for i < len(f.runes) && unicode.IsSpace(f.runes[i]) {
		i++
	}
	for i < len(f.runes) && !unicode.IsSpace(f.runes[i]) {
		i++
	}
	return i
}

// Update applies one key and reports whether the field used it. Keys it
// does not use — enter, esc, up, down, tab, ctrl chords it has no meaning
// for — come back unhandled for the app to act on.
//
//	typed text, space, paste  insert (keys.TypedText)
//	backspace, ctrl+h         delete before the cursor
//	delete, ctrl+d            delete under the cursor
//	ctrl+w, alt+backspace     delete the word before the cursor
//	ctrl+u / ctrl+k           delete to the start / to the end
//	left/right, ctrl+b/ctrl+f one rune
//	alt+left/alt+right, alt+b/alt+f, ctrl+left/ctrl+right  one word
//	home/end, ctrl+a/ctrl+e   start / end
//
// Text is taken as typed, never CJK-normalized: Korean goes into the field.
func (f Field) Update(k tea.KeyMsg) (Field, bool) {
	switch k.String() {
	case "backspace", "ctrl+h":
		return f.remove(f.pos-1, f.pos), true
	case "delete", "ctrl+d":
		return f.remove(f.pos, f.pos+1), true
	case "ctrl+w", "alt+backspace":
		return f.remove(f.wordLeft(), f.pos), true
	case "ctrl+u":
		return f.remove(0, f.pos), true
	case "ctrl+k":
		return f.remove(f.pos, len(f.runes)), true
	case "left", "ctrl+b":
		return f.SetCursor(f.pos - 1), true
	case "right", "ctrl+f":
		return f.SetCursor(f.pos + 1), true
	case "alt+left", "alt+b", "ctrl+left":
		return f.SetCursor(f.wordLeft()), true
	case "alt+right", "alt+f", "ctrl+right":
		return f.SetCursor(f.wordRight()), true
	case "home", "ctrl+a":
		return f.SetCursor(0), true
	case "end", "ctrl+e":
		return f.SetCursor(len(f.runes)), true
	}
	if s, ok := keys.TypedText(k); ok {
		return f.Insert(s), true
	}
	return f, false
}

// Style styles View. The zero value draws plain text with a reverse-video
// cursor.
type Style struct {
	Text        lipgloss.Style
	Cursor      lipgloss.Style // zero means reverse video
	Placeholder lipgloss.Style
}

// cellW is the width of one rune. Per rune, not per grapheme: a ZWJ emoji
// sequence measures wider than it draws, which only makes the view scroll a
// little early.
func cellW(r rune) int { return ansi.StringWidth(string(r)) }

// View draws the field in exactly width cells. While focused the cursor is
// drawn, and when the text is wider than width the view scrolls so the
// cursor stays visible — computed from the text alone, so View needs no
// stored offset. Wide characters are never split at either edge.
func (f Field) View(width int, focused bool, st Style) string {
	if width <= 0 {
		return ""
	}
	cur := st.Cursor.Render
	if reflect.ValueOf(st.Cursor).IsZero() {
		// Raw SGR, not a lipgloss style: the default renderer drops reverse
		// when stdout is not a terminal, and a cursor must never vanish.
		cur = func(s ...string) string { return "\x1b[7m" + strings.Join(s, "") + "\x1b[27m" }
	}
	if len(f.runes) == 0 {
		var b strings.Builder
		used := 0
		if focused {
			b.WriteString(cur(" "))
			used = 1
		}
		ph := ansi.Truncate(f.Placeholder, width-used, "")
		if focused && ph != "" {
			// The cursor sits on the placeholder's first cell.
			b.Reset()
			first := []rune(ph)[0]
			b.WriteString(cur(string(first)))
			ph = string([]rune(ph)[1:])
			used = cellW(first)
		}
		b.WriteString(st.Placeholder.Render(ph))
		return b.String() + strings.Repeat(" ", max(0, width-used-ansi.StringWidth(ph)))
	}
	// The cursor cell: the rune under it, or one blank cell past the end.
	curW := 1
	if f.pos < len(f.runes) {
		curW = max(1, cellW(f.runes[f.pos]))
	}
	start := 0
	if focused {
		need := curW
		start = f.pos
		for start > 0 && need+cellW(f.runes[start-1]) <= width {
			start--
			need += cellW(f.runes[start])
		}
		// Show from the beginning when everything up to the cursor fits.
		if start > 0 {
			total := 0
			for _, r := range f.runes[:f.pos] {
				total += cellW(r)
			}
			if total+curW <= width {
				start = 0
			}
		}
	}
	var before, after strings.Builder
	used, cursorDrawn := 0, false
	for i := start; i < len(f.runes); i++ {
		w := cellW(f.runes[i])
		if used+w > width {
			break
		}
		if focused && i == f.pos {
			cursorDrawn = true
			used += max(1, w)
			continue
		}
		if cursorDrawn {
			after.WriteRune(f.runes[i])
		} else {
			before.WriteRune(f.runes[i])
		}
		used += w
	}
	out := st.Text.Render(before.String())
	switch {
	case cursorDrawn:
		out += cur(string(f.runes[f.pos])) + st.Text.Render(after.String())
	case focused && f.pos == len(f.runes) && used < width:
		out += cur(" ")
		used++
	}
	return out + strings.Repeat(" ", max(0, width-used))
}
