package surface

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/selection"
	"github.com/keyolk/tuikit/toast"
)

// body is the rect the surface's root is drawn in: everything between the
// title row and the footer row. Toasts float over its bottom-right corner
// instead of taking rows, so a toast expiring never moves a list.
func (m Model) body() mouse.Rect {
	return mouse.Rect{X: 0, Y: 1, W: m.w, H: max(0, m.h-2)}
}

func (m Model) tooSmall() bool { return m.w < m.minW || m.h < m.minH }

// View draws the frame with the selection highlight and the open overlay.
func (m Model) View() string {
	if m.w <= 0 || m.h <= 0 {
		return ""
	}
	if m.tooSmall() {
		// "x", not "×": U+00D7 is East Asian Ambiguous, two cells under CJK.
		msg := fmt.Sprintf("terminal too small: need %dx%d, have %dx%d", m.minW, m.minH, m.w, m.h)
		return render.FitLines(m.th.Warn.Render(render.Truncate(msg, m.w)), m.w, m.h)
	}
	frame := m.plainFrame()
	if m.sel.Active() {
		frame = m.sel.Render(frame, m.selPanes(), selection.DefaultStyle())
	}
	if ts := toast.View(m.toasts, min(m.w, 60), m.th.ToastStyles()); ts != "" && !m.overlayOpen() {
		tw, th := 0, strings.Count(ts, "\n")+1
		for _, l := range strings.Split(ts, "\n") {
			tw = max(tw, render.Width(l))
		}
		frame = render.Overlay(frame, ts, m.w-tw, max(1, m.h-1-th))
	}
	switch {
	case m.confirm:
		return render.Center(render.Dim(frame), m.confirmBox(), m.w, m.h)
	case m.palette:
		return render.Center(render.Dim(frame), m.paletteBox(), m.w, m.h)
	case m.help:
		return render.Center(render.Dim(frame), m.helpBox(), m.w, m.h)
	}
	return frame
}

func (m Model) overlayOpen() bool { return m.confirm || m.palette || m.help }

// plainFrame is the frame before the selection highlight and overlays: what
// selection copies from.
func (m Model) plainFrame() string {
	title := m.th.Muted.Render("waiting for a surface…")
	body := blank(m.w, m.body().H)
	if sf, ok := m.st.active(); ok {
		title = m.th.Title.Render(sf.Title)
		if n := len(m.st.order); n > 1 {
			title += m.th.Muted.Render(fmt.Sprintf("  (%d surfaces, newest shown)", n))
		}
		d := newDrawer(sf, m.th, m.ui, m.logf, false)
		body = d.draw("root", "", m.body())
		if _, ok := sf.Components["root"]; !ok {
			body = block(m.th.Muted.Render("waiting for the root component…"), m.w, m.body().H)
		}
	}
	footer := m.km.Footer(m.active(), m.w, m.keyOpts(), m.th.FooterStyle())
	return strings.Join([]string{block(title, m.w, 1), body, block(footer, m.w, 1)}, "\n")
}

// selPanes are the regions a drag selection may live in: each Split pane,
// else the whole body.
func (m Model) selPanes() []selection.Pane {
	var out []selection.Pane
	for _, p := range m.placed {
		if p.kind == "pane" && p.rect.W > 0 {
			out = append(out, selection.Pane{ID: p.key, Rect: p.rect, Kind: selection.Text})
		}
	}
	if len(out) == 0 {
		out = append(out, selection.Pane{ID: "body", Rect: m.body(), Kind: selection.Text})
	}
	return out
}

func (m Model) boxSize(w, h int) (int, int) { return min(w, m.w-4), min(h, m.h-4) }

func (m Model) confirmBox() string {
	a := m.pending
	w, h := m.boxSize(56, 7)
	cw, _ := render.BoxContent(w, h, m.th.BoxFocus)
	title, verb := "confirm", "Run"
	st := m.th.Accent
	if a.danger {
		title, verb, st = "danger", "Really run", m.th.Error
	}
	lines := []string{
		st.Render(render.Truncate(fmt.Sprintf("%s %q?", verb, a.label), cw)),
		"",
		m.th.Muted.Render(render.Truncate("The agent acts on it; there is no undo here.", cw)),
		"",
		render.Truncate(m.km.Footer(stackConfirm, cw, keys.Opts{}, m.th.FooterStyle()), cw),
	}
	return render.Box(title, strings.Join(lines, "\n"), w, h, m.th.BoxFocus)
}

func (m Model) paletteBox() string {
	w, h := m.boxSize(60, 14)
	cw, ch := render.BoxContent(w, h, m.th.BoxFocus)
	lines := []string{":" + m.query.View(cw-1, true, inputStyle(m))}
	es := m.paletteEntries()
	start, end, _ := render.Window(len(es), ch-1, m.pick, 0)
	for i := start; i < end; i++ {
		e := es[i]
		k := "palette"
		if len(e.Keys) > 0 {
			k = m.th.KeyNames().Name(e.Keys[0])
		}
		line := render.Pad(e.Help, cw-10) + m.th.Muted.Render(render.PadLeft(render.Truncate(k, 9), 10))
		if i == m.pick {
			line = m.th.Selected.Render(render.Pad(e.Help, cw-10) + render.PadLeft(render.Truncate(k, 9), 10))
		}
		lines = append(lines, line)
	}
	if len(es) == 0 {
		lines = append(lines, m.th.Muted.Render("no match"))
	}
	return render.Box("actions", strings.Join(lines, "\n"), w, h, m.th.BoxFocus)
}

// helpSections is the help page: every table entry for the main context,
// plus the agent actions that could only go to the palette and why.
func (m Model) helpSections() []keys.Section {
	secs := m.km.Help(m.keyOpts(), stackMain)
	// The agent's actions are what this surface is for: list them first.
	slices.SortStableFunc(secs, func(a, b keys.Section) int {
		return cmp.Compare(boolInt(a.Title != "Actions"), boolInt(b.Title != "Actions"))
	})
	var notes []string
	for _, a := range m.actions {
		if m.ui.bound[a.id] == "" {
			why := m.reported[a.id]
			if why == "" {
				why = "no key requested"
			}
			notes = append(notes, fmt.Sprintf("%s - palette only (%s)", a.label, why))
		}
	}
	notes = append(notes, "Agent actions never take the renderer's own keys. Dangerous actions always ask first.")
	return append(secs, keys.Section{Title: "Notes", Notes: notes})
}

func (m Model) helpDims() (w, h, cw, ch int) {
	w, h = m.boxSize(64, 20)
	cw, ch = render.BoxContent(w, h, m.th.BoxFocus)
	return w, h, cw, ch
}

func (m Model) helpBox() string {
	w, h, cw, ch := m.helpDims()
	lines, _ := keys.RenderHelp(m.helpSections(), cw, ch, m.helpOff, m.th.HelpStyle())
	return render.Box("keys", strings.Join(lines, "\n"), w, h, m.th.BoxFocus)
}

func (m Model) clampHelp(off int) int {
	_, _, cw, ch := m.helpDims()
	return max(0, min(off, keys.HelpLineCount(m.helpSections(), cw)-ch))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
