package split

import (
	"math"
	"testing"

	"github.com/keyolk/tuikit/mouse"
)

type R = mouse.Rect

func TestLayout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		m         Model
		r         R
		a, div, b R
	}{
		{"80x24 half, divider 1", Model{Divider: 1}, rc(0, 0, 80, 24),
			rc(0, 0, 40, 24), rc(40, 0, 1, 24), rc(41, 0, 39, 24)},
		{"offset rect keeps origin", Model{Divider: 1}, rc(2, 3, 80, 20),
			rc(2, 3, 40, 20), rc(42, 3, 1, 20), rc(43, 3, 39, 20)},
		{"divider 0 is a seam", Model{}, rc(0, 0, 80, 24),
			rc(0, 0, 40, 24), rc(40, 0, 0, 24), rc(40, 0, 40, 24)},
		{"divider 2", Model{Divider: 2}, rc(0, 0, 80, 24),
			rc(0, 0, 39, 24), rc(39, 0, 2, 24), rc(41, 0, 39, 24)},
		{"divider 3", Model{Divider: 3}, rc(0, 0, 80, 24),
			rc(0, 0, 39, 24), rc(39, 0, 3, 24), rc(42, 0, 38, 24)}, // 38.5 rounds up
		{"60 cols at 0.3", Model{Divider: 1, ratio: 0.3}, rc(0, 0, 60, 10),
			rc(0, 0, 18, 10), rc(18, 0, 1, 10), rc(19, 0, 41, 10)},
		{"min A wins over ratio", Model{Divider: 1, MinA: 30, ratio: 0.2}, rc(0, 0, 60, 10),
			rc(0, 0, 30, 10), rc(30, 0, 1, 10), rc(31, 0, 29, 10)},
		{"min B wins over ratio", Model{Divider: 1, MinB: 20, ratio: 0.9}, rc(0, 0, 60, 10),
			rc(0, 0, 39, 10), rc(39, 0, 1, 10), rc(40, 0, 20, 10)},
		{"mins do not fit: primary A only", Model{Divider: 1, MinA: 30, MinB: 20}, rc(0, 0, 50, 10),
			rc(0, 0, 50, 10), R{}, R{}},
		{"mins do not fit: primary B only", Model{Divider: 1, MinA: 30, MinB: 20, Primary: B}, rc(0, 0, 50, 10),
			R{}, R{}, rc(0, 0, 50, 10)},
		{"exactly fits", Model{Divider: 1, MinA: 30, MinB: 20}, rc(0, 0, 51, 10),
			rc(0, 0, 30, 10), rc(30, 0, 1, 10), rc(31, 0, 20, 10)},
		{"under Collapse", Model{Divider: 1, Collapse: 70, Primary: B}, rc(0, 0, 69, 10),
			R{}, R{}, rc(0, 0, 69, 10)},
		{"at Collapse", Model{Divider: 1, Collapse: 70}, rc(0, 0, 70, 10),
			rc(0, 0, 35, 10), rc(35, 0, 1, 10), rc(36, 0, 34, 10)},
		{"tiny: 2 cols, divider 1", Model{Divider: 1}, rc(0, 0, 2, 1),
			rc(0, 0, 2, 1), R{}, R{}},
		{"tiny: 3 cols, divider 1", Model{Divider: 1}, rc(0, 0, 3, 1),
			rc(0, 0, 1, 1), rc(1, 0, 1, 1), rc(2, 0, 1, 1)},
		{"tiny: 1 col, divider 3", Model{Divider: 3}, rc(0, 0, 1, 1),
			rc(0, 0, 1, 1), R{}, R{}},
		{"zero rect", Model{Divider: 1}, R{}, R{}, R{}, R{}},
		{"vertical 80x24", Model{Orientation: Vertical, Divider: 1}, rc(0, 0, 80, 24),
			rc(0, 0, 80, 12), rc(0, 12, 80, 1), rc(0, 13, 80, 11)},
		{"vertical divider 3, min B", Model{Orientation: Vertical, Divider: 3, MinB: 15}, rc(0, 1, 80, 24),
			rc(0, 1, 80, 6), rc(0, 7, 80, 3), rc(0, 10, 80, 15)},
		{"vertical collapse by rows", Model{Orientation: Vertical, Divider: 1, Collapse: 20}, rc(0, 0, 200, 19),
			rc(0, 0, 200, 19), R{}, R{}},
		{"ratio outside bounds is clamped", Model{Divider: 1, ratio: 0.99}, rc(0, 0, 101, 5),
			rc(0, 0, 90, 5), rc(90, 0, 1, 5), rc(91, 0, 10, 5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, div, b := tc.m.Layout(tc.r)
			if a != tc.a || div != tc.div || b != tc.b {
				t.Fatalf("Layout(%v) = %v %v %v, want %v %v %v", tc.r, a, div, b, tc.a, tc.div, tc.b)
			}
			// The three rects tile r along the axis, with no overlap: the
			// divider's cells belong to no pane.
			if !tc.m.Collapsed(tc.r) && tc.r.W > 0 {
				for x := tc.r.X; x < tc.r.X+tc.r.W; x++ {
					for y := tc.r.Y; y < tc.r.Y+tc.r.H; y++ {
						n := 0
						for _, rr := range []R{a, div, b} {
							if rr.Contains(x, y) {
								n++
							}
						}
						if n != 1 {
							t.Fatalf("cell (%d,%d) is in %d rects", x, y, n)
						}
					}
				}
			}
		})
	}
}

func TestRatioSurvivesResize(t *testing.T) {
	m := Model{Divider: 1}.Set(0.3, rc(0, 0, 101, 10))
	a, _, _ := m.Layout(rc(0, 0, 201, 10))
	if a.W != 60 {
		t.Fatalf("A at 201 cols = %d, want 60 (30%% of 200)", a.W)
	}
}

func TestWritesClamp(t *testing.T) {
	area := rc(0, 0, 61, 10) // avail 60
	m := Model{Divider: 1, MinA: 30, MinB: 20}
	for _, v := range []float64{-1, 0, 0.01, 0.2} {
		if got := m.Set(v, area).Ratio(); got != 0.5 {
			t.Errorf("Set(%v) ratio = %v, want 0.5 (MinA 30 of 60)", v, got)
		}
	}
	if got := m.Set(0.95, area).Ratio(); math.Abs(got-40.0/60) > 1e-9 {
		t.Errorf("Set(0.95) ratio = %v, want %v (MinB 20 of 60)", got, 40.0/60)
	}
	if got := m.Set(math.NaN(), area); got.Ratio() != m.Ratio() {
		t.Errorf("Set(NaN) changed the ratio to %v", got.Ratio())
	}
	if got := (Model{Lo: 0.25, Hi: 0.75}).Set(0.9, R{}).Ratio(); got != 0.75 {
		t.Errorf("Set(0.9) with Hi 0.75 and no area = %v, want 0.75", got)
	}
}

// narwhal's unclamped delta: pressing grow past the limit stored a value
// the layout could not show, so the presses back did nothing for a while.
// Clamping on write means the first press back always moves the divider.
func TestNoDeadZone(t *testing.T) {
	area := rc(0, 0, 81, 10)
	m := Model{Divider: 1, MinB: 30}
	for range 20 {
		m = m.Grow(area)
	}
	a1, _, _ := m.Layout(area)
	m = m.Shrink(area)
	a2, _, _ := m.Layout(area)
	if a2.W >= a1.W {
		t.Fatalf("shrink after growing to the limit: A %d -> %d, want smaller", a1.W, a2.W)
	}
}

func TestStepMovesAtLeastOneCell(t *testing.T) {
	area := rc(0, 0, 11, 3) // avail 10: a 0.01 step is a tenth of a cell
	m := Model{Divider: 1, Step: 0.01}
	a0, _, _ := m.Layout(area)
	a1, _, _ := m.Grow(area).Layout(area)
	if a1.W != a0.W+1 {
		t.Fatalf("Grow on 10 cells: A %d -> %d, want +1", a0.W, a1.W)
	}
	a2, _, _ := m.Shrink(area).Layout(area)
	if a2.W != a0.W-1 {
		t.Fatalf("Shrink on 10 cells: A %d -> %d, want -1", a0.W, a2.W)
	}
}

func TestHandleReportsChange(t *testing.T) {
	area := rc(0, 0, 101, 10)
	m := Model{Divider: 1, Hi: 0.55}
	m, r := m.Handle(ActionGrow, area)
	if !r.Consumed || !r.Changed {
		t.Fatalf("grow from 0.5: %+v", r)
	}
	if _, r = m.Handle(ActionGrow, area); !r.Consumed || r.Changed {
		t.Fatalf("grow at Hi: %+v, want consumed, unchanged", r)
	}
	if _, r = m.Handle("list.open", area); r.Consumed {
		t.Fatalf("an unrelated action was consumed")
	}
	m, _ = m.Handle(ActionReset, area)
	if m.Ratio() != 0.5 {
		t.Fatalf("reset = %v", m.Ratio())
	}
}

func TestStateRestore(t *testing.T) {
	m := Model{Lo: 0.25, Hi: 0.75}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, -0.3, 1, 50, 25} {
		if got, ok := m.Restore(State{Ratio: bad}); ok || got.Ratio() != 0.5 {
			t.Errorf("Restore(%v) = %v, %v; want rejected, 0.5", bad, got.Ratio(), ok)
		}
	}
	if got, ok := m.Restore(State{Ratio: 0.9}); !ok || got.Ratio() != 0.75 {
		t.Errorf("Restore(0.9) = %v, %v; want clamped to 0.75", got.Ratio(), ok)
	}
	got, ok := m.Restore(State{Ratio: 0.3})
	if !ok || got.State() != (State{Ratio: 0.3}) {
		t.Errorf("Restore(0.3) round trip = %+v, %v", got.State(), ok)
	}
}

func rc(x, y, w, h int) R { return R{X: x, Y: y, W: w, H: h} }

// A restored ratio is clamped to the bounds only (no size is known yet), so
// at a small size it can sit past what the minimums allow. The first key
// press must still move the divider: steps start from the screen.
func TestStepAfterRestoreMovesAtOnce(t *testing.T) {
	area := rc(0, 0, 81, 10) // avail 80; MinB 30 caps A at 50
	m, _ := Model{Divider: 1, MinB: 30}.Restore(State{Ratio: 0.85})
	a0, _, _ := m.Layout(area)
	a1, _, _ := m.Shrink(area).Layout(area)
	if a1.W >= a0.W {
		t.Fatalf("first shrink after restoring 0.85: A %d -> %d", a0.W, a1.W)
	}
}
