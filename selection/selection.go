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
//     (Result.Scroll) and the selection keeps the lines that scrolled out,
//     because a text pane gives its whole content (Pane.Lines), not just the
//     visible rows.
//   - In a text pane a double click selects a word and a triple click a line.
//     List panes keep the fleet convention (E14): the second click on the
//     selected row is the Enter action, so multi-click selection is off there.
//   - Releasing copies the text (Copy), and the highlight stays until the next
//     press or key, as the engine does with copyOnSelect.
//
// A press that does not move is still a click: Mouse leaves it to the app
// (Result.Consumed is false), so click-to-select and wheel keep working.
package selection

import (
	"encoding/base64"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/render"
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
// so an overlay listed after the base owns its area.
type Pane struct {
	ID   string
	Rect mouse.Rect
	Kind Kind
	// Lines is the pane's full content, styled or plain, one entry per drawn
	// line, and Offset the index of the line drawn at Rect.Y. A pane that sets
	// Lines can be scrolled by a drag and keeps the lines that scroll out. A
	// pane without Lines is read from the frame and cannot scroll.
	Lines  []string
	Offset int
}

// Point is a cell in a pane: Row is a line of Pane.Lines (or a row of the
// pane's rect when it has no Lines), Col a cell column inside the rect.
type Point struct{ Row, Col int }

// Result says what Mouse did with an event.
type Result struct {
	// Consumed: the event belongs to the selection (a drag, the release that
	// ends one, a word or line pick). The app must not also treat it as a
	// click.
	Consumed bool
	// Copy is the selected text, set once when a selection is finished.
	// Pass it to Copy.
	Copy string
	// Scroll asks the app to scroll pane ScrollPane by this many lines
	// (negative is up), because a drag went past its edge.
	Scroll     int
	ScrollPane string
}

// Selection is the selection state. The zero value has no selection.
type Selection struct {
	pane     string
	anchor   Point
	focus    Point
	active   bool // a range exists and is drawn
	dragging bool
	moved    bool

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

// Clear drops the selection. Call it on every key event, in the same place the
// mouse.Clicks detector is reset: a key is the "next action" that ends the
// engine's selection.
func (s *Selection) Clear() {
	*s = Selection{lastAt: s.lastAt, lastPane: s.lastPane, lastCell: s.lastCell, pressRuns: s.pressRuns}
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

// Mouse feeds one mouse event. frame is the current View() output, used to
// read panes without Lines; now is injectable for tests.
func (s *Selection) Mouse(msg tea.MouseMsg, panes []Pane, frame string, now time.Time) Result {
	switch {
	case mouse.IsLeftPress(msg):
		return s.press(msg, panes, frame, now)
	case msg.Action == tea.MouseActionMotion && s.dragging:
		return s.drag(msg, panes)
	case msg.Action == tea.MouseActionRelease && s.dragging:
		return s.release(panes, frame)
	}
	return Result{}
}

func (s *Selection) press(msg tea.MouseMsg, panes []Pane, frame string, now time.Time) Result {
	p, ok := paneAt(panes, msg.X, msg.Y)
	s.active = false
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
	s.dragging, s.moved = true, false

	if p.Kind != Text || s.pressRuns < 2 {
		return Result{}
	}
	line := plainLine(p, cell.Row, frame)
	lo, hi := 0, max(ansi.StringWidth(line)-1, 0)
	if s.pressRuns == 2 {
		var found bool
		if lo, hi, found = wordAt(line, cell.Col); !found {
			return Result{Consumed: true}
		}
	}
	s.anchor, s.focus = Point{cell.Row, lo}, Point{cell.Row, hi}
	s.active, s.dragging = true, false
	if s.pressRuns >= 3 {
		s.pressRuns = 0 // a fourth click starts over instead of repeating
	}
	return Result{Consumed: true, Copy: s.text(panes, frame)}
}

func (s *Selection) drag(msg tea.MouseMsg, panes []Pane) Result {
	p, ok := paneByID(panes, s.pane)
	if !ok {
		return Result{}
	}
	r := Result{Consumed: true}
	y := msg.Y
	if p.Lines != nil {
		switch {
		case msg.Y < p.Rect.Y && p.Offset > 0:
			r.Scroll, r.ScrollPane = -1, p.ID
			y = p.Rect.Y - 1
		case msg.Y >= p.Rect.Y+p.Rect.H && p.Offset+p.Rect.H < len(p.Lines):
			r.Scroll, r.ScrollPane = 1, p.ID
			y = p.Rect.Y + p.Rect.H
		}
	}
	focus := pointIn(p, msg.X, y)
	if !s.moved && focus == s.anchor {
		return Result{}
	}
	s.focus, s.moved, s.active = focus, true, true
	return r
}

func (s *Selection) release(panes []Pane, frame string) Result {
	s.dragging = false
	if !s.moved {
		return Result{}
	}
	return Result{Consumed: true, Copy: s.text(panes, frame)}
}

// Text returns the selected text: lines joined with "\n", styles dropped,
// trailing blanks trimmed per line, only the selection's pane's columns.
func (s Selection) Text(panes []Pane, frame string) string { return s.text(panes, frame) }

func (s Selection) text(panes []Pane, frame string) string {
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
		out = append(out, strings.TrimRight(ansi.Cut(line, lo, hi+1), " "))
	}
	return strings.Join(out, "\n")
}

// Render draws the selection over frame (the View output) with style. It is
// pure; call it at the end of View. Rows outside the pane's visible window
// are skipped, and the selected cells lose their own colors for style's.
func (s Selection) Render(frame string, panes []Pane, style lipgloss.Style) string {
	id, a, b, ok := s.Range()
	if !ok {
		return frame
	}
	p, ok := paneByID(panes, id)
	if !ok {
		return frame
	}
	rows := strings.Split(frame, "\n")
	for y := p.Rect.Y; y < p.Rect.Y+p.Rect.H && y < len(rows); y++ {
		row := y - p.Rect.Y
		if p.Lines != nil {
			row += p.Offset
		}
		if row < a.Row || row > b.Row {
			continue
		}
		lo, hi := 0, p.Rect.W-1
		if row == a.Row {
			lo = a.Col
		}
		if row == b.Row {
			hi = b.Col
		}
		x := p.Rect.X + lo
		cells := ansi.Strip(ansi.Cut(rows[y], x, p.Rect.X+hi+1))
		if cells == "" {
			continue
		}
		frame = render.Overlay(frame, style.Render(cells), x, y)
	}
	return frame
}

// CopiedMsg reports the result of Copy.
type CopiedMsg struct {
	Text string
	Err  error
}

// Copy puts text on the clipboard off the UI thread. Locally it uses the
// platform tool (pbcopy, wl-copy, xclip), which never writes to the terminal
// the renderer is drawing on. Over SSH, or when no tool exists, it falls back
// to OSC 52 on /dev/tty, as the engine does; tmux forwards it with
// set-clipboard on.
func Copy(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return func() tea.Msg {
		return CopiedMsg{Text: text, Err: copyText(text)}
	}
}

var clipboardTools = map[string][][]string{
	"darwin": {{"pbcopy"}},
	"linux":  {{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}},
}

func copyText(text string) error {
	if os.Getenv("SSH_TTY") == "" && os.Getenv("SSH_CONNECTION") == "" {
		for _, argv := range clipboardTools[runtime.GOOS] {
			if _, err := exec.LookPath(argv[0]); err != nil {
				continue
			}
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
	}
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer tty.Close()
	_, err = tty.WriteString(OSC52(text))
	return err
}

// OSC52 is the clipboard-write escape for text.
func OSC52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}

// wordChar is the engine's word class: letters, digits and the characters
// paths and flags are made of, so a double click takes a whole path.
var wordChar = regexp.MustCompile(`^[\p{L}\p{N}_/.\-+~\\]$`)

// wordAt returns the cell columns of the word covering column col.
func wordAt(line string, col int) (lo, hi int, ok bool) {
	type cell struct {
		r      string
		lo, hi int
	}
	var cells []cell
	x := 0
	for _, r := range line {
		w := max(ansi.StringWidth(string(r)), 1)
		cells = append(cells, cell{string(r), x, x + w - 1})
		x += w
	}
	at := -1
	for i, c := range cells {
		if col >= c.lo && col <= c.hi {
			at = i
			break
		}
	}
	if at < 0 || !wordChar.MatchString(cells[at].r) {
		return 0, 0, false
	}
	first, last := at, at
	for first > 0 && wordChar.MatchString(cells[first-1].r) {
		first--
	}
	for last < len(cells)-1 && wordChar.MatchString(cells[last+1].r) {
		last++
	}
	return cells[first].lo, cells[last].hi, true
}

// plainLine is row of p without styles, as cells of the pane only.
func plainLine(p Pane, row int, frame string) string {
	if p.Lines != nil {
		if row < 0 || row >= len(p.Lines) {
			return ""
		}
		return ansi.Truncate(ansi.Strip(p.Lines[row]), p.Rect.W, "")
	}
	rows := strings.Split(frame, "\n")
	y := p.Rect.Y + row
	if y < 0 || y >= len(rows) {
		return ""
	}
	return ansi.Strip(ansi.Cut(rows[y], p.Rect.X, p.Rect.X+p.Rect.W))
}

// pointIn maps a screen cell to a point of p, clamped to the pane: columns
// never leave it (the engine's scope), rows stay within its lines.
func pointIn(p Pane, x, y int) Point {
	col := min(max(x-p.Rect.X, 0), max(p.Rect.W-1, 0))
	row := y - p.Rect.Y
	if p.Lines != nil {
		row = min(max(row+p.Offset, 0), max(len(p.Lines)-1, 0))
	} else {
		row = min(max(row, 0), max(p.Rect.H-1, 0))
	}
	return Point{Row: row, Col: col}
}

func paneAt(panes []Pane, x, y int) (Pane, bool) {
	for i := len(panes) - 1; i >= 0; i-- {
		if panes[i].Rect.Contains(x, y) {
			return panes[i], true
		}
	}
	return Pane{}, false
}

func paneByID(panes []Pane, id string) (Pane, bool) {
	for _, p := range panes {
		if p.ID == id {
			return p, true
		}
	}
	return Pane{}, false
}
