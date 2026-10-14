package input

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func key(s string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"backspace": tea.KeyBackspace, "delete": tea.KeyDelete, "left": tea.KeyLeft, "right": tea.KeyRight,
		"home": tea.KeyHome, "end": tea.KeyEnd, "ctrl+u": tea.KeyCtrlU, "ctrl+w": tea.KeyCtrlW,
		"ctrl+k": tea.KeyCtrlK, "ctrl+a": tea.KeyCtrlA, "ctrl+e": tea.KeyCtrlE, "enter": tea.KeyEnter,
		"esc": tea.KeyEsc, "space": tea.KeySpace, "up": tea.KeyUp,
	}
	if t, ok := named[s]; ok {
		if t == tea.KeySpace {
			return tea.KeyMsg{Type: t, Runes: []rune{' '}}
		}
		return tea.KeyMsg{Type: t}
	}
	if rest, ok := strings.CutPrefix(s, "alt+"); ok {
		k := key(rest)
		k.Alt = true
		return k
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeKeys(f Field, ks ...string) Field {
	for _, k := range ks {
		f, _ = f.Update(key(k))
	}
	return f
}

func TestEditing(t *testing.T) {
	cases := []struct {
		name string
		keys []string
		val  string
		pos  int
	}{
		{"type", []string{"a", "b", "c"}, "abc", 3},
		{"batched runes", []string{"abc", "de"}, "abcde", 5},
		{"korean", []string{"한", "글"}, "한글", 2},
		{"space", []string{"a", "space", "b"}, "a b", 3},
		{"backspace removes one syllable", []string{"한글", "backspace"}, "한", 1},
		{"backspace at start", []string{"ab", "home", "backspace"}, "ab", 0},
		{"insert in the middle", []string{"ac", "left", "b"}, "abc", 2},
		{"delete under cursor", []string{"abc", "home", "delete"}, "bc", 0},
		{"ctrl+u deletes to start", []string{"hello world", "left", "left", "ctrl+u"}, "ld", 0},
		{"ctrl+k deletes to end", []string{"hello world", "home", "right", "ctrl+k"}, "h", 1},
		{"ctrl+w deletes a word", []string{"foo bar  ", "ctrl+w"}, "foo ", 4},
		{"alt+backspace deletes a word", []string{"foo bar", "alt+backspace"}, "foo ", 4},
		{"home end", []string{"abc", "home", "x", "end", "y"}, "xabcy", 5},
		{"ctrl+a ctrl+e", []string{"abc", "ctrl+a", "x", "ctrl+e", "y"}, "xabcy", 5},
		{"word moves", []string{"one two three", "alt+left", "alt+left", "X", "alt+right", "Y"}, "one XtwoY three", 9},
		{"word move by alt+b/f", []string{"one two", "alt+b", "_", "alt+f"}, "one _two", 8},
		{"left right clamp", []string{"ab", "right", "right", "home", "left", "left"}, "ab", 0},
	}
	for _, c := range cases {
		f := typeKeys(Field{}, c.keys...)
		if f.Value() != c.val || f.Cursor() != c.pos {
			t.Errorf("%s: %q cursor %d, want %q cursor %d", c.name, f.Value(), f.Cursor(), c.val, c.pos)
		}
	}
}

func TestPasteIsOneLine(t *testing.T) {
	paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\r\nb\tc\x1bd"), Paste: true}
	f, ok := Field{}.Update(paste)
	if !ok || f.Value() != "a b cd" {
		t.Fatalf("paste = %q", f.Value())
	}
}

func TestUnhandledKeysFallThrough(t *testing.T) {
	f := New("abc")
	for _, k := range []string{"enter", "esc", "up"} {
		if g, ok := f.Update(key(k)); ok || g.Value() != "abc" {
			t.Errorf("%s was used by the field", k)
		}
	}
	if _, ok := f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}); ok {
		t.Error("alt+x is a command, not text")
	}
}

func TestLimit(t *testing.T) {
	f := Field{Limit: 3}
	f = typeKeys(f, "ab", "cde", "f")
	if f.Value() != "abc" || f.Cursor() != 3 {
		t.Fatalf("limit = %q", f.Value())
	}
	f = typeKeys(f, "home", "backspace", "delete", "Z")
	if f.Value() != "Zbc" {
		t.Fatalf("after edit = %q", f.Value())
	}
}

func TestCopiesAreIndependent(t *testing.T) {
	a := New("abc")
	a = a.SetCursor(1)
	b := typeKeys(a, "X")
	c := typeKeys(a, "backspace")
	d := typeKeys(a, "ctrl+k")
	if a.Value() != "abc" || a.Cursor() != 1 {
		t.Fatalf("original changed: %q %d", a.Value(), a.Cursor())
	}
	if b.Value() != "aXbc" || c.Value() != "bc" || d.Value() != "a" {
		t.Fatalf("copies = %q %q %q", b.Value(), c.Value(), d.Value())
	}
	// Two copies typing at the same spot never share a backing array.
	e, g := typeKeys(b, "1"), typeKeys(b, "2")
	if e.Value() != "aX1bc" || g.Value() != "aX2bc" {
		t.Fatalf("siblings = %q %q", e.Value(), g.Value())
	}
}

func TestViewWidthAndCursor(t *testing.T) {
	st := Style{Cursor: lipgloss.NewStyle()} // plain, so the output is the text
	for _, c := range []struct {
		name    string
		f       Field
		w       int
		focused bool
		want    string
	}{
		{"fits, cursor at end", New("abc"), 6, true, "abc   "},
		{"blurred", New("abc"), 5, false, "abc  "},
		{"scrolls to keep the end cursor", New("abcdefgh"), 5, true, "efgh "},
		{"blurred shows the start", New("abcdefgh"), 5, false, "abcde"},
		{"cursor in the middle", New("abcdefgh").SetCursor(2), 5, true, "abcde"},
		{"wide chars never split", New("한글입니다"), 5, true, "니다 "},
		{"wide at the right edge", New("a한글").SetCursor(0), 4, true, "a한 "},
		{"placeholder", Field{Placeholder: "filter…"}, 9, false, "filter…  "},
		{"placeholder focused", Field{Placeholder: "filter"}, 8, true, "filter  "},
		{"empty focused", Field{}, 3, true, "   "},
	} {
		got := c.f.View(c.w, c.focused, st)
		if ansi.StringWidth(got) != c.w {
			t.Errorf("%s: %q is %d cells, want %d", c.name, got, ansi.StringWidth(got), c.w)
		}
		if ansi.Strip(got) != c.want {
			t.Errorf("%s: %q, want %q", c.name, ansi.Strip(got), c.want)
		}
	}
	if New("x").View(0, true, st) != "" {
		t.Fatal("zero width is empty")
	}
}

// The cursor is visible after any edit, at any width, in any script.
func TestViewAlwaysShowsTheCursorCell(t *testing.T) {
	text := "ab한글cd 😀 ef"
	for w := 1; w <= 16; w++ {
		f := New(text)
		for pos := 0; pos <= f.Len(); pos++ {
			got := f.SetCursor(pos).View(w, true, Style{})
			if ansi.StringWidth(got) != w {
				t.Fatalf("w=%d pos=%d: %q is %d cells", w, pos, got, ansi.StringWidth(got))
			}
			if w >= 2 && !strings.Contains(got, "\x1b[7m") {
				t.Fatalf("w=%d pos=%d: no cursor in %q", w, pos, got)
			}
		}
	}
}
