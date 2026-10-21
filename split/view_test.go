package split

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/render"
)

var noStyle = lipgloss.NewStyle()

func TestDivider(t *testing.T) {
	for _, tc := range []struct {
		o    Orientation
		div  R
		drag bool
		want string
	}{
		{Horizontal, rc(0, 0, 1, 2), false, "│\n│"},
		{Horizontal, rc(0, 0, 2, 1), false, "│ "},
		{Horizontal, rc(0, 0, 3, 1), false, " │ "},
		{Horizontal, rc(0, 0, 3, 1), true, " ┃ "},
		{Vertical, rc(0, 0, 4, 1), false, "────"},
		{Vertical, rc(0, 0, 3, 2), false, "───\n   "},
		{Vertical, rc(0, 0, 3, 3), true, "   \n━━━\n   "},
		{Horizontal, R{}, false, ""},
	} {
		got := ansi.Strip(Divider(tc.o, tc.div, noStyle, tc.drag))
		if got != tc.want {
			t.Errorf("Divider(%v, %v, drag=%v) = %q, want %q", tc.o, tc.div, tc.drag, got, tc.want)
		}
		for _, l := range strings.Split(got, "\n") {
			if tc.div.W > 0 && render.Width(l) != tc.div.W {
				t.Errorf("Divider row %q is %d cells, want %d", l, render.Width(l), tc.div.W)
			}
		}
	}
}

func TestJoinWidthsExact(t *testing.T) {
	long := strings.Repeat("한글", 40) // wide characters, longer than any pane
	for _, tc := range []struct {
		name string
		m    Model
		r    R
	}{
		{"horizontal d1", Model{Divider: 1}, rc(0, 0, 80, 5)},
		{"horizontal d0 odd", Model{ratio: 0.33}, rc(0, 0, 61, 4)},
		{"horizontal d2", Model{Divider: 2, ratio: 0.37}, rc(0, 0, 47, 3)},
		{"horizontal d3", Model{Divider: 3}, rc(0, 0, 31, 3)},
		{"collapsed", Model{Divider: 1, Collapse: 90}, rc(0, 0, 60, 3)},
		{"vertical d1", Model{Orientation: Vertical, Divider: 1}, rc(0, 0, 33, 9)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, div, _ := tc.m.Layout(tc.r)
			out := tc.m.Join(tc.r, "a1 "+long+"\nshort", tc.m.DrawDivider(div, noStyle), "가나다\n"+long)
			rows := strings.Split(out, "\n")
			if len(rows) != tc.r.H {
				t.Fatalf("%d rows, want %d", len(rows), tc.r.H)
			}
			for i, row := range rows {
				if w := render.Width(row); w != tc.r.W {
					t.Errorf("row %d is %d cells, want %d: %q", i, w, tc.r.W, ansi.Strip(row))
				}
			}
		})
	}
}

func TestJoinPlacesDividerAtLayout(t *testing.T) {
	m := Model{Divider: 3, ratio: 0.25}
	r := rc(0, 0, 43, 2)
	_, div, _ := m.Layout(r)
	out := ansi.Strip(m.Join(r, "left", m.DrawDivider(div, noStyle), "right"))
	for _, row := range strings.Split(out, "\n") {
		if got := ansi.Cut(row, div.X, div.X+div.W); got != " │ " {
			t.Fatalf("divider cells %q in row %q, want \" │ \" at %d", got, row, div.X)
		}
	}
}
