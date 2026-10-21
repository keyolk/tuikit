package surface

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/keyolk/tuikit/input"
	"github.com/keyolk/tuikit/link"
	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/render"
)

// inputState is an Input's field: its text and cursor. The text is mirrored
// into the data model at path on every keystroke (two-way binding).
type inputState struct {
	field input.Field
	path  string
}

func (d *drawer) style(t tone) lipgloss.Style {
	switch t {
	case toneMuted:
		return d.th.Muted
	case toneAccent:
		return d.th.Title
	case toneOK:
		return d.th.Success
	case toneWarn:
		return d.th.Warn
	case toneErr:
		return d.th.Error
	}
	return d.th.Text
}

// glyph leads every status so it reads with color stripped (NO_COLOR).
func (d *drawer) glyph(t tone) string {
	g := d.th.Glyphs
	switch t {
	case toneOK:
		return g.Check
	case toneWarn:
		return g.Warn
	case toneErr:
		return g.Cross
	}
	return g.Bullet
}

func (d *drawer) leaf(c Component, scope string, r mouse.Rect) (string, error) {
	data := d.sf.Data
	switch c.Type {
	case "Text":
		p, err := decode[textProps](c)
		if err != nil {
			return "", err
		}
		s, err := text(data, p.Text, scope)
		if err != nil {
			return "", err
		}
		t := toneOf(p.Style)
		if p.Style == "" {
			t = toneOf(p.Variant)
		}
		if p.Variant == "caption" {
			t = toneMuted
		}
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			lines[i] = d.style(t).Render(render.Truncate(l, r.W))
		}
		return block(strings.Join(lines, "\n"), r.W, r.H), nil
	case "Detail":
		return d.detail(c, scope, r)
	case "Input", "TextField":
		return d.inputView(c, scope, r)
	case "Button":
		return d.button(c, scope, r)
	case "Badge", "Status":
		p, err := decode[badgeProps](c)
		if err != nil {
			return "", err
		}
		s, err := text(data, p.Text, scope)
		if err != nil {
			return "", err
		}
		st, err := text(data, p.Status, scope)
		if err != nil {
			return "", err
		}
		if st == "" {
			st = s
		}
		t := toneOf(st)
		return block(d.style(t).Render(render.Truncate(d.glyph(t)+" "+s, r.W)), r.W, r.H), nil
	case "Link":
		p, err := decode[linkProps](c)
		if err != nil {
			return "", err
		}
		u, err := text(data, p.URL, scope)
		if err != nil {
			return "", err
		}
		label, err := text(data, p.Text, scope)
		if err != nil {
			return "", err
		}
		if label == "" {
			label = u
		}
		label = render.Truncate(label, r.W)
		if !link.Valid(u) {
			// Not http(s): drawn as plain words, never an OSC 8 target and
			// never offered to `o`.
			return block(d.th.Muted.Render(label), r.W, r.H), nil
		}
		if d.quiet == 0 {
			d.links = append(d.links, u)
		}
		return block(link.Wrap(u, d.th.Info.Underline(true).Render(label)), r.W, r.H), nil
	case "Progress":
		p, err := decode[progressProps](c)
		if err != nil {
			return "", err
		}
		v, ok, err := number(data, p.Value, scope)
		if err != nil {
			return "", err
		}
		label, err := text(data, p.Label, scope)
		if err != nil {
			return "", err
		}
		pct := "  ?%"
		frac := 0.0
		if ok && !math.IsNaN(v) {
			frac = max(0, min(1, v))
			pct = fmt.Sprintf("%3d%%", int(frac*100))
		}
		if label != "" {
			label = " " + label
		}
		barW := max(0, min(30, r.W-render.Width(pct)-render.Width(label)-1))
		return block(render.Truncate(d.th.Accent.Render(d.th.Glyphs.Bar(frac, barW))+" "+pct+d.th.Muted.Render(label), r.W), r.W, r.H), nil
	case "Divider":
		rule := "─"
		if d.th.Glyphs.ASCII || render.CJKLocale() {
			rule = "-"
		}
		return block(d.th.Subtle.Render(strings.Repeat(rule, r.W)), r.W, r.H), nil
	}
	// Unknown to this catalog: say so in the pane and keep drawing the rest.
	return d.note(r, d.th.Warn, fmt.Sprintf("%s unsupported component %s (%s)", d.th.Glyphs.Warn, c.Type, c.ID)), nil
}

func (d *drawer) detail(c Component, scope string, r mouse.Rect) (string, error) {
	p, err := decode[detailProps](c)
	if err != nil {
		return "", err
	}
	lw := 0
	for _, f := range p.Fields {
		lw = max(lw, render.Width(f.Label))
	}
	lw = min(lw, r.W/3)
	var lines []string
	for _, f := range p.Fields {
		v, err := text(d.sf.Data, f.Value, scope)
		if err != nil {
			v = d.th.Glyphs.Cross + " " + err.Error()
		}
		v = strings.ReplaceAll(v, "\n", " ")
		lines = append(lines, d.th.Muted.Render(render.Pad(f.Label, lw))+"  "+render.Truncate(v, max(0, r.W-lw-2)))
	}
	return block(strings.Join(lines, "\n"), r.W, r.H), nil
}

// bindingPath reads a two-way {"path": …} property.
func bindingPath(raw json.RawMessage, scope string) (string, bool) {
	var b struct {
		Path string `json:"path"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &b) != nil || b.Path == "" {
		return "", false
	}
	return join(scope, b.Path), true
}

func (d *drawer) inputView(c Component, scope string, r mouse.Rect) (string, error) {
	p, err := decode[inputProps](c)
	if err != nil {
		return "", err
	}
	key := uiKey(c.ID, scope)
	label, err := text(d.sf.Data, p.Label, scope)
	if err != nil {
		return "", err
	}
	path, bound := bindingPath(p.Value, scope)
	st, ok := d.ui.inputs[key]
	cur, _ := text(d.sf.Data, p.Value, scope) // unresolved data is an empty field
	if d.sync && (!ok || st.field.Value() != cur) {
		// The data model is the source of truth: an agent update replaces
		// what the field shows.
		st = inputState{field: input.New(cur), path: path}
		if bound {
			d.ui.inputs[key] = st
		}
	}
	if !ok && !d.sync {
		st.field = input.New(cur)
	}
	d.place(placed{key: key, id: c.ID, kind: "input", scope: scope, rect: r})
	head := ""
	if label != "" {
		head = d.th.Muted.Render(render.Truncate(label, r.W/3)) + " "
	}
	fw := max(1, r.W-render.Width(head))
	field := st.field.View(fw, d.ui.focus == key, input.Style{Text: d.th.Text, Placeholder: d.th.Subtle})
	if st.field.Len() == 0 && p.Placeholder != "" && d.ui.focus != key {
		field = d.th.Subtle.Render(render.Pad(p.Placeholder, fw))
	}
	if !bound {
		field = d.th.Warn.Render(render.Truncate(d.th.Glyphs.Warn+" value must be a {\"path\": …} binding", fw))
	}
	return block(head+field, r.W, r.H), nil
}

func (d *drawer) button(c Component, scope string, r mouse.Rect) (string, error) {
	p, err := decode[buttonProps](c)
	if err != nil {
		return "", err
	}
	label, err := text(d.sf.Data, p.Label, scope)
	if err != nil {
		return "", err
	}
	if label == "" && p.Child != "" {
		if ch, ok := d.sf.Components[p.Child]; ok {
			if tp, err := decode[textProps](ch); err == nil {
				label, _ = text(d.sf.Data, tp.Text, scope) // a bad child label falls back to the id
			}
		}
	}
	if label == "" {
		label = c.ID
	}
	a := agentAction{id: c.ID, label: label, key: p.Key, confirm: p.Confirm, danger: p.Danger, spec: p.Action, scope: scope}
	if d.quiet == 0 && !d.seen[c.ID] {
		d.seen[c.ID] = true
		d.actions = append(d.actions, a)
	}
	d.place(placed{key: uiKey(c.ID, scope), id: c.ID, kind: "button", scope: scope, rect: r})
	k := d.ui.bound[c.ID]
	shown := "[:] " + label // palette only
	if k != "" {
		shown = "[" + d.th.KeyNames().Name(k) + "] " + label
	}
	st := d.th.Accent
	if p.Danger {
		st = d.th.Error
	}
	return block(st.Render(render.Truncate(shown, r.W)), r.W, r.H), nil
}

func (d *drawer) table(c Component, scope string, r mouse.Rect) (string, error) {
	p, err := decode[tableProps](c)
	if err != nil {
		return "", err
	}
	if len(p.Columns) == 0 {
		return "", fmt.Errorf("Table %q has no columns", c.ID)
	}
	rows := d.children(childList{Template: c.ID, Path: p.Rows.Path}, scope)
	ws := share(max(0, r.W-2*(len(p.Columns)-1)), make([]float64, len(p.Columns)))
	cell := func(s string, i int) string { return render.Pad(s, ws[i]) }
	var head []string
	for i, col := range p.Columns {
		head = append(head, d.th.Bold.Render(cell(col.Title, i)))
	}
	lines := []string{strings.Join(head, "  ")}
	for _, rw := range rows {
		var cells []string
		for i, col := range p.Columns {
			v, err := text(d.sf.Data, col.Value, rw.scope)
			if err != nil {
				v = d.th.Glyphs.Cross + " " + err.Error()
			}
			v = strings.ReplaceAll(v, "\n", " ")
			if col.Status {
				t := toneOf(v)
				cells = append(cells, d.style(t).Render(cell(d.glyph(t)+" "+v, i)))
				continue
			}
			cells = append(cells, cell(v, i))
		}
		lines = append(lines, strings.Join(cells, "  "))
	}
	if len(rows) == 0 {
		lines = append(lines, d.th.Muted.Render("(no rows)"))
	}
	if len(lines) > r.H && r.H > 1 {
		hidden := len(lines) - r.H + 1
		lines = append(lines[:r.H-1], d.th.Subtle.Render(render.MoreLine(0, hidden)))
	}
	return block(strings.Join(lines, "\n"), r.W, r.H), nil
}
