package mouse

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/keyolk/tuikit/list"
)

type item struct{ id string }

func (i item) ID() string { return i.id }

func items(n int) []item {
	out := make([]item, n)
	for i := range out {
		out[i] = item{id: string(rune('a' + i))}
	}
	return out
}

func msg(x, y int, b tea.MouseButton, a tea.MouseAction) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: b, Action: a}
}

func TestRectContains(t *testing.T) {
	r := Rect{X: 5, Y: 2, W: 10, H: 5}
	cases := []struct {
		x, y int
		want bool
	}{
		{5, 2, true},   // top-left
		{14, 6, true},  // bottom-right
		{4, 2, false},  // left edge
		{15, 2, false}, // right edge
		{5, 7, false},  // below
		{5, 1, false},  // above
	}
	for _, c := range cases {
		if r.Contains(c.x, c.y) != c.want {
			t.Errorf("Contains(%d,%d)=%v want %v", c.x, c.y, !c.want, c.want)
		}
	}
}

func TestEmptyRectRejectsAll(t *testing.T) {
	r := Rect{}
	if r.Contains(0, 0) {
		t.Fatal("zero rect should not contain anything")
	}
}

func TestDelta(t *testing.T) {
	cases := []struct {
		b    tea.MouseButton
		want int
	}{
		{tea.MouseButtonWheelUp, -WheelRows},
		{tea.MouseButtonWheelDown, WheelRows},
		{tea.MouseButtonLeft, 0},
		{tea.MouseButtonNone, 0},
	}
	for _, c := range cases {
		m := msg(0, 0, c.b, tea.MouseActionPress)
		if got := Delta(m); got != c.want {
			t.Errorf("Delta(%v)=%d want %d", c.b, got, c.want)
		}
	}
}

func TestIsPress(t *testing.T) {
	if !IsPress(msg(0, 0, tea.MouseButtonLeft, tea.MouseActionPress)) {
		t.Error("left press should be a press")
	}
	if IsPress(msg(0, 0, tea.MouseButtonLeft, tea.MouseActionRelease)) {
		t.Error("release should not be a press")
	}
	if IsPress(msg(0, 0, tea.MouseButtonWheelUp, tea.MouseActionPress)) {
		t.Error("wheel should not be a press")
	}
}

func TestButtonFilters(t *testing.T) {
	press := msg(0, 0, tea.MouseButtonLeft, tea.MouseActionPress)
	if !IsLeftPress(press) || IsRightPress(press) || IsMiddlePress(press) {
		t.Error("left press misclassified")
	}
	right := msg(0, 0, tea.MouseButtonRight, tea.MouseActionPress)
	if !IsRightPress(right) || IsLeftPress(right) {
		t.Error("right press misclassified")
	}
}

func TestListHitScrolls(t *testing.T) {
	l := list.New(items(50)).SetHeight(10)
	r := Rect{X: 0, Y: 0, W: 40, H: 10}
	l = l.Move(20) // cursor at row 20, window starts at ~15
	hit := ListHit(l, r, msg(2, 0, tea.MouseButtonLeft, tea.MouseActionPress))
	if !hit.Visible || !hit.HasItem {
		t.Fatalf("expected a hit, got %+v", hit)
	}
	start, _ := l.Window()
	if hit.Index != start {
		t.Errorf("top row hit index=%d want %d", hit.Index, start)
	}
}

func TestListHitMissesEmptyRows(t *testing.T) {
	l := list.New(items(3)).SetHeight(10)
	r := Rect{X: 0, Y: 0, W: 40, H: 10}
	hit := ListHit(l, r, msg(2, 5, tea.MouseButtonLeft, tea.MouseActionPress))
	if !hit.Visible {
		t.Fatal("click inside rect should be visible")
	}
	if hit.HasItem {
		t.Fatal("click on empty row below short list should not have an item")
	}
}

func TestListHitOutsideRect(t *testing.T) {
	l := list.New(items(10)).SetHeight(5)
	r := Rect{X: 10, Y: 3, W: 20, H: 5}
	hit := ListHit(l, r, msg(2, 4, tea.MouseButtonLeft, tea.MouseActionPress))
	if hit.Visible {
		t.Fatal("click outside rect should not be visible")
	}
}

func TestSelectByClick(t *testing.T) {
	l := list.New(items(10)).SetHeight(5)
	r := Rect{X: 0, Y: 0, W: 40, H: 5}
	l2, ok := SelectByClick(l, r, msg(2, 2, tea.MouseButtonLeft, tea.MouseActionPress))
	if !ok {
		t.Fatal("expected a selection")
	}
	if l2.Index() != 2 {
		t.Errorf("index=%d want 2", l2.Index())
	}
}

func TestSelectByClickMiss(t *testing.T) {
	l := list.New(items(3)).SetHeight(10)
	r := Rect{X: 0, Y: 0, W: 40, H: 10}
	l2, ok := SelectByClick(l, r, msg(2, 8, tea.MouseButtonLeft, tea.MouseActionPress))
	if ok {
		t.Fatal("should not select an empty row")
	}
	if l2.Index() != l.Index() {
		t.Error("list should be unchanged on miss")
	}
}

func TestMoveByWheel(t *testing.T) {
	l := list.New(items(20)).SetHeight(5)
	l2, ok := MoveByWheel(l, msg(0, 0, tea.MouseButtonWheelDown, tea.MouseActionPress))
	if !ok {
		t.Fatal("wheel down should move")
	}
	if l2.Index() <= l.Index() {
		t.Error("wheel down should move cursor down")
	}
	l3, ok := MoveByWheel(l, msg(0, 0, tea.MouseButtonLeft, tea.MouseActionPress))
	if ok {
		t.Fatal("non-wheel should not move")
	}
	if l3.Index() != l.Index() {
		t.Error("list should be unchanged for non-wheel")
	}
}

func TestProgramOptionsNonEmpty(t *testing.T) {
	opts := ProgramOptions()
	if len(opts) == 0 {
		t.Fatal("ProgramOptions should return at least one option")
	}
}
