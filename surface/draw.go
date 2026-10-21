package surface

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/keyolk/tuikit/list"
	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/split"
	"github.com/keyolk/tuikit/theme"
)

// maxDepth bounds the component tree. The adjacency list lets an agent
// write a cycle (a Column that lists itself); the drawer must stop, not
// recurse until the stack runs out.
const maxDepth = 32

// row is one List item. id is its identity (itemKey's value, else the
// index, else the static child's id); scope is the data pointer the row's
// template resolves relative paths against.
type row struct{ id, comp, scope string }

func (r row) ID() string { return r.id }

// ui is the renderer's own per-component state, keyed by uiKey: what a
// keystroke changes without the agent knowing (cursor, tab, field text).
type ui struct {
	lists  map[string]list.List[row]
	tabs   map[string]int
	inputs map[string]inputState
	splits map[string]split.Model
	focus  string
	bound  map[string]string // action id -> the key that runs it, "" when palette-only
}

func (u ui) clone() ui {
	return ui{lists: maps.Clone(u.lists), tabs: maps.Clone(u.tabs), inputs: maps.Clone(u.inputs), splits: maps.Clone(u.splits), focus: u.focus, bound: maps.Clone(u.bound)}
}

func uiKey(id, scope string) string {
	if scope == "" {
		return id
	}
	return id + "@" + scope
}

// placed is a component the frame drew where the mouse or the keyboard can
// reach it. The layout pass and View produce the same list from the same
// inputs, so Update hit-tests against exactly what is on screen (E9).
type placed struct {
	key, id, kind string
	scope         string
	rect          mouse.Rect
	tabs          []mouse.Rect // header hit boxes, Tabs only
	start         int          // first visible row, List only
}

// drawer renders one surface into a frame. It is built per frame and thrown
// away; in a sync pass it also refreshes list state from the data model,
// which Update keeps (View never writes model state).
type drawer struct {
	sf   Surface
	th   theme.Theme
	ui   ui
	logf func(string, ...any)
	sync bool

	placed  []placed
	links   []string
	actions []agentAction
	seen    map[string]bool // action ids already collected
	quiet   int             // >0 while drawing list rows: they place nothing
	depth   int
	onPath  map[string]bool
}

func newDrawer(sf Surface, th theme.Theme, u ui, logf func(string, ...any), sync bool) *drawer {
	if sync {
		u = u.clone()
	}
	return &drawer{sf: sf, th: th, ui: u, logf: logf, sync: sync, seen: map[string]bool{}, onPath: map[string]bool{}}
}

func (d *drawer) place(p placed) {
	if d.quiet == 0 {
		d.placed = append(d.placed, p)
	}
}

// draw renders component id in r. Every component is isolated: a panic,
// a decode error, an unknown type or a missing id costs only its own rect.
func (d *drawer) draw(id, scope string, r mouse.Rect) string {
	if r.W <= 0 || r.H <= 0 {
		return ""
	}
	c, ok := d.sf.Components[id]
	switch {
	case !ok:
		// The spec allows references to components that have not arrived
		// yet; show that the slot is pending rather than an error.
		return d.note(r, d.th.Muted, "… "+id)
	case d.depth >= maxDepth || d.onPath[id+"@"+scope]:
		return d.note(r, d.th.Warn, d.th.Glyphs.Warn+" cycle at "+id)
	}
	d.depth++
	d.onPath[id+"@"+scope] = true
	defer func() { d.depth--; delete(d.onPath, id+"@"+scope) }()

	return block(render.Safe(id, r.W, r.H, d.logf, func() string {
		if so := commonOf(c).SelectionOf; so != "" {
			l, ok := d.ui.lists[so]
			sel, has := l.Selected()
			if !ok || !has {
				return d.note(r, d.th.Muted, "nothing selected")
			}
			scope = sel.scope
		}
		out, err := d.component(c, scope, r)
		if err != nil {
			d.logf("surface %s: %v", d.sf.ID, err)
			return d.note(r, d.th.Error, d.th.Glyphs.Cross+" "+err.Error())
		}
		return out
	}), r.W, r.H)
}

func (d *drawer) note(r mouse.Rect, st interface{ Render(...string) string }, s string) string {
	return block(st.Render(render.Truncate(s, r.W)), r.W, r.H)
}

func (d *drawer) component(c Component, scope string, r mouse.Rect) (string, error) {
	switch c.Type {
	case "Column", "Row":
		p, err := decode[stackProps](c)
		if err != nil {
			return "", err
		}
		kids := d.children(p.Children, scope)
		if c.Type == "Row" {
			return d.row(kids, r), nil
		}
		return d.column(kids, r), nil
	case "Card":
		p, err := decode[cardProps](c)
		if err != nil {
			return "", err
		}
		cw, ch := render.BoxContent(r.W, r.H, d.th.Box)
		inner := d.draw(p.Child, scope, mouse.Rect{X: r.X + (r.W-cw)/2, Y: r.Y + (r.H-ch)/2, W: cw, H: ch})
		return render.Box("", inner, r.W, r.H, d.th.Box), nil
	case "Split":
		return d.split(c, scope, r)
	case "Tabs":
		return d.tabsView(c, scope, r)
	case "List":
		return d.listView(c, scope, r)
	case "Table":
		return d.table(c, scope, r)
	}
	return d.leaf(c, scope, r)
}

type kid struct{ id, scope string }

// children expands a ChildList: static ids, or one template instance per
// element of the array (or object) at path.
func (d *drawer) children(cl childList, scope string) []kid {
	if cl.Template == "" {
		out := make([]kid, len(cl.IDs))
		for i, id := range cl.IDs {
			out[i] = kid{id, scope}
		}
		return out
	}
	base := join(scope, cl.Path)
	v, _ := lookup(d.sf.Data, base) // missing data: no children yet
	var out []kid
	switch x := v.(type) {
	case []any:
		for i := range x {
			out = append(out, kid{cl.Template, strings.TrimSuffix(base, "/") + "/" + strconv.Itoa(i)})
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			out = append(out, kid{cl.Template, strings.TrimSuffix(base, "/") + "/" + escapeToken(k)})
		}
	}
	return out
}

// natural is how many rows a component wants, and whether it can use more.
// A Column gives fixed children their rows and shares the rest among the
// growing ones, so a title stays one line while a list takes the height.
func (d *drawer) natural(k kid, depth int) (h int, grow bool) {
	c, ok := d.sf.Components[k.id]
	if !ok || depth > maxDepth {
		return 1, false
	}
	switch c.Type {
	case "List", "Table", "Tabs", "Split", "Card":
		return 3, true
	case "Detail":
		p, err := decode[detailProps](c)
		if err != nil {
			return 1, false
		}
		return max(1, len(p.Fields)), false
	case "Text":
		p, err := decode[textProps](c)
		if err != nil {
			return 1, false
		}
		s, _ := text(d.sf.Data, p.Text, k.scope) // an error draws as one line
		return max(1, strings.Count(s, "\n")+1), false
	case "Column", "Row":
		p, err := decode[stackProps](c)
		if err != nil {
			return 1, false
		}
		for _, ch := range d.children(p.Children, k.scope) {
			kh, kg := d.natural(ch, depth+1)
			grow = grow || kg
			if c.Type == "Row" {
				h = max(h, kh)
			} else {
				h += kh
			}
		}
		return max(h, 1), grow
	}
	return 1, false
}

func (d *drawer) column(kids []kid, r mouse.Rect) string {
	hs := make([]int, len(kids))
	var growers []int
	var weights []float64
	used := 0
	for i, k := range kids {
		h, g := d.natural(k, d.depth)
		if g {
			growers = append(growers, i)
			weights = append(weights, commonOf(d.sf.Components[k.id]).Weight)
			continue
		}
		hs[i] = min(h, max(0, r.H-used))
		used += hs[i]
	}
	for j, n := range share(max(0, r.H-used), weights) {
		hs[growers[j]] = n
	}
	var parts []string
	y := r.Y
	for i, k := range kids {
		if hs[i] == 0 {
			continue
		}
		parts = append(parts, d.draw(k.id, k.scope, mouse.Rect{X: r.X, Y: y, W: r.W, H: hs[i]}))
		y += hs[i]
	}
	if y < r.Y+r.H {
		parts = append(parts, blank(r.W, r.Y+r.H-y))
	}
	return strings.Join(parts, "\n")
}

func (d *drawer) row(kids []kid, r mouse.Rect) string {
	if len(kids) == 0 {
		return blank(r.W, r.H)
	}
	gap := 1
	weights := make([]float64, len(kids))
	for i, k := range kids {
		weights[i] = commonOf(d.sf.Components[k.id]).Weight
	}
	ws := share(max(0, r.W-gap*(len(kids)-1)), weights)
	var blocks []string
	x := r.X
	for i, k := range kids {
		if i > 0 {
			blocks = append(blocks, blank(gap, r.H))
			x += gap
		}
		if ws[i] > 0 {
			blocks = append(blocks, d.draw(k.id, k.scope, mouse.Rect{X: x, Y: r.Y, W: ws[i], H: r.H}))
		}
		x += ws[i]
	}
	return beside(blocks...)
}

// minPane is the narrowest a Split pane gets while both fit.
const minPane = 10

func (d *drawer) split(c Component, scope string, r mouse.Rect) (string, error) {
	p, err := decode[splitProps](c)
	if err != nil {
		return "", err
	}
	key := uiKey(c.ID, scope)
	sm, ok := d.ui.splits[key]
	if !ok {
		// The agent's ratio is the default; the user's resizing (< > =,
		// drag, double click) is renderer state and survives agent updates.
		sm = split.Model{Divider: 1, MinA: minPane, MinB: minPane, Default: p.Ratio}
		if d.sync {
			d.ui.splits[key] = sm
		}
	}
	a, div, b := sm.Layout(r)
	d.place(placed{key: key, id: c.ID, kind: "split", scope: scope, rect: r})
	d.place(placed{key: key + "/a", id: c.ID, kind: "pane", scope: scope, rect: a})
	d.place(placed{key: key + "/b", id: c.ID, kind: "pane", scope: scope, rect: b})
	return sm.Join(r, d.draw(p.Left, scope, a), sm.DrawDivider(div, d.th.Subtle), d.draw(p.Right, scope, b)), nil
}

func (d *drawer) tabsView(c Component, scope string, r mouse.Rect) (string, error) {
	p, err := decode[tabsProps](c)
	if err != nil {
		return "", err
	}
	if len(p.Tabs) == 0 {
		return "", fmt.Errorf("Tabs %q has no tabs", c.ID)
	}
	key := uiKey(c.ID, scope)
	cur := max(0, min(d.ui.tabs[key], len(p.Tabs)-1))
	focused := d.ui.focus == key
	var head strings.Builder
	var hits []mouse.Rect
	x := r.X
	for i, t := range p.Tabs {
		title, err := text(d.sf.Data, t.Title, scope)
		if err != nil {
			title = t.Child
		}
		label := " " + title + " "
		st := d.th.Muted
		if i == cur {
			st = d.th.Title.Underline(true)
			if focused {
				st = d.th.Selected
			}
		}
		w := render.Width(label)
		hits = append(hits, mouse.Rect{X: x, Y: r.Y, W: w, H: 1})
		head.WriteString(st.Render(label))
		x += w
	}
	d.place(placed{key: key, id: c.ID, kind: "tabs", scope: scope, rect: mouse.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, tabs: hits})
	body := blank(r.W, r.H-1)
	if r.H > 1 {
		body = d.draw(p.Tabs[cur].Child, scope, mouse.Rect{X: r.X, Y: r.Y + 1, W: r.W, H: r.H - 1})
	}
	return block(render.Truncate(head.String(), r.W), r.W, 1) + "\n" + body, nil
}

func (d *drawer) listView(c Component, scope string, r mouse.Rect) (string, error) {
	p, err := decode[listProps](c)
	if err != nil {
		return "", err
	}
	rows := d.listRows(p, scope)
	key := uiKey(c.ID, scope)
	l, ok := d.ui.lists[key]
	if d.sync {
		if !ok {
			l = list.New(rows)
		} else {
			l = l.SetItems(rows) // the cursor stays on its item id (E3)
		}
		h := r.H
		if len(rows) > r.H {
			h = r.H - 1 // room for the "↓ N more" line
		}
		l = l.SetHeight(max(1, h))
		d.ui.lists[key] = l
	}
	focused := d.ui.focus == key
	start, end := l.Window()
	d.place(placed{key: key, id: c.ID, kind: "list", scope: scope, rect: mouse.Rect{X: r.X, Y: r.Y, W: r.W, H: max(0, end-start)}, start: start})
	if l.Len() == 0 {
		return d.note(r, d.th.Muted, "(empty)"), nil
	}
	sel := l.Index()
	var lines []string
	for i := start; i < end; i++ {
		it := l.At(i)
		if i != sel {
			d.quiet++
		}
		label := d.draw(it.comp, it.scope, mouse.Rect{X: r.X + 2, Y: r.Y + len(lines), W: max(1, r.W-2), H: 1})
		if i != sel {
			d.quiet--
		}
		line := "  " + label
		if i == sel {
			line = d.th.Glyphs.Cursor + " " + label
			if focused {
				line = d.th.Selected.Render(render.Pad(d.th.Glyphs.Cursor+" "+ansi.Strip(label), r.W))
			}
		}
		lines = append(lines, render.Truncate(line, r.W))
	}
	if more := l.MoreLine(); more != "" && len(lines) < r.H {
		lines = append(lines, d.th.Subtle.Render(more))
	}
	return block(strings.Join(lines, "\n"), r.W, r.H), nil
}

// listRows turns the List's children into rows with stable ids.
func (d *drawer) listRows(p listProps, scope string) []row {
	kids := d.children(p.Children, scope)
	rows := make([]row, 0, len(kids))
	for i, k := range kids {
		id := k.id
		if p.Children.Template != "" {
			id = "#" + strconv.Itoa(i)
			if p.ItemKey != "" {
				if v, ok := lookup(d.sf.Data, join(k.scope, p.ItemKey)); ok && v != nil {
					id = stringify(v)
				}
			}
		}
		rows = append(rows, row{id: id, comp: k.id, scope: k.scope})
	}
	return rows
}
