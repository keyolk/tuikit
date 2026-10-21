package surface

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keyolk/tuikit/input"
	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/link"
	"github.com/keyolk/tuikit/split"
	"github.com/keyolk/tuikit/toast"
)

// active is the context stack for the current mode, innermost first.
func (m Model) active() []keys.Ctx {
	switch {
	case m.confirm:
		return stackConfirm
	case m.help:
		return stackHelp
	case m.palette:
		return stackPalette
	case m.editing:
		return stackInput
	}
	return stackMain
}

func (m Model) keyOpts() keys.Opts {
	return keys.Opts{Visible: m.offered}
}

// offered narrows the footer to what applies to this screen; every key
// still works (the palette and help list them all).
func (m Model) offered(action string) bool {
	has := func(kind string) bool { _, ok := m.target(kind); return ok }
	switch action {
	case "down", "up":
		return has("list")
	case "tab.next", "tab.prev":
		return has("tabs")
	case split.ActionGrow, split.ActionShrink, split.ActionReset:
		return has("split")
	case link.Action:
		return len(m.links) > 0
	case "focus.next":
		return len(m.focusables()) > 1
	}
	return true
}

func (m Model) key(k tea.KeyMsg) (Model, tea.Cmd) {
	if (m.editing || m.palette) && !m.tooSmall() {
		return m.typeText(k)
	}
	b, ok := m.km.Resolve(m.active(), k, m.keyOpts())
	if !ok {
		return m, nil
	}
	if m.tooSmall() && b.Action != "quit" {
		// Nothing is on screen to act on, not even a confirm overlay.
		return m, nil
	}
	return m.run(b)
}

// typeText routes keys while a text field owns the keyboard: the mode's own
// keys first, raw (a jamo is text, not a shortcut), then the field.
func (m Model) typeText(k tea.KeyMsg) (Model, tea.Cmd) {
	if b, ok := m.km.ResolveRaw(m.active(), k, m.keyOpts()); ok {
		return m.run(b)
	}
	if m.palette {
		if f, ok := m.query.Update(k); ok {
			m.query, m.pick = f, 0
		}
		return m, nil
	}
	p, ok := m.focused()
	if !ok || p.kind != "input" {
		m.editing = false
		return m, nil
	}
	st := m.ui.inputs[p.key]
	f, used := st.field.Update(k)
	if !used {
		return m, nil
	}
	sf, _ := m.st.active()
	next, err := m.st.setData(sf.ID, st.path, f.Value())
	if err != nil {
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "bind", Text: "input " + p.id + ": " + err.Error(), Level: toast.Warn})
		return m, nil
	}
	m.st = next
	m.ui = m.ui.clone()
	m.ui.inputs[p.key] = inputState{field: f, path: st.path}
	return m.relayout(), nil
}

func (m Model) run(b keys.Binding) (Model, tea.Cmd) {
	if id, ok := strings.CutPrefix(b.Action, agentPrefix); ok {
		return m.request(id)
	}
	switch b.Action {
	case "quit":
		return m, tea.Quit
	case "help":
		m.help, m.helpOff = true, 0
		m.toasts = toast.Hold(m.toasts) // toasts wait while a dialog is up
	case "help.close":
		m.help = false
		m.toasts, _ = toast.Release(m.toasts) // Update's toast.Ticks re-arms them
	case "help.down":
		m.helpOff = m.clampHelp(m.helpOff + 1)
	case "help.up":
		m.helpOff = m.clampHelp(m.helpOff - 1)
	case "palette":
		m.palette, m.query, m.pick = true, input.Field{}, 0
		m.toasts = toast.Hold(m.toasts)
	case "palette.close":
		m.palette = false
		m.toasts, _ = toast.Release(m.toasts)
	case "palette.down", "palette.up":
		n := len(m.paletteEntries())
		if n > 0 {
			d := 1
			if b.Action == "palette.up" {
				d = -1
			}
			m.pick = (m.pick + d + n) % n
		}
	case "palette.run":
		es := m.paletteEntries()
		m.palette = false
		m.toasts, _ = toast.Release(m.toasts)
		if m.pick < len(es) {
			return m.run(keys.Binding{Action: es[m.pick].Action})
		}
	case "confirm.yes":
		m.confirm = false
		m.toasts, _ = toast.Release(m.toasts)
		return m.fire(m.pending)
	case "confirm.no":
		m.confirm = false
		m.toasts, _ = toast.Release(m.toasts)
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "action", Text: m.pending.label + " cancelled"})
	case "down", "up", "top", "bottom":
		return m.moveList(b.Action), nil
	case "focus.next", "focus.prev":
		return m.cycleFocus(b.Action == "focus.next"), nil
	case "tab.next", "tab.prev":
		return m.cycleTab(b.Action == "tab.next"), nil
	case "activate":
		if p, ok := m.focused(); ok && p.kind == "input" {
			m.editing = true
		}
	case "input.done":
		m.editing = false
	case link.Action:
		return m, m.openFirst()
	case split.ActionGrow, split.ActionShrink, split.ActionReset:
		if p, ok := m.target("split"); ok {
			m.ui = m.ui.clone()
			m.ui.splits[p.key], _ = m.ui.splits[p.key].Handle(b.Action, p.rect)
			return m.relayout(), nil
		}
	}
	return m, nil
}

func (m Model) moveList(action string) Model {
	p, ok := m.target("list")
	if !ok {
		return m
	}
	l := m.ui.lists[p.key]
	switch action {
	case "down":
		l = l.Move(1)
	case "up":
		l = l.Move(-1)
	case "top":
		l = l.Top()
	case "bottom":
		l = l.Bottom()
	}
	m.ui = m.ui.clone()
	m.ui.lists[p.key] = l
	m.ui.focus = p.key
	return m.relayout()
}

func (m Model) cycleFocus(forward bool) Model {
	fs := m.focusables()
	if len(fs) == 0 {
		return m
	}
	i := 0
	for j, p := range fs {
		if p.key == m.ui.focus {
			i = j
		}
	}
	if forward {
		i = (i + 1) % len(fs)
	} else {
		i = (i - 1 + len(fs)) % len(fs)
	}
	m.ui = m.ui.clone()
	m.ui.focus = fs[i].key
	// Tabbing onto a field means "type here"; leaving it hands keys back.
	m.editing = fs[i].kind == "input"
	return m.relayout()
}

func (m Model) cycleTab(forward bool) Model {
	p, ok := m.target("tabs")
	if !ok || len(p.tabs) == 0 {
		return m
	}
	n := len(p.tabs)
	cur := m.ui.tabs[p.key]
	if forward {
		cur = (cur + 1) % n
	} else {
		cur = (cur - 1 + n) % n
	}
	return m.setTab(p.key, cur)
}

func (m Model) setTab(key string, i int) Model {
	m.ui = m.ui.clone()
	m.ui.tabs[key] = i
	return m.relayout()
}

// paletteEntries is the palette: the table's own entries for the main
// context plus agent actions whose key was refused, which the table cannot
// list because they have no key.
func (m Model) paletteEntries() []keys.Entry {
	es := m.km.Entries(stackMain, m.keyOpts())
	for _, a := range m.actions {
		if m.ui.bound[a.id] == "" {
			es = append(es, keys.Entry{Action: a.action(), Help: a.label, Group: "Actions"})
		}
	}
	return keys.Filter(es, m.query.Value())
}

// openFirst is `o`: the first valid link on screen, through the model's
// launcher. Like link.OpenFirst, but OpenFirst only takes the package-wide
// Launcher.
func (m Model) openFirst() tea.Cmd {
	for _, u := range m.links {
		if link.Valid(u) {
			return link.OpenWith(m.launch, u)
		}
	}
	return func() tea.Msg { return link.OpenedMsg{Err: link.ErrNoLink} }
}
