// Package selection is mouse text selection that stays inside one pane, the
// way Claude Code's mod engine selects text.
//
// With mouse capture on, the terminal's own drag-to-select crosses every pane
// on the screen (borders, the neighbor list, the footer) and only works with
// Shift held. This package does what the engine does instead:
//
//   - A press picks the pane under it; the selection's columns are clamped to
//     that pane, so a drag never reaches into a neighbor. Rows run across
//     lines inside the pane.
//   - Dragging past the top or bottom of a text pane asks the app to scroll it
//     (Result.Scroll, Result.Offset) and the selection keeps the lines that
//     scrolled out, because a text pane gives its whole content (Pane.Lines),
//     not just the visible rows. While the pointer rests past the edge the
//     scroll goes on by itself (Result.AutoScroll), as the engine does: a
//     terminal sends no motion for a pointer that does not move.
//   - In a text pane a double click selects a word and a triple click a line.
//     List panes keep the fleet convention (E14): the second click on the
//     selected row is the Enter action, so multi-click selection is off there.
//   - Releasing copies the text (Copy), and the highlight stays until the next
//     press or key, as the engine does with copyOnSelect.
//
// A press that does not move is still a click: the selection leaves it to the
// app (Result.Consumed is false), so click-to-select and wheel keep working.
//
// # Wiring
//
// In Update, feed every message to Update before the app's own mouse
// handling, and Clear on every key, where mouse.Clicks is reset:
//
//	case tea.MouseMsg, selection.AutoScrollMsg:
//		r, cmd := m.sel.Update(msg, m.panes, m.frame, time.Now())
//		if r.Scroll != 0 {
//			m.setOffset(r.ScrollPane, r.Offset)
//		}
//		if r.Consumed {
//			return m, cmd
//		}
//		// the app's click and wheel handling; batch cmd with its command
//
// m.frame is View before Render (see Render), and it runs only when the
// selection needs it. At the end of View, draw the highlight:
//
//	return m.sel.Render(frame, m.panes(), selection.DefaultStyle())
//
// The selection sees an event before the app does, so a press is hit-tested
// against the layout on screen. If the app's click handler then changes the
// layout (moves focus, scrolls to keep a cursor visible), the selection's
// anchor still names the line that was clicked.
package selection

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/mouse"
)

// Kind decides what a double click means in a pane.
type Kind int

const (
	// List panes keep "second click on the selected row = Enter" (E14).
	List Kind = iota
	// Text panes (detail, preview, logs) select a word on a double click and
	// a line on a triple click.
	Text
)

// Pane is one region a selection can live in, with the numbers View drew it
// with. Pass the panes in drawing order; the last one containing a point wins,
// so an overlay listed after the base owns its area. While an overlay is open,
// pass only its panes: the base must not be selectable under a modal.
type Pane struct {
	ID   string
	Rect mouse.Rect
	Kind Kind
	// Lines is the pane's full content, styled or plain, one entry per line as
	// drawn: after wrapping, never the source lines a wrap splits. Offset is
	// the index of the line drawn at Rect.Y. A pane that sets Lines can be
	// scrolled by a drag and keeps the lines that scroll out. A pane without
	// Lines is read from the frame and cannot scroll.
	//
	// Rect.H counts only the rows that draw Lines[Offset:]. Leave out rows the
	// pane draws below them, such as a `… N more` marker: Lines does not hold
	// them, so counting them shifts nothing on screen but lets a drag past the
	// last drawn line stop short of scrolling.
	Lines  []string
	Offset int
	// LineX is the cell column of each Lines entry drawn at Rect.X. Set it
	// when the lines carry a gutter (line numbers, a marker column) the pane
	// does not want selected: Lines keeps the gutter, Rect starts after it.
	LineX int
}

// Point is a cell in a pane: Row is a line of Pane.Lines (or a row of the
// pane's rect when it has no Lines), Col a cell column inside the rect.
type Point struct{ Row, Col int }

// Result says what the selection did with an event.
type Result struct {
	// Consumed: the event belongs to the selection (a drag, the release that
	// ends one, a word or line pick). The app must not also treat it as a
	// click. A press that does not move is never consumed, so the app's own
	// click always runs; Consumed only matters for the events after it.
	Consumed bool
	// Copy is the selected text, set once when a selection is finished.
	// Update already returns Copy(Copy) as its command.
	Copy string
	// Scroll asks the app to scroll pane ScrollPane by this many lines
	// (negative is up), because a drag went past its edge. Offset is that
	// pane's offset after the scroll, Pane.Offset+Scroll, and is the safer
	// one to apply: it does not depend on the app's own scroll value staying
	// in range (an unclamped detail offset would swallow a -1).
	Scroll     int
	ScrollPane string
	Offset     int
	// AutoScroll keeps a drag scrolling while the pointer rests past the
	// pane's edge. It is a tea.Tick that delivers an AutoScrollMsg; return it
	// from Update and feed the message back. Update returns it batched with
	// the copy.
	AutoScroll tea.Cmd
}

// AutoScrollMsg is the tick behind Result.AutoScroll. Feed it to Update (or
// Tick); a tick from an earlier drag is ignored.
type AutoScrollMsg struct{ gen int }

// AutoScrollInterval is how often a drag resting past a pane's edge scrolls
// one more line.
var AutoScrollInterval = 50 * time.Millisecond

// Selection is the selection state. The zero value has no selection.
type Selection struct {
	pane     string
	anchor   Point
	focus    Point
	active   bool // a range exists and is drawn
	dragging bool
	moved    bool

	// The last pointer cell of the drag, replayed by an auto-scroll tick.
	lastX, lastY int
	autoGen      int
	autoPending  bool

	lastAt    time.Time
	lastPane  string
	lastCell  Point
	pressRuns int
}

// MultiClickWindow matches mouse.DoubleClickWindow so a word pick and a row
// activation feel the same.
var MultiClickWindow = mouse.DoubleClickWindow

// Active reports whether a selection is drawn.
func (s Selection) Active() bool { return s.active }

// Dragging reports whether the left button is down on a selection.
func (s Selection) Dragging() bool { return s.dragging }

// Wants reports whether msg changes the selection. The app can skip building
// panes and frames for every other event (wheel, plain motion, right click).
func (s Selection) Wants(msg tea.MouseMsg) bool {
	if mouse.IsLeftPress(msg) {
		return true
	}
	return s.dragging && (msg.Action == tea.MouseActionMotion || msg.Action == tea.MouseActionRelease)
}

// Clear drops the selection. Call it on every key event, in the same place the
// mouse.Clicks detector is reset: a key is the "next action" that ends the
// engine's selection.
func (s *Selection) Clear() {
	*s = Selection{
		lastAt: s.lastAt, lastPane: s.lastPane, lastCell: s.lastCell, pressRuns: s.pressRuns,
		// Generations only grow, so a tick still in flight can never match
		// the next drag's.
		autoGen: s.autoGen,
	}
}

// Range returns the selection's pane and its start and end, start first.
func (s Selection) Range() (pane string, start, end Point, ok bool) {
	if !s.active {
		return "", Point{}, Point{}, false
	}
	a, b := s.anchor, s.focus
	if b.Row < a.Row || (b.Row == a.Row && b.Col < a.Col) {
		a, b = b, a
	}
	return s.pane, a, b, true
}

// Update is the one call an app's Update needs. It takes a tea.MouseMsg or an
// AutoScrollMsg (anything else is ignored), builds panes and the frame only
// when the event needs them, and returns the Result with a command that
// copies a finished selection and keeps an auto-scroll going.
//
// frame must be View before Render: the selection reads the frame to copy
// panes without Lines, and a frame that already has the highlight would copy
// the highlight's styling with it.
func (s *Selection) Update(msg tea.Msg, panes func() []Pane, frame func() string, now time.Time) (Result, tea.Cmd) {
	var r Result
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if !s.Wants(msg) {
			return Result{}, nil
		}
		r = s.Feed(msg, panes(), frame, now)
	case AutoScrollMsg:
		r = s.Tick(msg, panes())
	default:
		return Result{}, nil
	}
	return r, tea.Batch(Copy(r.Copy), r.AutoScroll)
}

// Mouse feeds one mouse event. frame is the current View() output before
// Render, used to read panes without Lines; now is injectable for tests.
// Feed takes the frame lazily; Update also builds the panes lazily.
func (s *Selection) Mouse(msg tea.MouseMsg, panes []Pane, frame string, now time.Time) Result {
	return s.Feed(msg, panes, func() string { return frame }, now)
}

// Feed is Mouse with the frame built only when the event reads it: a press
// on a pane without Lines, a word or line pick, the release that copies.
func (s *Selection) Feed(msg tea.MouseMsg, panes []Pane, frame func() string, now time.Time) Result {
	lazy := &lazyFrame{build: frame}
	switch {
	case mouse.IsLeftPress(msg):
		return s.press(msg, panes, lazy, now)
	case msg.Action == tea.MouseActionMotion && s.dragging && msg.Button == tea.MouseButtonLeft:
		return s.drag(msg.X, msg.Y, panes)
	case msg.Action == tea.MouseActionRelease && s.dragging,
		// Motion with no button held means the release was lost (it happened
		// outside the window). Hover must never extend a selection, so the
		// drag ends here as if released.
		msg.Action == tea.MouseActionMotion && s.dragging && msg.Button == tea.MouseButtonNone:
		return s.release(panes, lazy)
	}
	return Result{}
}

// Tick handles an AutoScrollMsg: while the drag still rests past a pane's
// edge it scrolls one more line and schedules the next tick.
func (s *Selection) Tick(msg AutoScrollMsg, panes []Pane) Result {
	if msg.gen != s.autoGen || !s.dragging {
		return Result{}
	}
	s.autoPending = false
	return s.drag(s.lastX, s.lastY, panes)
}

type lazyFrame struct {
	build func() string
	done  bool
	rows  []string
}

func (f *lazyFrame) lines() []string {
	if !f.done {
		f.done = true
		if f.build != nil {
			f.rows = strings.Split(f.build(), "\n")
		}
	}
	return f.rows
}

func (s *Selection) press(msg tea.MouseMsg, panes []Pane, frame *lazyFrame, now time.Time) Result {
	p, ok := paneAt(panes, msg.X, msg.Y)
	s.active = false
	s.stopAuto()
	if !ok {
		s.dragging = false
		s.pressRuns = 0
		return Result{}
	}
	cell := pointIn(p, msg.X, msg.Y)
	if p.ID == s.lastPane && cell == s.lastCell && now.Sub(s.lastAt) <= MultiClickWindow {
		s.pressRuns++
	} else {
		s.pressRuns = 1
	}
	s.lastAt, s.lastPane, s.lastCell = now, p.ID, cell
	s.pane, s.anchor, s.focus = p.ID, cell, cell
	s.lastX, s.lastY = msg.X, msg.Y
	s.dragging, s.moved = true, false

	if p.Kind != Text || s.pressRuns < 2 {
		return Result{}
	}
	line := plainLine(p, cell.Row, frame)
	var lo, hi int
	var found bool
	if s.pressRuns == 2 {
		lo, hi, found = wordAt(line, cell.Col)
	} else {
		lo, hi, found = lineSpan(line)
	}
	if !found {
		return Result{Consumed: true}
	}
	s.anchor, s.focus = Point{cell.Row, lo}, Point{cell.Row, hi}
	s.active, s.dragging = true, false
	if s.pressRuns >= 3 {
		s.pressRuns = 0 // a fourth click starts over instead of repeating
	}
	return Result{Consumed: true, Copy: s.text(panes, frame)}
}

// drag moves the focus to the cell (x, y). Past the top or bottom of a pane
// with Lines it scrolls one line and keeps the line that scrolled out.
func (s *Selection) drag(x, y int, panes []Pane) Result {
	p, ok := paneByID(panes, s.pane)
	if !ok {
		return Result{}
	}
	s.lastX, s.lastY = x, y
	r := Result{Consumed: true}
	if p.Lines != nil {
		switch {
		case y < p.Rect.Y && p.Offset > 0:
			r.Scroll = -1
			y = p.Rect.Y - 1
		case y >= p.Rect.Y+p.Rect.H && p.Offset+p.Rect.H < len(p.Lines):
			r.Scroll = 1
			y = p.Rect.Y + p.Rect.H
		}
		if r.Scroll != 0 {
			r.ScrollPane, r.Offset = p.ID, p.Offset+r.Scroll
		}
	}
	focus := pointIn(p, x, y)
	if !s.moved && focus == s.anchor && r.Scroll == 0 {
		return Result{}
	}
	s.focus, s.moved, s.active = focus, true, true
	if r.Scroll != 0 && !s.autoPending {
		s.autoPending = true
		s.autoGen++
		gen := s.autoGen
		r.AutoScroll = tea.Tick(AutoScrollInterval, func(time.Time) tea.Msg { return AutoScrollMsg{gen: gen} })
	}
	return r
}

func (s *Selection) release(panes []Pane, frame *lazyFrame) Result {
	s.dragging = false
	s.stopAuto()
	if !s.moved {
		return Result{}
	}
	return Result{Consumed: true, Copy: s.text(panes, frame)}
}

func (s *Selection) stopAuto() {
	if s.autoPending {
		s.autoPending = false
		s.autoGen++
	}
}

// Text returns the selected text: lines joined with "\n", styles dropped,
// trailing blanks trimmed per line, only the selection's pane's columns, and
// no kitty image placeholder cells. frame is View before Render.
func (s Selection) Text(panes []Pane, frame string) string {
	return s.text(panes, &lazyFrame{build: func() string { return frame }})
}

func (s Selection) text(panes []Pane, frame *lazyFrame) string {
	id, a, b, ok := s.Range()
	if !ok {
		return ""
	}
	p, ok := paneByID(panes, id)
	if !ok {
		return ""
	}
	var out []string
	for row := a.Row; row <= b.Row; row++ {
		line := plainLine(p, row, frame)
		lo, hi := 0, p.Rect.W-1
		if row == a.Row {
			lo = a.Col
		}
		if row == b.Row {
			hi = b.Col
		}
		out = append(out, strings.TrimRight(stripPlaceholders(ansi.Cut(line, lo, hi+1)), " "))
	}
	return strings.Join(out, "\n")
}
