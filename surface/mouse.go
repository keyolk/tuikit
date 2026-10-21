package surface

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/keyolk/tuikit/input"
	"github.com/keyolk/tuikit/mouse"
)

func inputStyle(m Model) input.Style { return input.Style{Text: m.th.Text, Placeholder: m.th.Subtle} }

// mouse reaches the same commands as the keyboard. Order matters: a split
// divider first (its press is never a pane's), then the drag selection (a
// press that does not move is never consumed, so clicks still land), then
// clicks and the wheel on lists, tab headers, buttons and fields.
func (m Model) mouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.tooSmall() || m.overlayOpen() {
		return m, nil
	}
	for _, p := range m.placed {
		if p.kind != "split" {
			continue
		}
		sm, res := m.ui.splits[p.key].UpdateAt(msg, p.rect, m.now())
		if res.Consumed || res.Changed || sm.Dragging() != m.ui.splits[p.key].Dragging() {
			m.ui = m.ui.clone()
			m.ui.splits[p.key] = sm
			return m.relayout(), nil
		}
	}
	res, cmd := m.sel.Update(msg, m.selPanes, m.plainFrame, m.now())
	if res.Consumed {
		return m, cmd
	}
	if d := mouse.Delta(msg); d != 0 {
		for _, p := range m.placed {
			if p.kind == "list" && p.rect.Contains(msg.X, msg.Y) {
				m.ui = m.ui.clone()
				m.ui.lists[p.key] = m.ui.lists[p.key].Move(d)
				return m.relayout(), cmd
			}
		}
		return m, cmd
	}
	if !mouse.IsLeftPress(msg) {
		return m, cmd
	}
	// The innermost hit wins: placed is in drawing order, children after
	// their containers, so scan from the end.
	for i := len(m.placed) - 1; i >= 0; i-- {
		p := m.placed[i]
		switch p.kind {
		case "list":
			c := mouse.RowHit(p.rect, p.start, m.ui.lists[p.key].Len(), msg)
			if !c.HasItem {
				continue
			}
			m.ui = m.ui.clone()
			m.ui.lists[p.key] = m.ui.lists[p.key].SelectIndex(c.Index)
			m.ui.focus, m.editing = p.key, false
			return m.relayout(), cmd
		case "tabs":
			for j, r := range p.tabs {
				if r.Contains(msg.X, msg.Y) {
					m.ui = m.ui.clone()
					m.ui.focus, m.editing = p.key, false
					return m.setTab(p.key, j), cmd
				}
			}
		case "button":
			if p.rect.Contains(msg.X, msg.Y) {
				nm, c2 := m.request(p.id)
				return nm, tea.Batch(cmd, c2)
			}
		case "input":
			if p.rect.Contains(msg.X, msg.Y) {
				m.ui = m.ui.clone()
				m.ui.focus, m.editing = p.key, true
				return m, cmd
			}
		}
	}
	return m, cmd
}
