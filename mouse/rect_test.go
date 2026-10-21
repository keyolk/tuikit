package mouse

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/render"
)

func TestInsetStyleIsWhereBoxDrawsTheBody(t *testing.T) {
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Margin(1, 0, 0, 2)
	outer := Rect{X: 3, Y: 2, W: 20, H: 7}
	in := outer.InsetStyle(st)

	// Draw the box on a blank screen and find the body's first cell by its text.
	box := render.Box("t", "BODY", outer.W, outer.H, st)
	rows := strings.Split(box, "\n")
	for y, row := range rows {
		if x := strings.Index(row, "BODY"); x >= 0 {
			// Index is a byte offset; the box's left border is one 3-byte rune.
			cells := lipgloss.Width(row[:x])
			if got := (Rect{X: outer.X + cells, Y: outer.Y + y}); got.X != in.X || got.Y != in.Y {
				t.Fatalf("body drawn at (%d,%d), InsetStyle says (%d,%d)", got.X, got.Y, in.X, in.Y)
			}
			cw, ch := render.BoxContent(outer.W, outer.H, st)
			if in.W != cw || in.H != ch {
				t.Fatalf("InsetStyle size %dx%d, BoxContent %dx%d", in.W, in.H, cw, ch)
			}
			return
		}
	}
	t.Fatalf("BODY not found in\n%s", box)
}

func TestInsetNeverGoesNegative(t *testing.T) {
	if got := (Rect{X: 1, Y: 1, W: 2, H: 2}).Inset(1, 1, 1, 1); got.W != 0 || got.H != 0 || got.Contains(2, 2) {
		t.Fatalf("got %+v", got)
	}
}

func TestSplitTopAndBottom(t *testing.T) {
	r := Rect{X: 0, Y: 1, W: 10, H: 5}
	top, body := r.SplitTop(2)
	if top != (Rect{0, 1, 10, 2}) || body != (Rect{0, 3, 10, 3}) {
		t.Fatalf("SplitTop: %+v %+v", top, body)
	}
	body, foot := r.SplitBottom(1)
	if body != (Rect{0, 1, 10, 4}) || foot != (Rect{0, 5, 10, 1}) {
		t.Fatalf("SplitBottom: %+v %+v", body, foot)
	}
	if top, rest := r.SplitTop(9); top.H != 5 || rest.H != 0 {
		t.Fatalf("SplitTop past the height: %+v %+v", top, rest)
	}
}
