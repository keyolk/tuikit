package surface

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keyolk/tuikit/input"
	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/link"
	"github.com/keyolk/tuikit/list"
	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/selection"
	"github.com/keyolk/tuikit/split"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/toast"
)

// LineMsg carries one line of the agent's JSONL stream into Update. Run sends
// it; a test sends it directly.
type LineMsg []byte

// EndMsg says the stream ended. The last frame stays up: the user may still
// read it and trigger actions until they quit.
type EndMsg struct{ Err error }

// Sink receives every event the renderer emits, in order. It must not block
// the UI: Run's sink queues and reports a full queue as an error.
type Sink func(Event) error

// Options configure a Model.
type Options struct {
	Theme theme.Theme          // zero: theme.New(nil)
	Logf  func(string, ...any) // where malformed messages and pane panics go; nil discards
	Now   func() time.Time     // event timestamps and toast clock; nil is time.Now
	// Launch opens a validated http(s) URL (link.OpenWith); nil is the
	// platform opener. Tests stub it here instead of the package-wide
	// link.Launcher.
	Launch func(string) error
	// MinWidth and MinHeight are the floor below which only a "too small"
	// message is drawn. Zero is 80×24.
	MinWidth, MinHeight int
}

// Model is the Bubble Tea model of the renderer.
type Model struct {
	st      state
	ui      ui
	km      keys.Keymap
	actions []agentAction
	placed  []placed
	links   []string

	reported map[string]string // action id -> the key problem already toasted
	toasts   toast.Queue
	th       theme.Theme
	logf     func(string, ...any)
	now      func() time.Time
	sink     Sink
	launch   func(string) error

	w, h, minW, minH int

	help    bool
	helpOff int
	palette bool
	query   input.Field
	pick    int
	pending agentAction
	confirm bool
	editing bool // the focused input owns the keyboard
	sel     selection.Selection
	clicks  mouse.Clicks
}

// New returns a renderer model that sends its events to sink.
func New(sink Sink, o Options) Model {
	if o.Theme.Renderer() == nil {
		o.Theme = theme.New(nil)
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MinWidth == 0 {
		o.MinWidth = 80
	}
	if o.MinHeight == 0 {
		o.MinHeight = 24
	}
	if sink == nil {
		sink = func(Event) error { return nil }
	}
	m := Model{
		th: o.Theme, launch: o.Launch, logf: o.Logf, now: o.Now, sink: sink, minW: o.MinWidth, minH: o.MinHeight,
		toasts: toast.WithClock(toast.New(3), o.Now),
		ui:     ui{lists: map[string]list.List[row]{}, tabs: map[string]int{}, inputs: map[string]inputState{}, splits: map[string]split.Model{}},
		km:     keys.New(builtins()...),
	}
	return m
}

// Init does nothing: the renderer draws only when a message or input
// arrives, so an idle surface costs no frames.
func (m Model) Init() tea.Cmd { return nil }

// Update arms the expiry tick of every toast the handlers added, in one
// place, so handlers call toast.Add and never thread commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.toasts
	m, cmd := m.update(msg)
	return m, tea.Batch(cmd, toast.Ticks(before, m.toasts))
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m.relayout(), nil
	case LineMsg:
		return m.ingest(msg), nil
	case EndMsg:
		text, lvl := "agent stream ended", toast.Info
		if msg.Err != nil {
			text, lvl = "agent stream failed: "+msg.Err.Error(), toast.Error
		}
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "stream", Text: text, Level: lvl})
	case toast.Expired:
		m.toasts = toast.Update(m.toasts, msg)
	case link.OpenedMsg:
		lvl := toast.Info
		if msg.Err != nil {
			lvl = toast.Warn
		}
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "link", Text: msg.Note(), Level: lvl})
	case selection.CopiedMsg:
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "copy", Text: msg.Note()})
	case selection.AutoScrollMsg:
		_, cmd := m.sel.Update(msg, m.selPanes, m.plainFrame, m.now())
		return m, cmd
	case tea.MouseMsg:
		return m.mouse(msg)
	case tea.KeyMsg:
		m.toasts = toast.OnKey(m.toasts)
		m.clicks = mouse.Clicks{} // a key between two clicks breaks a double click
		m.sel.Clear()
		return m.key(msg)
	}
	return m, nil
}

// ingest applies one line. A line that fails leaves the last good tree on
// screen: apply returns the old state on error, and the error is toasted,
// logged and reported to the agent as VALIDATION_FAILED.
func (m Model) ingest(line []byte) Model {
	msgs, errs := decodeLine(line)
	for _, msg := range msgs {
		next, err := m.st.apply(msg)
		if err != nil {
			var pe *ProtocolError
			if errors.As(err, &pe) {
				pe.Path = msg.at + pe.Path
			}
			errs = append(errs, err)
			continue
		}
		m.st = next
	}
	for _, err := range errs {
		m.logf("surface: rejected message: %v (line %.200q)", err, line)
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "protocol", Text: "bad message: " + err.Error(), Level: toast.Error})
		m = m.emit(errorEvent(err))
	}
	return m.relayout()
}

func (m Model) emit(e Event) Model {
	if err := m.sink(e); err != nil {
		m.logf("surface: emit: %v", err)
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "emit", Text: "event not sent: " + err.Error(), Level: toast.Error})
	}
	return m
}

// relayout re-runs the layout with state writes allowed: list items are
// re-read from the data model (the cursor follows its item id), list
// selections are written back to their bound paths, and the action table is
// rebuilt from the Buttons on screen. Twice, because a written selection
// can change what a selectionOf pane draws.
func (m Model) relayout() Model {
	sf, ok := m.st.active()
	if !ok || m.w <= 0 || m.h <= 0 {
		m.placed, m.links, m.actions = nil, nil, nil
		m.km = keys.New(builtins()...)
		return m
	}
	for pass := 0; pass < 2; pass++ {
		d := newDrawer(sf, m.th, m.ui, m.logf, true)
		d.draw("root", "", m.body())
		m.ui, m.placed, m.links, m.actions = d.ui, d.placed, d.links, d.actions
		var changed bool
		if m, changed = m.writeSelections(); !changed {
			break
		}
		sf, _ = m.st.active()
	}
	m = m.ensureFocus()
	res := bindActions(m.actions)
	m.km = res.km
	m.ui.bound = res.bound
	for id, why := range res.problems {
		if m.reported[id] == why {
			continue
		}
		m.reported = maps.Clone(m.reported)
		if m.reported == nil {
			m.reported = map[string]string{}
		}
		m.reported[id] = why
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "keys." + id, Text: "action " + id + ": " + why, Level: toast.Warn})
	}
	return m
}

// writeSelections is the List side of two-way binding: the selected item's
// id goes to the List's `selected` path, so an action context can name it.
func (m Model) writeSelections() (Model, bool) {
	sf, _ := m.st.active()
	changed := false
	for _, p := range m.placed {
		if p.kind != "list" {
			continue
		}
		props, err := decode[listProps](sf.Components[p.id])
		if err != nil || props.Selected == nil || props.Selected.Path == "" {
			continue
		}
		id := m.ui.lists[p.key].SelectedID()
		path := join(p.scope, props.Selected.Path)
		if cur, _ := lookup(sf.Data, path); cur == id || (cur == nil && id == "") {
			continue
		}
		st, err := m.st.setData(sf.ID, path, id)
		if err != nil {
			m.toasts = toast.Add(m.toasts, toast.Toast{Source: "bind", Text: "List " + p.id + ": " + err.Error(), Level: toast.Warn})
			continue
		}
		m.st, changed = st, true
		sf, _ = m.st.active()
	}
	return m, changed
}

func (m Model) focusables() []placed {
	var out []placed
	for _, p := range m.placed {
		switch p.kind {
		case "list", "tabs", "input":
			out = append(out, p)
		}
	}
	return out
}

func (m Model) ensureFocus() Model {
	fs := m.focusables()
	for _, p := range fs {
		if p.key == m.ui.focus {
			return m
		}
	}
	m.ui.focus = ""
	m.editing = false
	for _, p := range fs {
		if p.kind != "input" { // never start in a text field: q must quit
			m.ui.focus = p.key
			break
		}
	}
	return m
}

func (m Model) focused() (placed, bool) {
	for _, p := range m.placed {
		if p.key == m.ui.focus {
			return p, true
		}
	}
	return placed{}, false
}

// target is the focused component of kind, else the first one on screen,
// so j/k move the only list without the user tabbing to it first.
func (m Model) target(kind string) (placed, bool) {
	if p, ok := m.focused(); ok && p.kind == kind {
		return p, true
	}
	for _, p := range m.placed {
		if p.kind == kind {
			return p, true
		}
	}
	return placed{}, false
}

// fire runs an action the user confirmed (or that needed no confirmation).
func (m Model) fire(a agentAction) (Model, tea.Cmd) {
	sf, ok := m.st.active()
	if !ok {
		return m, nil
	}
	switch {
	case a.spec.Event != nil:
		ctx, err := resolveContext(sf.Data, a.spec.Event.Context, a.scope)
		if err != nil {
			m.toasts = toast.Add(m.toasts, toast.Toast{Source: "action", Text: a.label + ": " + err.Error(), Level: toast.Error})
			return m, nil
		}
		m = m.emit(actionEvent(Action{Name: a.spec.Event.Name, SurfaceID: sf.ID, SourceComponentID: a.id, Timestamp: timestamp(m.now()), Context: ctx}))
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "action", Text: "sent " + a.spec.Event.Name, Level: toast.Success})
		return m, nil
	case a.spec.FunctionCall != nil && a.spec.FunctionCall.Call == "openUrl":
		raw, err := json.Marshal(a.spec.FunctionCall.Args["url"])
		if err != nil {
			return m, nil
		}
		u, err := text(sf.Data, raw, a.scope)
		if err != nil || !link.Valid(u) {
			// Only validated http(s) links leave the renderer.
			m.toasts = toast.Add(m.toasts, toast.Toast{Source: "link", Text: fmt.Sprintf("refused to open %q", u), Level: toast.Warn})
			return m, nil
		}
		return m, link.OpenWith(m.launch, u)
	case a.spec.FunctionCall != nil:
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "action", Text: "function " + a.spec.FunctionCall.Call + " is not supported here", Level: toast.Warn})
		return m, nil
	}
	m.toasts = toast.Add(m.toasts, toast.Toast{Source: "action", Text: a.label + " has no action", Level: toast.Warn})
	return m, nil
}

// request runs an agent action, through the confirm overlay when the agent
// asked for one and always when it is dangerous: an agent cannot opt a
// destructive action out of confirmation.
func (m Model) request(id string) (Model, tea.Cmd) {
	for _, a := range m.actions {
		if a.id != id {
			continue
		}
		if a.danger || a.confirm {
			m.pending, m.confirm = a, true
			m.toasts = toast.Hold(m.toasts)
			return m, nil
		}
		return m.fire(a)
	}
	return m, nil
}
