package selection

import (
	"encoding/base64"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/keyolk/tuikit/mouse"
)

// Two panes side by side, as most fleet tools draw them: a list on the left
// (cells 0-9), a border at cell 10, a text pane on the right (cells 11-30).
const (
	listW = 10
	textX = 11
	textW = 20
	textH = 3
)

var docLines = []string{
	"line0 alpha",
	"line1 ~/src/keyolk/tuikit",
	"line2 gamma",
	"line3 delta",
	"line4 epsilon",
	"line5 한글 단어",
}

func frameFor(offset int) string {
	left := []string{"item-one", "item-two", "item-three"}
	var rows []string
	for i := 0; i < textH; i++ {
		l := left[i] + strings.Repeat(" ", listW-len(left[i]))
		r := ""
		if offset+i < len(docLines) {
			r = docLines[offset+i]
		}
		r += strings.Repeat(" ", max(textW-ansi.StringWidth(r), 0))
		rows = append(rows, l+"│"+r)
	}
	return strings.Join(rows, "\n")
}

func panesFor(offset int) []Pane {
	return []Pane{
		{ID: "list", Rect: mouse.Rect{X: 0, Y: 0, W: listW, H: textH}, Kind: List},
		{ID: "text", Rect: mouse.Rect{X: textX, Y: 0, W: textW, H: textH}, Kind: Text, Lines: docLines, Offset: offset},
	}
}

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func motion(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion}
}

func release(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}
}

var t0 = time.Unix(1000, 0)

func TestDragIsClampedToThePaneUnderThePress(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	if r := s.Mouse(press(0, 0), p, f, t0); r.Consumed {
		t.Fatal("a press alone must stay a click for the app")
	}
	// Drag from the list across the border deep into the text pane.
	if r := s.Mouse(motion(25, 1), p, f, t0); !r.Consumed {
		t.Fatal("a drag must be consumed")
	}
	r := s.Mouse(release(25, 1), p, f, t0)
	want := "item-one\nitem-two"
	if r.Copy != want {
		t.Fatalf("copied %q, want %q (columns must stay in the list pane)", r.Copy, want)
	}
	// The highlight must stop at the pane edge too, not paint the border and
	// the neighbor (the copied text alone would not show that).
	for i, row := range strings.Split(s.Render(f, p, reverse), "\n")[:2] {
		at := strings.Index(row, "\x1b[7m")
		if at < 0 {
			t.Fatalf("row %d not highlighted: %q", i, row)
		}
		lit := row[at:]
		if end := strings.Index(lit, "\x1b[0m"); end >= 0 {
			lit = lit[:end]
		}
		if w := ansi.StringWidth(lit); w > listW {
			t.Fatalf("row %d highlight is %d cells, past the %d-cell list pane: %q", i, w, listW, row)
		}
	}
}

func TestMultiLineDragTakesOnlyThePaneColumnsOnMiddleRows(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(textX+6, 0), p, f, t0) // "alpha" on line0
	s.Mouse(motion(textX+4, 2), p, f, t0)
	r := s.Mouse(release(textX+4, 2), p, f, t0)
	want := "alpha\nline1 ~/src/keyolk/t\nline2"
	if r.Copy != want {
		t.Fatalf("copied %q, want %q", r.Copy, want)
	}
	if strings.Contains(r.Copy, "item") || strings.Contains(r.Copy, "│") {
		t.Fatalf("selection leaked out of the text pane: %q", r.Copy)
	}
}

func TestPressWithoutMovingIsAClick(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(textX+2, 1), p, f, t0)
	r := s.Mouse(release(textX+2, 1), p, f, t0)
	if r.Consumed || r.Copy != "" || s.Active() {
		t.Fatalf("press+release without motion: %+v active=%v", r, s.Active())
	}
}

func TestDragPastTheBottomScrollsAndKeepsScrolledOutLines(t *testing.T) {
	var s Selection
	off := 0
	s.Mouse(press(textX, 0), panesFor(off), frameFor(off), t0)
	for i := 0; i < 3; i++ {
		r := s.Mouse(motion(textX+4, textH+1), panesFor(off), frameFor(off), t0)
		if r.Scroll != 1 || r.ScrollPane != "text" {
			t.Fatalf("step %d: drag below the pane: scroll=%d pane=%q", i, r.Scroll, r.ScrollPane)
		}
		off += r.Scroll // the app scrolls its pane
	}
	r := s.Mouse(release(textX+4, textH-1), panesFor(off), frameFor(off), t0)
	if !strings.HasPrefix(r.Copy, "line0 alpha\nline1") || !strings.HasSuffix(r.Copy, "\nline5") {
		t.Fatalf("copied %q; want line0 (scrolled out) through line5", r.Copy)
	}
}

func TestDragPastTheEdgeOfAPaneWithoutLinesDoesNotScroll(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(1, 0), p, f, t0)
	r := s.Mouse(motion(3, 9), p, f, t0)
	if r.Scroll != 0 {
		t.Fatalf("a list pane without Lines asked to scroll %d", r.Scroll)
	}
	if got := s.Text(p, f); got != "tem-one\nitem-two\nitem" {
		t.Fatalf("rows must clamp to the pane: %q", got)
	}
}

func TestDoubleClickInTextSelectsAWholePath(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	x := textX + 10 // inside ~/src/keyolk/tuikit on line1
	s.Mouse(press(x, 1), p, f, t0)
	s.Mouse(release(x, 1), p, f, t0)
	r := s.Mouse(press(x, 1), p, f, t0.Add(100*time.Millisecond))
	if !r.Consumed || r.Copy != "~/src/keyolk/t" {
		t.Fatalf("double click: %+v; want the visible path part", r)
	}
}

func TestTripleClickInTextSelectsTheLine(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	for i := 0; i < 3; i++ {
		r := s.Mouse(press(textX+1, 2), p, f, t0.Add(time.Duration(i)*100*time.Millisecond))
		if i == 2 && r.Copy != "line2 gamma" {
			t.Fatalf("triple click copied %q", r.Copy)
		}
		s.Mouse(release(textX+1, 2), p, f, t0)
	}
}

func TestDoubleClickInAListIsLeftToTheApp(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(2, 1), p, f, t0)
	s.Mouse(release(2, 1), p, f, t0)
	if r := s.Mouse(press(2, 1), p, f, t0.Add(100*time.Millisecond)); r.Consumed || s.Active() {
		t.Fatalf("a list's second click is the app's Enter (E14): %+v", r)
	}
}

func TestSlowSecondClickIsNotAWordPick(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(textX+7, 0), p, f, t0)
	s.Mouse(release(textX+7, 0), p, f, t0)
	if r := s.Mouse(press(textX+7, 0), p, f, t0.Add(time.Second)); r.Consumed {
		t.Fatalf("a click after the window picked a word: %+v", r)
	}
}

func TestHangulWordUsesCellColumns(t *testing.T) {
	var s Selection
	off := 3 // line5 visible at row 2
	f, p := frameFor(off), panesFor(off)
	x := textX + 6 + 4 + 1 // "line5 " (6) + "한글 " (5 cells) → inside 단어
	s.Mouse(press(x, 2), p, f, t0)
	s.Mouse(release(x, 2), p, f, t0)
	if r := s.Mouse(press(x, 2), p, f, t0.Add(50*time.Millisecond)); r.Copy != "단어" {
		t.Fatalf("Hangul word: %+v", r)
	}
}

func TestRenderHighlightsOnlyTheSelectedPaneCells(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(textX, 0), p, f, t0)
	s.Mouse(motion(textX+4, 1), p, f, t0)
	out := s.Render(f, p, reverse)
	if ansi.Strip(out) != f {
		t.Fatalf("Render changed the text:\n%s\nwant\n%s", ansi.Strip(out), f)
	}
	rows := strings.Split(out, "\n")
	// The highlight must start at the text pane's first cell: ansi.Cut keeps
	// escapes outside its range, so measure where SGR 7 begins instead.
	at := strings.Index(rows[0], "\x1b[7m")
	if at < 0 {
		t.Fatalf("the selected cells are not highlighted: %q", rows[0])
	}
	if col := ansi.StringWidth(rows[0][:at]); col != textX {
		t.Fatalf("highlight starts at cell %d, want %d (the text pane): %q", col, textX, rows[0])
	}
	if rows[2] != strings.Split(f, "\n")[2] {
		t.Fatalf("an unselected row changed: %q", rows[2])
	}
}

// reverse is forced to emit SGR 7: lipgloss drops styles when tests run
// without a terminal.
var reverse = func() lipgloss.Style {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	return r.NewStyle().Reverse(true)
}()

func TestClearDropsTheSelection(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(textX, 0), p, f, t0)
	s.Mouse(motion(textX+3, 0), p, f, t0)
	s.Clear()
	if s.Active() || s.Render(f, p, lipgloss.NewStyle().Reverse(true)) != f {
		t.Fatal("Clear left a selection behind")
	}
}

func TestWheelAndRightClickAreNotTouched(t *testing.T) {
	var s Selection
	f, p := frameFor(0), panesFor(0)
	for _, m := range []tea.MouseMsg{
		{X: textX, Y: 0, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress},
		{X: textX, Y: 0, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		motion(textX, 0), // motion without a press first
	} {
		if r := s.Mouse(m, p, f, t0); r.Consumed {
			t.Fatalf("%+v was consumed", m)
		}
	}
}

func TestOverlayPaneListedLastOwnsItsArea(t *testing.T) {
	var s Selection
	f := frameFor(0)
	p := append(panesFor(0), Pane{ID: "modal", Rect: mouse.Rect{X: 2, Y: 0, W: 5, H: 2}, Kind: Text})
	s.Mouse(press(3, 0), p, f, t0)
	s.Mouse(motion(20, 1), p, f, t0)
	if id, _, _, _ := s.Range(); id != "modal" {
		t.Fatalf("selection pane = %q, want the overlay", id)
	}
}

func TestOSC52(t *testing.T) {
	got := OSC52("hi 한")
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("hi 한")) + "\a"
	if got != want {
		t.Fatalf("OSC52 = %q", got)
	}
}
