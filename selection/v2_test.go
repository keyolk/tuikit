package selection

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/kitty"
	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/tuitest"
)

// counted wraps panes and frame so a test sees how often Update built them.
type counted struct{ panes, frames int }

func (c *counted) fns(offset int) (func() []Pane, func() string) {
	return func() []Pane { c.panes++; return panesFor(offset) },
		func() string { c.frames++; return frameFor(offset) }
}

func TestUpdateBuildsNothingForEventsTheSelectionIgnores(t *testing.T) {
	var s Selection
	var c counted
	panes, frame := c.fns(0)
	for _, msg := range []tea.Msg{
		tea.MouseMsg{X: 12, Y: 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress},
		tea.MouseMsg{X: 12, Y: 1, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		tea.MouseMsg{X: 12, Y: 1, Action: tea.MouseActionMotion}, // hover
		tea.KeyMsg{Type: tea.KeyEnter},
	} {
		if r, cmd := s.Update(msg, panes, frame, t0); r.Consumed || cmd != nil {
			t.Fatalf("%v: consumed=%v cmd=%v", msg, r.Consumed, cmd != nil)
		}
	}
	if c.panes != 0 || c.frames != 0 {
		t.Fatalf("built panes %d times and frames %d times for ignored events", c.panes, c.frames)
	}
}

func TestUpdateReadsTheFrameOnlyWhenAPaneNeedsIt(t *testing.T) {
	var s Selection
	var c counted
	panes, frame := c.fns(0)
	// A drag inside the text pane, which passes its Lines: nothing reads the frame.
	s.Update(press(textX+6, 0), panes, frame, t0)
	s.Update(motion(textX+10, 1), panes, frame, t0)
	r, _ := s.Update(release(textX+10, 1), panes, frame, t0)
	if r.Copy == "" {
		t.Fatal("drag copied nothing")
	}
	if c.frames != 0 {
		t.Fatalf("frame built %d times for a pane with Lines", c.frames)
	}
	// The list has no Lines, so copying from it reads the frame, once.
	s.Update(press(0, 0), panes, frame, t0.Add(time.Second))
	s.Update(motion(5, 1), panes, frame, t0.Add(time.Second))
	s.Update(release(5, 1), panes, frame, t0.Add(time.Second))
	if c.frames != 1 {
		t.Fatalf("frame built %d times for one copy from the list, want 1", c.frames)
	}
}

func TestUpdateReturnsTheCopyCommand(t *testing.T) {
	var got []string
	old := Clipboard
	Clipboard = func(s string) error { got = append(got, s); return nil }
	t.Cleanup(func() { Clipboard = old })

	var s Selection
	panes, frame := func() []Pane { return panesFor(0) }, func() string { return frameFor(0) }
	s.Update(press(textX+6, 0), panes, frame, t0)
	s.Update(motion(textX+10, 0), panes, frame, t0)
	_, cmd := s.Update(release(textX+10, 0), panes, frame, t0)
	msgs := tuitest.Drain(cmd, time.Second)
	if len(msgs) != 1 {
		t.Fatalf("release returned %d messages, want the CopiedMsg", len(msgs))
	}
	m, ok := msgs[0].(CopiedMsg)
	if !ok || m.Text != "alpha" || m.Err != nil {
		t.Fatalf("got %#v, want CopiedMsg{alpha}", msgs[0])
	}
	if len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("clipboard got %q", got)
	}
}

func TestMotionWithNoButtonEndsADragInsteadOfExtendingIt(t *testing.T) {
	// With all-motion on, a release lost outside the window is followed by
	// hover. Hover must not drag the selection along.
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(textX+6, 0), p, f, t0)
	s.Mouse(motion(textX+10, 0), p, f, t0)
	hover := tea.MouseMsg{X: textX + 4, Y: 2, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}
	r := s.Mouse(hover, p, f, t0)
	if r.Copy != "alpha" {
		t.Fatalf("hover after a lost release copied %q, want the drag so far (alpha)", r.Copy)
	}
	if s.Dragging() {
		t.Fatal("still dragging after hover")
	}
	if r := s.Mouse(tea.MouseMsg{X: textX + 2, Y: 1, Action: tea.MouseActionMotion}, p, f, t0); r.Consumed {
		t.Fatal("more hover was consumed")
	}
	if _, _, end, _ := s.Range(); end != (Point{Row: 0, Col: 10}) {
		t.Fatalf("hover moved the selection end to %+v", end)
	}
}

func TestDragPastTheEdgeReportsTheTargetOffsetAndAutoScrolls(t *testing.T) {
	var s Selection
	offset := 0
	panes := func() []Pane { return panesFor(offset) }
	frame := func() string { return frameFor(offset) }
	s.Update(press(textX, 0), panes, frame, t0)
	r, _ := s.Update(motion(textX+3, textH+2), panes, frame, t0)
	if r.Scroll != 1 || r.ScrollPane != "text" || r.Offset != 1 {
		t.Fatalf("got scroll=%d pane=%q offset=%d, want 1 text 1", r.Scroll, r.ScrollPane, r.Offset)
	}
	if r.AutoScroll == nil {
		t.Fatal("no auto-scroll for a pointer resting past the edge")
	}
	offset = r.Offset

	// The pointer stays put; only ticks arrive. Each scrolls one more line
	// until the content ends.
	tick := tickOf(t, r.AutoScroll)
	for want := 2; ; want++ {
		r, _ = s.Update(tick, panes, frame, t0)
		if r.Scroll == 0 {
			break
		}
		if r.Offset != want {
			t.Fatalf("tick scrolled to %d, want %d", r.Offset, want)
		}
		offset = r.Offset
		if r.AutoScroll == nil {
			t.Fatalf("tick at offset %d did not schedule the next", offset)
		}
		tick = tickOf(t, r.AutoScroll)
	}
	if offset != len(docLines)-textH {
		t.Fatalf("auto-scroll stopped at %d, want the end (%d)", offset, len(docLines)-textH)
	}
	r, _ = s.Update(release(textX+3, textH+2), panes, frame, t0)
	// From line0 col 0 to line5 col 3, the pointer's column.
	if got := strings.Split(r.Copy, "\n"); len(got) != len(docLines) || got[0] != docLines[0] || got[len(got)-1] != "line" {
		t.Fatalf("copy %q does not run from line0 through the start of line5", r.Copy)
	}
}

func TestAStaleAutoScrollTickDoesNothing(t *testing.T) {
	for name, end := range map[string]func(*Selection, func() []Pane, func() string){
		"release": func(s *Selection, p func() []Pane, f func() string) { s.Update(release(textX, textH+2), p, f, t0) },
		"key":     func(s *Selection, _ func() []Pane, _ func() string) { s.Clear() },
		// A new drag after a key must not be scrolled by the old drag's tick.
		"key then a new press": func(s *Selection, p func() []Pane, f func() string) {
			s.Clear()
			s.Update(press(textX+1, 1), p, f, t0.Add(time.Second))
		},
		"back inside": func(s *Selection, p func() []Pane, f func() string) {
			s.Update(motion(textX+2, 1), p, f, t0)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var s Selection
			panes := func() []Pane { return panesFor(1) }
			frame := func() string { return frameFor(1) }
			s.Update(press(textX, 0), panes, frame, t0)
			r, _ := s.Update(motion(textX, textH+2), panes, frame, t0)
			tick := tickOf(t, r.AutoScroll)
			end(&s, panes, frame)
			if r, cmd := s.Update(tick, panes, frame, t0); r.Scroll != 0 || cmd != nil {
				t.Fatalf("tick after %s scrolled %d (cmd %v)", name, r.Scroll, cmd != nil)
			}
		})
	}
}

func TestAnOldDragsTickDoesNotDoubleTheNextDragsScroll(t *testing.T) {
	for name, end := range map[string]func(*Selection, func() []Pane, func() string){
		"release": func(s *Selection, p func() []Pane, f func() string) { s.Update(release(textX, textH+2), p, f, t0) },
		"key":     func(s *Selection, _ func() []Pane, _ func() string) { s.Clear() },
	} {
		t.Run(name, func(t *testing.T) {
			var s Selection
			panes := func() []Pane { return panesFor(1) }
			frame := func() string { return frameFor(1) }
			s.Update(press(textX, 0), panes, frame, t0)
			r, _ := s.Update(motion(textX, textH+2), panes, frame, t0)
			old := tickOf(t, r.AutoScroll)
			end(&s, panes, frame)
			// A new drag rests past the same edge: it has its own tick chain.
			s.Update(press(textX+1, 1), panes, frame, t0.Add(time.Second))
			r, _ = s.Update(motion(textX+1, textH+2), panes, frame, t0.Add(time.Second))
			tickOf(t, r.AutoScroll)
			if r, _ := s.Update(old, panes, frame, t0); r.Scroll != 0 {
				t.Fatalf("the old drag's tick scrolled the new drag after %s: two chains, double speed", name)
			}
		})
	}
}

func tickOf(t *testing.T, cmd tea.Cmd) AutoScrollMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no tick")
	}
	m, ok := cmd().(AutoScrollMsg)
	if !ok {
		t.Fatalf("tick delivered %T", m)
	}
	return m
}

func TestLineXKeepsTheGutterOutOfTheCopy(t *testing.T) {
	lines := []string{" 1 │ first", " 2 │ second", " 3 │ third"}
	const gutter = 5
	p := []Pane{{ID: "code", Rect: mouse.Rect{X: gutter, Y: 0, W: 10, H: 3}, Kind: Text, Lines: lines, LineX: gutter}}
	var s Selection
	s.Mouse(press(gutter, 0), p, strings.Join(lines, "\n"), t0)
	s.Mouse(motion(gutter+5, 1), p, strings.Join(lines, "\n"), t0)
	r := s.Mouse(release(gutter+5, 1), p, strings.Join(lines, "\n"), t0)
	if r.Copy != "first\nsecond" {
		t.Fatalf("copied %q, want first\\nsecond without the gutter", r.Copy)
	}
	// A double click reads the same cells: col 0 is 's' of second, not the gutter.
	s.Mouse(press(gutter, 1), p, "", t0.Add(time.Second))
	if r := s.Mouse(press(gutter, 1), p, "", t0.Add(time.Second)); r.Copy != "second" {
		t.Fatalf("word pick %q, want second", r.Copy)
	}
}

func TestTripleClickLeavesTheIndentOut(t *testing.T) {
	lines := []string{"Event", "  @ Room 7F-Orion   "}
	p := []Pane{{ID: "detail", Rect: mouse.Rect{X: 0, Y: 0, W: 20, H: 2}, Kind: Text, Lines: lines}}
	var s Selection
	var r Result
	for i := 0; i < 3; i++ {
		r = s.Mouse(press(12, 1), p, "", t0)
	}
	if r.Copy != "@ Room 7F-Orion" {
		t.Fatalf("triple click copied %q", r.Copy)
	}
	_, a, b, _ := s.Range()
	if a.Col != 2 || b.Col != 16 {
		t.Fatalf("line pick spans cols %d-%d, want 2-16 (the highlight too)", a.Col, b.Col)
	}
}

func TestKittyPlaceholdersAreNotCopiedNorRestyled(t *testing.T) {
	pic := kitty.Placeholder(7, 3, 1)[0] // three cells: rune + diacritics, colored by id
	row := "ab" + pic + "cd"
	p := []Pane{{ID: "chat", Rect: mouse.Rect{X: 0, Y: 0, W: 7, H: 1}, Kind: Text}}
	var s Selection
	s.Mouse(press(0, 0), p, row, t0)
	s.Mouse(motion(6, 0), p, row, t0)
	if r := s.Mouse(release(6, 0), p, row, t0); r.Copy != "abcd" {
		t.Fatalf("copied %q, want abcd without the picture", r.Copy)
	}
	out := s.Render(row, p, reverse)
	if got := tuitest.Reversed(out); strings.Join(got, "|") != "ab|cd" {
		t.Fatalf("highlighted %q, want ab and cd only", got)
	}
	if !strings.Contains(out, pic) {
		t.Fatalf("the picture's cells were restyled: %q", out)
	}
}

func TestNoColorKeepsTheHighlight(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := DefaultStyle().Render("x"); !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("DefaultStyle under NO_COLOR = %q, no reverse", got)
	}
	// An app style from the default renderer (a color, or even reverse) draws
	// nothing under NO_COLOR; Render must still show the selection.
	appStyle := lipgloss.NewRenderer(io.Discard).NewStyle().Background(lipgloss.Color("4")).Reverse(true)
	if appStyle.Render("x") != "x" {
		t.Skip("lipgloss now keeps SGR under NO_COLOR; the fallback is untested")
	}
	var s Selection
	f, p := frameFor(0), panesFor(0)
	s.Mouse(press(textX+6, 0), p, f, t0)
	s.Mouse(motion(textX+10, 0), p, f, t0)
	if got := tuitest.Reversed(s.Render(f, p, appStyle)); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("under NO_COLOR the highlight is %q, want alpha", got)
	}
}

func TestCopiedNote(t *testing.T) {
	if got := (CopiedMsg{Text: "한글 ab"}).Note(); got != "copied 5 chars" {
		t.Fatalf("got %q", got)
	}
	if got := (CopiedMsg{Err: errors.New("no tty")}).Note(); got != "copy failed: no tty" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderedHighlightMatchesTheCopyForAWideLine(t *testing.T) {
	// The highlight and the copy are computed separately; keep them agreeing.
	var s Selection
	f, p := frameFor(3), panesFor(3)
	s.Mouse(press(textX, 2), p, f, t0)
	s.Mouse(motion(textX+8, 2), p, f, t0)
	r := s.Mouse(release(textX+8, 2), p, f, t0)
	got := tuitest.Reversed(s.Render(f, p, reverse))
	if len(got) != 1 || ansi.Strip(got[0]) != r.Copy {
		t.Fatalf("highlight %q, copy %q", got, r.Copy)
	}
}
