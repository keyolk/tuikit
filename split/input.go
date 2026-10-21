package split

import (
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/mouse"
)

// Action names, for the app's binding table and user overrides.
const (
	ActionGrow   = "split.grow"
	ActionShrink = "split.shrink"
	ActionReset  = "split.reset"
)

// Bindings are the split's keys in ctx: `>` grow, `<` shrink, `=` reset,
// out of the footer (they are in help and the palette). Put them in the
// app's one binding table, so keys.Conflicts reports a clash: gcl binds
// `< >` to nudge times and rebinds these in keys.yaml or in its table.
// `[ ]` were not chosen because they mean tabs or history in argx, ccproxy,
// a9s and kkt; `{ }` are taken in ghx's diff and gcl.
func Bindings(ctx keys.Ctx) []keys.Binding {
	return []keys.Binding{
		{Action: ActionGrow, Keys: []string{">"}, Ctx: ctx, Group: "layout", Help: "grow first pane", Short: "grow"},
		{Action: ActionShrink, Keys: []string{"<"}, Ctx: ctx, Group: "layout", Help: "shrink first pane", Short: "shrink"},
		{Action: ActionReset, Keys: []string{"="}, Ctx: ctx, Group: "layout", Help: "reset split", Short: "reset"},
	}
}

// Handle runs a split action resolved from the binding table against the
// area the split is laid out in. Consumed is false for any other action.
func (m Model) Handle(action string, area mouse.Rect) (Model, Result) {
	before := m.Ratio()
	switch action {
	case ActionGrow:
		m = m.Grow(area)
	case ActionShrink:
		m = m.Shrink(area)
	case ActionReset:
		m = m.Reset(area)
	default:
		return m, Result{}
	}
	return m, Result{Consumed: true, Changed: m.Ratio() != before}
}

// grabRect is the divider's press target: the divider widened by Grab.
func (m Model) grabRect(div mouse.Rect) mouse.Rect {
	if div.H <= 0 && div.W <= 0 || m.Grab <= 0 {
		return div
	}
	if m.Orientation == Vertical {
		return mouse.Rect{X: div.X, Y: div.Y - m.Grab, W: div.W, H: div.H + 2*m.Grab}
	}
	return mouse.Rect{X: div.X - m.Grab, Y: div.Y, W: div.W + 2*m.Grab, H: div.H}
}

// along is the coordinate of (x, y) along the split's axis.
func (m Model) along(x, y int) int {
	if m.Orientation == Vertical {
		return y
	}
	return x
}

// Update is UpdateAt at time.Now().
func (m Model) Update(msg tea.MouseMsg, area mouse.Rect) (Model, Result) {
	return m.UpdateAt(msg, area, time.Now())
}

// UpdateAt feeds one mouse event, with area the rect the split is laid out
// in (the same one View passes to Layout). Call it before selection.Update
// and skip both selection and the app's click handling when Consumed:
//
//	case tea.MouseMsg:
//		var r split.Result
//		m.split, r = m.split.Update(msg, m.body)
//		if r.Changed { m.saver, cmd = m.saver.Save(m.uiState()) }
//		if r.Consumed { return m, cmd }
//		// selection.Update, then clicks and wheel
//
// A left press on the divider (or within Grab of it) starts a drag; motion
// with the left button moves the divider, clamped; release, or motion with
// no button (the release was lost outside the window), ends it. A second
// press on the divider within mouse.DoubleClickWindow, with no drag between,
// resets the ratio. Any press elsewhere is not consumed, so a pane press
// still reaches selection and the app.
func (m Model) UpdateAt(msg tea.MouseMsg, area mouse.Rect, now time.Time) (Model, Result) {
	switch {
	case mouse.IsLeftPress(msg):
		_, div, _ := m.Layout(area)
		if !m.grabRect(div).Contains(msg.X, msg.Y) {
			m.dragging, m.clicks = false, mouse.Clicks{}
			return m, Result{}
		}
		var double bool
		m.clicks, double = m.clicks.Press("divider", now)
		if double {
			before := m.Ratio()
			m = m.Reset(area)
			m.dragging = false
			return m, Result{Consumed: true, Changed: m.Ratio() != before}
		}
		m.dragging = true
		m.grabOff = m.along(msg.X, msg.Y) - m.along(div.X, div.Y)
		return m, Result{Consumed: true}
	case m.dragging && msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft:
		_, _, avail, ok := m.cells(area)
		if !ok || m.Collapsed(area) {
			return m, Result{Consumed: true}
		}
		before := m.Ratio()
		na := m.along(msg.X, msg.Y) - m.grabOff - m.along(area.X, area.Y)
		m = m.Set(float64(na)/float64(avail), area)
		changed := m.Ratio() != before
		if changed {
			// A press that went on to drag is not the first half of a
			// double click: re-grabbing right after a drag must not reset.
			m.clicks = mouse.Clicks{}
		}
		return m, Result{Consumed: true, Changed: changed}
	case m.dragging && (msg.Action == tea.MouseActionRelease ||
		msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonNone):
		m.dragging = false
		return m, Result{Consumed: true}
	}
	return m, Result{}
}

// State is what an app persists for the split, inside its prefs struct.
type State struct {
	Ratio float64 `json:"ratio,omitempty"`
}

// State returns the persistable state.
func (m Model) State() State { return State{Ratio: m.Ratio()} }

// Restore applies a saved State, clamped to the bounds. A ratio that is not
// a fraction strictly between 0 and 1 (NaN, a percentage like 50, a zero
// from a missing field) is rejected: the Model is returned unchanged and ok
// is false. The minimum sizes are enforced again at the next Layout.
func (m Model) Restore(s State) (Model, bool) {
	r := s.Ratio
	if math.IsNaN(r) || r <= 0 || r >= 1 {
		return m, false
	}
	m.ratio = m.clampBounds(r)
	return m, true
}
