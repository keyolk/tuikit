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
// a9s and kkt; `{ }` are taken in ghx's diff and gcl. The order is kept
// for the apps that assert it (ghx, narwhal); BindingsWith lists shrink
// first, the order a footer reads.
func Bindings(ctx keys.Ctx) []keys.Binding {
	return []keys.Binding{
		{Action: ActionGrow, Keys: []string{">"}, Ctx: ctx, Group: "layout", Help: "grow first pane", Short: "grow"},
		{Action: ActionShrink, Keys: []string{"<"}, Ctx: ctx, Group: "layout", Help: "shrink first pane", Short: "shrink"},
		{Action: ActionReset, Keys: []string{"="}, Ctx: ctx, Group: "layout", Help: "reset split", Short: "reset"},
	}
}

// FooterGroup is the footer hint BindingsWith collapses the three keys into.
const FooterGroup = "resize"

// BindingsWith is Bindings in the order shrink, grow, reset, so the footer
// reads `<>= resize`, under the app's help group and in the footer at
// priority footer as one FooterGroup hint. footer 0 keeps them out of the
// footer and leaves FooterGroup empty. Set Help on the result for wording
// that names the panes.
func BindingsWith(ctx keys.Ctx, group string, footer int) []keys.Binding {
	bs := Bindings(ctx)
	bs[0], bs[1] = bs[1], bs[0]
	for i := range bs {
		bs[i].Group, bs[i].Footer = group, footer
		if footer != 0 {
			bs[i].FooterGroup = FooterGroup
		}
	}
	return bs
}

// Handle runs a split action resolved from the binding table against the
// area the split is laid out in. Consumed is false for any other action.
func (m Model) Handle(action string, area mouse.Rect) (Model, Result) {
	before := m.stateIn(area)
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
	return m, Result{Consumed: true, Changed: m.stateIn(area) != before, Reset: action == ActionReset}
}

// grabs is how far the press target reaches into A and into B.
func (m Model) grabs() (a, b int) {
	if m.GrabA > 0 || m.GrabB > 0 {
		return max(m.GrabA, 0), max(m.GrabB, 0)
	}
	return max(m.Grab, 0), max(m.Grab, 0)
}

// grabRect is the divider's press target: the divider widened by GrabA
// into A and GrabB into B (or Grab into both).
func (m Model) grabRect(div mouse.Rect) mouse.Rect {
	ga, gb := m.grabs()
	if div.H <= 0 && div.W <= 0 || ga+gb == 0 {
		return div
	}
	if m.Orientation == Vertical {
		return mouse.Rect{X: div.X, Y: div.Y - ga, W: div.W, H: div.H + ga + gb}
	}
	return mouse.Rect{X: div.X - ga, Y: div.Y, W: div.W + ga + gb, H: div.H}
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
// in (the same one View passes to Layout). The order is split, then
// selection, then the app's clicks: call it before selection.Update, clear
// the selection on Pressed, and skip both selection and the app's click
// handling when Consumed:
//
//	case tea.MouseMsg:
//		var r split.Result
//		m.split, r = m.split.Update(msg, m.body)
//		if r.Pressed { m.sel.Clear() }
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
			before := m.stateIn(area)
			m = m.Reset(area)
			m.dragging = false
			return m, Result{Consumed: true, Pressed: true, Reset: true, Changed: m.stateIn(area) != before}
		}
		m.dragging = true
		m.grabOff = m.along(msg.X, msg.Y) - m.along(div.X, div.Y)
		return m, Result{Consumed: true, Pressed: true}
	case m.dragging && msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft:
		_, _, avail, ok := m.cells(area)
		if !ok || m.Collapsed(area) {
			return m, Result{Consumed: true}
		}
		na := m.along(msg.X, msg.Y) - m.grabOff - m.along(area.X, area.Y)
		n := m.Set(float64(na)/float64(avail), area)
		if n.sizeA(area) == m.sizeA(area) {
			// The divider did not move a cell: a press with a jitter, or a
			// drag against a limit, does not make an unset split set.
			return m, Result{Consumed: true}
		}
		// A press that went on to drag is not the first half of a double
		// click: re-grabbing right after a drag must not reset.
		n.clicks = mouse.Clicks{}
		return n, Result{Consumed: true, Changed: true}
	case m.dragging && (msg.Action == tea.MouseActionRelease ||
		msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonNone):
		m.dragging = false
		return m, Result{Consumed: true}
	}
	return m, Result{}
}

// State is what an app persists for the split, inside its prefs struct.
// The zero State (`{}` in JSON) is an unset split.
type State struct {
	Ratio float64 `json:"ratio,omitempty"`
}

// State returns the persistable state: the zero State for an unset split,
// so a split nobody moved keeps following Default (or DefaultFor) after a
// restart instead of pinning the ratio it happened to show.
func (m Model) State() State {
	if !m.IsSet() {
		return State{}
	}
	return State{Ratio: m.Ratio()}
}

// Restore applies a saved State, clamped to the bounds. A ratio that is not
// a fraction strictly between 0 and 1 (NaN, a percentage like 50, a zero
// from a missing field or an unset split) is rejected: the Model is
// returned unchanged (unset, if it was) and ok is false. The minimum sizes are enforced again at the next Layout.
func (m Model) Restore(s State) (Model, bool) {
	r := s.Ratio
	if math.IsNaN(r) || r <= 0 || r >= 1 {
		return m, false
	}
	m.ratio = m.clampBounds(r)
	return m, true
}
