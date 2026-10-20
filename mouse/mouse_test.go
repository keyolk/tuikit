package mouse

import (
	"time"

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

func TestRowHitUsesOffset(t *testing.T) {
	r := Rect{X: 0, Y: 4, W: 30, H: 6}
	hit := RowHit(r, 10, 20, msg(1, 6, tea.MouseButtonLeft, tea.MouseActionPress))
	if !hit.HasItem || hit.Index != 12 {
		t.Fatalf("got %+v, want index 12", hit)
	}
	if RowHit(r, 18, 20, msg(1, 9, tea.MouseButtonLeft, tea.MouseActionPress)).HasItem {
		t.Fatal("a row past the last item must not hit")
	}
}

func TestBlockHitLandsOnTheCardNotTheLine(t *testing.T) {
	// Cards of 3, 2 and 4 lines, scrolled so card 1 is the first visible.
	r := Rect{X: 0, Y: 1, W: 20, H: 10}
	heights := []int{3, 2, 4}
	cases := []struct{ y, want int }{
		{1, 1}, {2, 1}, // both lines of card 1
		{3, 2}, {6, 2}, // first and last line of card 2
	}
	for _, c := range cases {
		hit := BlockHit(r, 1, heights, msg(0, c.y, tea.MouseButtonLeft, tea.MouseActionPress))
		if !hit.HasItem || hit.Index != c.want {
			t.Errorf("y=%d got %+v want card %d", c.y, hit, c.want)
		}
	}
	if BlockHit(r, 1, heights, msg(0, 7, tea.MouseButtonLeft, tea.MouseActionPress)).HasItem {
		t.Fatal("a line below the last card must not hit")
	}
}

func TestSplitColumnsCoversTheRectWithGaps(t *testing.T) {
	cols := SplitColumns(Rect{X: 0, Y: 2, W: 83, H: 5}, 4, 1)
	if len(cols) != 4 {
		t.Fatalf("got %d columns", len(cols))
	}
	if cols[0].W != 20 || cols[1].X != 21 {
		t.Errorf("unexpected layout %+v", cols)
	}
	last := cols[3]
	if last.X+last.W != 83 {
		t.Errorf("last column ends at %d, want 83", last.X+last.W)
	}
	if Column(cols, 20, 3) != -1 {
		t.Error("the gap between columns belongs to no column")
	}
	if Column(cols, 21, 3) != 1 {
		t.Error("x=21 is column 1")
	}
}

func TestClicksNeedTheSameTargetInTime(t *testing.T) {
	t0 := time.Unix(0, 0)
	var c Clicks
	c, d := c.Press("a", t0)
	if d {
		t.Fatal("a first click is not a double click")
	}
	c, d = c.Press("a", t0.Add(200*time.Millisecond))
	if !d {
		t.Fatal("a second click on the same target in time is a double click")
	}
	c, d = c.Press("a", t0.Add(300*time.Millisecond))
	if d {
		t.Fatal("a third click must not reuse the double click")
	}
	c, d = c.Press("b", t0.Add(400*time.Millisecond))
	if d {
		t.Fatal("a different target is not a double click")
	}
	_, d = c.Press("b", t0.Add(time.Second))
	if d {
		t.Fatal("a slow second click is not a double click")
	}
}
