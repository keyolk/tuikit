package split

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/mouse"
)

// Friction the fleet reported while adopting split (tuikit #13).

func TestPressedClearsSelection(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("abcdefghij", 5)
	}
	a := &app{split: Model{Divider: 1}, lines: lines}
	// Select text in pane A.
	a.mouse(press(5, 1), t0)
	a.mouse(motion(12, 1), t0.Add(10*time.Millisecond))
	a.mouse(release(12, 1), t0.Add(20*time.Millisecond))
	if !a.sel.Active() {
		t.Fatal("setup: no selection")
	}
	// A divider press: the README's one-liner clears the selection.
	r, _ := a.mouse(press(50, 2), t0.Add(time.Second))
	if !r.Consumed || !r.Pressed {
		t.Fatalf("divider press: %+v, want Consumed and Pressed", r)
	}
	if r.Pressed {
		a.sel.Clear()
	}
	if a.sel.Active() {
		t.Fatal("selection still drawn after a divider press")
	}
	// Motion and release of the drag are not presses.
	r, _ = a.mouse(motion(40, 2), t0.Add(time.Second+10*time.Millisecond))
	if r.Pressed {
		t.Fatalf("drag motion reported Pressed: %+v", r)
	}
	r, _ = a.mouse(release(40, 2), t0.Add(time.Second+20*time.Millisecond))
	if r.Pressed {
		t.Fatalf("release reported Pressed: %+v", r)
	}
	// A pane press is never the split's.
	if r, _ := a.mouse(press(5, 1), t0.Add(3*time.Second)); r.Pressed || r.Consumed {
		t.Fatalf("pane press: %+v", r)
	}
}

func TestDoubleClickReportsResetAndPressed(t *testing.T) {
	m := Model{Divider: 1}.Set(0.7, area)
	m, r := m.UpdateAt(press(70, 0), area, t0)
	if r.Reset {
		t.Fatalf("a single press reported Reset: %+v", r)
	}
	m, _ = m.UpdateAt(release(70, 0), area, t0.Add(50*time.Millisecond))
	m, r = m.UpdateAt(press(70, 0), area, t0.Add(100*time.Millisecond))
	if !r.Reset || !r.Pressed || !r.Changed || m.IsSet() {
		t.Fatalf("double click: %+v set=%v", r, m.IsSet())
	}
	if _, r := m.Handle(ActionReset, area); !r.Reset {
		t.Fatalf("ActionReset: %+v, want Reset", r)
	}
	if _, r := m.Handle(ActionGrow, area); r.Reset {
		t.Fatalf("ActionGrow reported Reset")
	}
}

func TestIsSetLifecycle(t *testing.T) {
	m := Model{Divider: 1}
	if m.IsSet() || m.State() != (State{}) {
		t.Fatalf("fresh split: set=%v state=%+v", m.IsSet(), m.State())
	}
	b, _ := json.Marshal(m.State())
	if string(b) != "{}" {
		t.Fatalf("unset State encodes as %s, want {}", b)
	}
	m, r := m.Handle(ActionGrow, area)
	if !m.IsSet() || !r.Changed || m.State().Ratio == 0 {
		t.Fatalf("after grow: set=%v %+v state %+v", m.IsSet(), r, m.State())
	}
	// Reset unsets, and says Changed even where the ratio it shows does
	// not move: the saved ratio must be dropped.
	m = m.Set(0.5, area)
	m, r = m.Handle(ActionReset, area)
	if m.IsSet() || !r.Changed || m.State() != (State{}) {
		t.Fatalf("after reset: set=%v %+v", m.IsSet(), r)
	}
	// Reset of an unset split changes nothing.
	if _, r := m.Handle(ActionReset, area); r.Changed {
		t.Fatalf("reset of an unset split: %+v", r)
	}
	// Restore of an unset State keeps it unset; a real ratio sets it.
	if got, ok := m.Restore(State{}); ok || got.IsSet() {
		t.Fatalf("Restore({}) = set %v ok %v", got.IsSet(), ok)
	}
	if got, ok := m.Restore(State{Ratio: 0.3}); !ok || !got.IsSet() {
		t.Fatalf("Restore(0.3) = set %v ok %v", got.IsSet(), ok)
	}
}

func TestUnmovedGesturesKeepSplitUnset(t *testing.T) {
	// A press and a motion inside the same cell is not a choice.
	m := Model{Divider: 1}
	m, rs := feed(t, m, t0, press(50, 0), motion(50, 1), release(50, 1))
	if m.IsSet() || rs[1].Changed {
		t.Fatalf("a drag that never moved set the split: %+v", rs[1])
	}
	// A key at the limit is not a choice either.
	lim := Model{Divider: 1, Default: 0.9}
	lim, r := lim.Handle(ActionGrow, area)
	if lim.IsSet() || r.Changed {
		t.Fatalf("grow at Hi set the split: %+v", r)
	}
	// A drag against the limit does not set it.
	lim, rs = feed(t, lim, t0, press(90, 0), motion(99, 0))
	if lim.IsSet() || rs[1].Changed {
		t.Fatalf("drag past Hi set the split: %+v", rs[1])
	}
}

func TestDefaultFor(t *testing.T) {
	third := func(r mouse.Rect) float64 { return 1.0 / 3 }
	m := Model{Divider: 1, DefaultFor: DefaultFunc(third)}
	if got := widthA(m); got != 33 {
		t.Fatalf("unset with DefaultFor: A = %d, want 33", got)
	}
	if m.RatioIn(area) != 1.0/3 || m.Ratio() != DefaultRatio {
		t.Fatalf("RatioIn %v Ratio %v", m.RatioIn(area), m.Ratio())
	}
	// A key steps from the 33 cells on screen.
	g, r := m.Handle(ActionGrow, area)
	if !r.Changed || widthA(g) != 38 {
		t.Fatalf("grow from DefaultFor: %+v A = %d, want 38", r, widthA(g))
	}
	// Set wins over DefaultFor; Reset goes back to it.
	if widthA(m.Set(0.6, area)) != 60 || widthA(g.Reset(area)) != 33 {
		t.Fatalf("set/reset with DefaultFor: %d %d", widthA(m.Set(0.6, area)), widthA(g.Reset(area)))
	}
	// A bad answer falls back to Default; bounds still apply.
	for _, v := range []float64{math.NaN(), 0, 1, -2, 40} {
		bad := Model{Divider: 1, Default: 0.4, DefaultFor: DefaultFunc(func(mouse.Rect) float64 { return v })}
		if widthA(bad) != 40 {
			t.Errorf("DefaultFor %v: A = %d, want Default 40", v, widthA(bad))
		}
	}
	low := Model{Divider: 1, DefaultFor: DefaultFunc(func(mouse.Rect) float64 { return 0.01 })}
	if widthA(low) != 10 {
		t.Fatalf("DefaultFor under Lo: A = %d, want 10", widthA(low))
	}
	// The area it sees is the one laid out.
	var seen mouse.Rect
	spy := Model{Divider: 1, DefaultFor: DefaultFunc(func(r mouse.Rect) float64 { seen = r; return 0.5 })}
	spy.Layout(area)
	if seen != area {
		t.Fatalf("DefaultFor saw %+v, want %+v", seen, area)
	}
}

func TestMaxCaps(t *testing.T) {
	m := Model{Divider: 1, MaxA: 30}
	if widthA(m) != 30 {
		t.Fatalf("MaxA 30 at 50%%: A = %d", widthA(m))
	}
	if g, r := m.Handle(ActionGrow, area); widthA(g) != 30 || r.Changed {
		t.Fatalf("grow past MaxA: A = %d %+v", widthA(g), r)
	}
	if widthA(m.Set(0.8, area)) != 30 {
		t.Fatalf("Set past MaxA: A = %d", widthA(m.Set(0.8, area)))
	}
	d, _ := feed(t, m, t0, press(30, 0), motion(80, 0))
	if widthA(d) != 30 {
		t.Fatalf("drag past MaxA: A = %d", widthA(d))
	}
	if s, _ := m.Handle(ActionShrink, area); widthA(s) != 25 {
		t.Fatalf("shrink under MaxA: A = %d, want 25", widthA(s))
	}

	b := Model{Divider: 1, MaxB: 20}
	if widthA(b) != 80 {
		t.Fatalf("MaxB 20: A = %d, want 80", widthA(b))
	}
	if widthA(b.Set(0.2, area)) != 80 {
		t.Fatalf("Set under MaxB: A = %d", widthA(b.Set(0.2, area)))
	}
	// Both caps that cannot hold together: MaxA holds, B takes the rest.
	both := Model{Divider: 1, MaxA: 30, MaxB: 30}
	if a, _, pb := both.Layout(area); a.W != 30 || pb.W != 70 {
		t.Fatalf("conflicting caps: A %d B %d, want 30 70", a.W, pb.W)
	}
	// The minimum wins over a cap.
	if widthA(Model{Divider: 1, MinA: 40, MaxA: 30}) != 40 {
		t.Fatalf("MinA over MaxA: A = %d", widthA(Model{Divider: 1, MinA: 40, MaxA: 30}))
	}
	if widthA(Model{Divider: 1, MinB: 90, MaxB: 5}) != 10 {
		t.Fatalf("MinB over MaxB: A = %d", widthA(Model{Divider: 1, MinB: 90, MaxB: 5}))
	}
	// A cap never collapses the split.
	if (Model{Divider: 1, MaxA: 1, MaxB: 1}).Collapsed(area) {
		t.Fatal("caps collapsed the split")
	}
}

func TestLookBlank(t *testing.T) {
	div := rc(10, 0, 1, 3)
	m := Model{Divider: 1, Look: LookBlank}
	if got := m.DrawDivider(div, noStyle); got != " \n \n " {
		t.Fatalf("idle blank divider = %q", got)
	}
	m, _ = m.UpdateAt(press(50, 0), area, t0)
	if got := m.DrawDivider(div, noStyle); !strings.Contains(got, lineVDrag) {
		t.Fatalf("dragging blank divider = %q, want the heavy line", got)
	}
	if got := (Model{Divider: 1}).DrawDivider(div, noStyle); !strings.Contains(got, lineV) {
		t.Fatalf("default look = %q, want the line", got)
	}
	if got := (Model{Look: LookBlank}).DrawDivider(R{}, noStyle); got != "" {
		t.Fatalf("empty blank divider = %q", got)
	}
}

func TestCancelDrag(t *testing.T) {
	m, _ := feed(t, Model{Divider: 1}, t0, press(50, 0), motion(60, 0))
	m = m.CancelDrag()
	if m.Dragging() || widthA(m) != 60 {
		t.Fatalf("CancelDrag: dragging %v A %d, want false 60", m.Dragging(), widthA(m))
	}
	if _, r := m.UpdateAt(motion(80, 0), area, t0); r.Consumed {
		t.Fatal("motion after CancelDrag consumed")
	}
}

func TestBindingsOrderAndWith(t *testing.T) {
	// Bindings keeps its order (ghx and narwhal assert it); BindingsWith
	// lists shrink first, the order a footer reads.
	for name, c := range map[string]struct {
		bs   []keys.Binding
		want []string
	}{
		"Bindings":     {Bindings("list"), []string{ActionGrow, ActionShrink, ActionReset}},
		"BindingsWith": {BindingsWith("list", "Layout", 7), []string{ActionShrink, ActionGrow, ActionReset}},
	} {
		for i, b := range c.bs {
			if b.Action != c.want[i] {
				t.Errorf("%s[%d] = %s, want %s", name, i, b.Action, c.want[i])
			}
		}
	}
	for _, b := range BindingsWith("list", "Layout", 7) {
		if b.Group != "Layout" || b.Footer != 7 || b.FooterGroup != FooterGroup || b.Ctx != "list" {
			t.Errorf("BindingsWith %s: %+v", b.Action, b)
		}
	}
	for _, b := range BindingsWith("list", "Layout", 0) {
		if b.Footer != 0 || b.FooterGroup != "" {
			t.Errorf("BindingsWith footer 0 %s: %+v", b.Action, b)
		}
	}
	km := keys.New(BindingsWith("list", "Layout", 1)...)
	if got := km.Footer([]keys.Ctx{"list"}, 80, keys.Opts{}, keys.FooterStyle{}); !strings.Contains(got, "<>= resize") {
		t.Fatalf("footer = %q, want `<>= resize`", got)
	}
}

func TestAsymmetricGrab(t *testing.T) {
	// gcl's bottom split: a zero-height divider, dragged by A's last row.
	ar := rc(0, 0, 40, 20)
	m := Model{Orientation: Vertical, GrabA: 1}
	a, _, b := m.Layout(ar)
	lastA, firstB := a.Y+a.H-1, b.Y
	if _, r := m.UpdateAt(press(5, firstB), ar, t0); r.Consumed {
		t.Fatalf("press on B's first row (y=%d) consumed", firstB)
	}
	m, r := m.UpdateAt(press(5, lastA), ar, t0)
	if !r.Consumed || !r.Pressed {
		t.Fatalf("press on A's last row (y=%d): %+v", lastA, r)
	}
	m, r = m.UpdateAt(motion(5, lastA+4), ar, t0)
	if a, _, _ := m.Layout(ar); !r.Changed || a.H != 14 {
		t.Fatalf("drag by A's last row: %+v A.H = %d, want 14", r, a.H)
	}
	// GrabB alone, horizontal: B's first column only.
	g := Model{Divider: 1, GrabB: 1}
	for x, want := range map[int]bool{49: false, 50: true, 51: true, 52: false} {
		if _, r := g.UpdateAt(press(x, 0), area, t0); r.Consumed != want {
			t.Errorf("GrabB 1: x=%d consumed %v, want %v", x, r.Consumed, want)
		}
	}
	// GrabA/GrabB override Grab.
	o := Model{Divider: 1, Grab: 3, GrabA: 1}
	if _, r := o.UpdateAt(press(51, 0), area, t0); r.Consumed {
		t.Fatal("Grab applied although GrabA was set")
	}
}

func TestZeroModelIsUsable(t *testing.T) {
	var m Model
	if !m.IsZero() {
		t.Fatal("zero Model not IsZero")
	}
	a, div, b := m.Layout(rc(0, 0, 10, 2))
	if a.W != 5 || div.W != 0 || b.W != 5 || b.X != 5 {
		t.Fatalf("zero Model layout: %+v %+v %+v", a, div, b)
	}
	if got := m.Join(rc(0, 0, 10, 1), "aaaaa", m.DrawDivider(div, noStyle), "bbbbb"); got != "aaaaabbbbb" {
		t.Fatalf("zero Model join = %q", got)
	}
	if _, r := m.UpdateAt(press(5, 0), area, t0); r.Consumed {
		t.Fatal("zero Model consumed a press: it has no divider cell")
	}
	g, r := m.Handle(ActionGrow, area)
	if !r.Changed || g.IsZero() {
		t.Fatalf("zero Model grow: %+v zero=%v", r, g.IsZero())
	}
	for _, nz := range []Model{{Divider: 1}, {Default: 0.4}, {DefaultFor: DefaultFunc(func(mouse.Rect) float64 { return 0.5 })}, {GrabA: 1}, {MaxB: 3}, {Look: LookBlank}} {
		if nz.IsZero() {
			t.Errorf("%+v reported IsZero", nz)
		}
	}
}

// thirdCapped is a comparable Defaulter: a struct keeps Model comparable.
type thirdCapped struct{ max int }

func (d thirdCapped) DefaultRatio(r mouse.Rect) float64 {
	return float64(min(r.W/3, d.max)) / float64(r.W-1)
}

func TestModelStaysComparable(t *testing.T) {
	// gcl and kkt compare a split to split.Model{}; that must compile and
	// not panic, with or without a Defaulter.
	var zero Model
	withFunc := Model{DefaultFor: DefaultFunc(func(mouse.Rect) float64 { return 0.5 })}
	if withFunc == zero || !(zero == Model{}) {
		t.Fatal("== against the zero Model")
	}
	a, b := Model{DefaultFor: thirdCapped{20}}, Model{DefaultFor: thirdCapped{20}}
	if a != b {
		t.Fatal("two Models with equal struct Defaulters differ")
	}
	if got := widthA(a); got != 20 {
		t.Fatalf("struct Defaulter: A = %d, want 20", got)
	}
}
