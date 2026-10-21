package split

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/selection"
)

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}
func motion(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion}
}
func hover(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}
}
func release(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}
}

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// area: 101 columns, so the panes share 100 and A's width is the percent.
var area = rc(0, 0, 101, 10)

func feed(t *testing.T, m Model, at time.Time, msgs ...tea.MouseMsg) (Model, []Result) {
	t.Helper()
	var out []Result
	for i, msg := range msgs {
		var r Result
		m, r = m.UpdateAt(msg, area, at.Add(time.Duration(i)*time.Second))
		out = append(out, r)
	}
	return m, out
}

func widthA(m Model) int { a, _, _ := m.Layout(area); return a.W }

func TestDragMovesDivider(t *testing.T) {
	m := Model{Divider: 1}
	m, rs := feed(t, m, t0, press(50, 3), motion(60, 3), motion(70, 4), release(70, 4))
	if !rs[0].Consumed || rs[0].Changed {
		t.Fatalf("press on divider: %+v, want consumed, unchanged", rs[0])
	}
	for i, r := range rs[1:3] {
		if !r.Consumed || !r.Changed {
			t.Fatalf("motion %d: %+v", i, r)
		}
	}
	if !rs[3].Consumed || m.Dragging() {
		t.Fatalf("release: %+v dragging=%v", rs[3], m.Dragging())
	}
	if widthA(m) != 70 {
		t.Fatalf("A = %d after dragging to x=70, want 70", widthA(m))
	}
	// After the release, motion is no longer the split's.
	if _, r := m.UpdateAt(motion(30, 3), area, t0); r.Consumed {
		t.Fatalf("motion after release consumed")
	}
}

// Grabbing a 3-cell divider by its right edge must not jump it by 2.
func TestDragKeepsGrabOffset(t *testing.T) {
	m := Model{Divider: 3}
	_, div, _ := m.Layout(area)
	m, _ = feed(t, m, t0, press(div.X+2, 0), motion(div.X+3, 0))
	if got, want := widthA(m), div.X+1; got != want {
		t.Fatalf("A = %d after a 1-cell drag, want %d", got, want)
	}
}

func TestDragPastEdgesClamps(t *testing.T) {
	m := Model{Divider: 1, MinA: 20, MinB: 30}
	m, _ = feed(t, m, t0, press(50, 0), motion(-40, 0))
	if widthA(m) != 20 {
		t.Fatalf("drag past the left edge: A = %d, want MinA 20", widthA(m))
	}
	m, _ = feed(t, m, t0, motion(500, 0))
	if widthA(m) != 70 {
		t.Fatalf("drag past the right edge: A = %d, want 100-MinB = 70", widthA(m))
	}
}

func TestButtonlessMotionEndsDrag(t *testing.T) {
	m := Model{Divider: 1}
	m, rs := feed(t, m, t0, press(50, 0), motion(55, 0), hover(80, 0), motion(90, 0))
	if !rs[2].Consumed || m.Dragging() {
		t.Fatalf("hover after a lost release: %+v dragging=%v", rs[2], m.Dragging())
	}
	if rs[3].Consumed || widthA(m) != 55 {
		t.Fatalf("motion after the drag ended moved the divider: %+v A=%d", rs[3], widthA(m))
	}
}

func TestPanePressNotConsumed(t *testing.T) {
	m := Model{Divider: 1}
	for _, x := range []int{0, 49, 51, 100} {
		if _, r := m.UpdateAt(press(x, 2), area, t0); r.Consumed {
			t.Errorf("press at x=%d (a pane) consumed", x)
		}
	}
	// Grab takes cells next to the divider for presses, and only those.
	g := Model{Divider: 1, Grab: 1}
	for x, want := range map[int]bool{48: false, 49: true, 50: true, 51: true, 52: false} {
		if _, r := g.UpdateAt(press(x, 2), area, t0); r.Consumed != want {
			t.Errorf("Grab 1: press at x=%d consumed=%v, want %v", x, r.Consumed, want)
		}
	}
}

func TestVerticalDrag(t *testing.T) {
	m := Model{Orientation: Vertical, Divider: 1}
	ar := rc(0, 0, 40, 21)
	m, _ = m.UpdateAt(press(5, 10), ar, t0)
	m, r := m.UpdateAt(motion(5, 14), ar, t0)
	a, _, _ := m.Layout(ar)
	if !r.Changed || a.H != 14 {
		t.Fatalf("vertical drag: %+v A.H = %d, want 14", r, a.H)
	}
}

func TestDoubleClickResets(t *testing.T) {
	m := Model{Divider: 1}.Set(0.7, area)
	m, r := m.UpdateAt(press(70, 0), area, t0)
	m, _ = m.UpdateAt(release(70, 0), area, t0.Add(50*time.Millisecond))
	m, r = m.UpdateAt(press(70, 0), area, t0.Add(100*time.Millisecond))
	if !r.Consumed || !r.Changed || m.Ratio() != 0.5 || m.Dragging() {
		t.Fatalf("double click: %+v ratio %v dragging %v", r, m.Ratio(), m.Dragging())
	}
}

func TestDragThenRegrabDoesNotReset(t *testing.T) {
	m := Model{Divider: 1}
	m, _ = m.UpdateAt(press(50, 0), area, t0)
	m, _ = m.UpdateAt(motion(60, 0), area, t0.Add(10*time.Millisecond))
	m, _ = m.UpdateAt(release(60, 0), area, t0.Add(20*time.Millisecond))
	m, _ = m.UpdateAt(press(60, 0), area, t0.Add(30*time.Millisecond))
	if widthA(m) != 60 || !m.Dragging() {
		t.Fatalf("re-grab after a drag: A = %d dragging %v, want 60 and a new drag", widthA(m), m.Dragging())
	}
}

// The order the README prescribes: split first, selection only when the
// split did not consume the event.
type app struct {
	split Model
	sel   selection.Selection
	lines []string
}

func (a *app) panes() []selection.Pane {
	pa, _, pb := a.split.Layout(area)
	return []selection.Pane{
		{ID: "a", Rect: pa, Kind: selection.Text, Lines: a.lines},
		{ID: "b", Rect: pb, Kind: selection.Text, Lines: a.lines},
	}
}

func (a *app) frame() string {
	pa, pb := strings.Join(a.lines, "\n"), strings.Join(a.lines, "\n")
	return a.split.Join(area, pa, a.split.DrawDivider(R{}, noStyle), pb)
}

func (a *app) mouse(msg tea.MouseMsg, now time.Time) (splitR Result, selR selection.Result) {
	a.split, splitR = a.split.UpdateAt(msg, area, now)
	if splitR.Consumed {
		return splitR, selection.Result{}
	}
	selR, _ = a.sel.Update(msg, a.panes, a.frame, now)
	return splitR, selR
}

func TestComposesWithSelection(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("abcdefghij", 5)
	}
	a := &app{split: Model{Divider: 1}, lines: lines}

	// A drag that starts on the divider moves it and never selects text.
	a.mouse(press(50, 2), t0)
	a.mouse(motion(40, 4), t0.Add(10*time.Millisecond))
	a.mouse(release(40, 4), t0.Add(20*time.Millisecond))
	if a.sel.Active() || a.sel.Dragging() {
		t.Fatalf("a divider drag started a text selection")
	}
	if widthA(a.split) != 40 {
		t.Fatalf("divider at %d, want 40", widthA(a.split))
	}
	// Even an app that called selection for a divider press (wrong order)
	// gets no selection: the divider's cells are in no pane.
	var s selection.Selection
	s.Update(press(40, 2), a.panes, a.frame, t0)
	s.Update(motion(45, 4), a.panes, a.frame, t0)
	if s.Active() {
		t.Fatalf("a divider press selected text: the divider overlaps a pane")
	}

	// A drag inside a pane is the selection's, and the split stays put.
	sr, _ := a.mouse(press(5, 1), t0.Add(time.Second))
	if sr.Consumed {
		t.Fatalf("the split consumed a pane press")
	}
	_, selR := a.mouse(motion(12, 1), t0.Add(time.Second+10*time.Millisecond))
	if !selR.Consumed || !a.sel.Active() {
		t.Fatalf("a drag in pane A did not select: %+v", selR)
	}
	if widthA(a.split) != 40 {
		t.Fatalf("a pane drag moved the divider to %d", widthA(a.split))
	}
}

func TestBindingsHaveNoConflicts(t *testing.T) {
	stack := []keys.Ctx{"list", "global"}
	km := keys.New(append(Bindings("list"),
		keys.Binding{Action: "quit", Keys: []string{"q"}, Ctx: "global", Help: "quit", Pinned: true},
		keys.Binding{Action: "list.down", Keys: []string{"j"}, Ctx: "list", Help: "down"},
	)...)
	for _, c := range km.Conflicts(stack) {
		t.Error(c)
	}
	for _, b := range Bindings("list") {
		if b.Footer != 0 || b.Help == "" || b.Group == "" {
			t.Errorf("%s: footer %d help %q group %q", b.Action, b.Footer, b.Help, b.Group)
		}
	}

	// gcl binds < and > to nudge times: Conflicts must say so.
	gcl := keys.New(append(Bindings("agenda"),
		keys.Binding{Action: "time.earlier", Keys: []string{"<"}, Ctx: "agenda", Help: "earlier"},
	)...)
	found := false
	for _, c := range gcl.Conflicts() {
		if c.Kind == keys.Duplicate && c.Key == "<" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a clash on < was not reported: %v", gcl.Conflicts())
	}

	// Resolved actions reach Handle.
	b, ok := km.Resolve(stack, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'>'}}, keys.Opts{})
	if !ok || b.Action != ActionGrow {
		t.Fatalf("> resolved to %v %v", b.Action, ok)
	}
}
