package keys

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/render"
)

// FooterStyle styles the footer. The zero value renders plain text with two
// spaces between hints and word key names, which is what NO_COLOR wants
// anyway.
type FooterStyle struct {
	Key  lipgloss.Style // the key ("q")
	Desc lipgloss.Style // the description ("quit")
	More lipgloss.Style // the marker for dropped hints
	// Separator goes between hints. ASCII by default: `·` and `│` are East
	// Asian ambiguous and render two cells on some CJK terminals, which would
	// push a footer measured as fitting past the edge (gcl hints.go).
	Separator string
	Names     Names // how keys are displayed
	// GroupSeparator joins the keys of a FooterGroup hint ("j/k"). Empty
	// concatenates single-cell names ("hjkl", "↑↓") and uses "/" otherwise.
	GroupSeparator string
	// Ellipsis marks dropped hints and a cut line. Empty uses
	// render.Ellipsis(): `…`, or `~` under a CJK locale where `…` is two
	// cells wide.
	Ellipsis string
	// Right is drawn flush right after the hints: a scroll position, a
	// legend, a count. The hints make room for it first — the lowest
	// priorities drop — and it goes only when the pinned hints alone no
	// longer fit beside it. Style it yourself; it is counted by its cells.
	Right string
}

// hint is one footer entry: a binding, or a FooterGroup of them.
type hint struct {
	names  []string // display names of the member keys (KeyLabel when set)
	label  string
	prio   int
	pinned bool
	group  string
	// labelled is set once a group member's GroupLabels answered, so a later
	// member does not overwrite it.
	labelled bool
}

// Footer renders the one-line hint bar for the active contexts.
//
// Hints come from bindings with Footer > 0 or Pinned that o.Enabled and
// o.Visible both allow and that no inner context shadows. Bindings sharing a
// FooterGroup collapse into one hint. Unpinned hints are ordered by Footer
// (lower first) and dropped from the high-number end until the line fits;
// pinned hints (quit, help) go last and are never dropped, because they are
// how a user finds the keys that did not fit (argx view_chrome.go, gcl
// hints.go). When anything was dropped the Ellipsis marker says so.
//
// On a bar too narrow for even that, things go in this order: unpinned
// hints, then the marker, then Right, and only then are the pinned hints
// cut — the marker never pushes quit off the line (gcl, under 18 columns).
// The result never exceeds width cells.
func (km Keymap) Footer(active []Ctx, width int, o Opts, st FooterStyle) string {
	if width <= 0 {
		return ""
	}
	sep := st.Separator
	if sep == "" {
		sep = "  "
	}
	marker := render.Or(st.Ellipsis)
	var hints []hint
	at := map[string]int{}
	for _, s := range km.reachable(active, o.Enabled) {
		b := s.Binding
		if (!b.Pinned && b.Footer <= 0) || !o.visible(b.Action) {
			continue
		}
		name := b.KeyLabel
		if name == "" {
			name = st.Names.Name(b.Key())
		}
		if i, ok := at[b.FooterGroup]; ok && b.FooterGroup != "" {
			h := &hints[i]
			h.names = append(h.names, name)
			h.prio = min(h.prio, b.Footer)
			h.pinned = h.pinned || b.Pinned
			if l, ok := groupLabel(b, o); ok && !h.labelled {
				h.label, h.labelled = l, true
			}
			continue
		}
		h := hint{names: []string{name}, label: b.footerLabel(o), prio: b.Footer, pinned: b.Pinned}
		if b.FooterGroup != "" {
			h.group, h.label = b.FooterGroup, b.FooterGroup
			if l, ok := groupLabel(b, o); ok {
				h.label, h.labelled = l, true
			}
			at[b.FooterGroup] = len(hints)
		}
		hints = append(hints, h)
	}
	var body, pinned []hint
	for _, h := range hints {
		if h.pinned {
			pinned = append(pinned, h)
		} else {
			body = append(body, h)
		}
	}
	// Stable, so equal priorities keep innermost-context-first order.
	sort.SliceStable(body, func(i, j int) bool { return body[i].prio < body[j].prio })
	sort.SliceStable(pinned, func(i, j int) bool { return pinned[i].prio < pinned[j].prio })

	draw := func(h hint) string {
		k := st.Key.Render(joinNames(h.names, st.GroupSeparator))
		if h.label == "" {
			return k
		}
		return k + " " + st.Desc.Render(h.label)
	}
	right, rightW := st.Right, render.Width(st.Right)
	// line draws the first n body hints; ok is false when it does not fit.
	line := func(n int, withMore, withRight bool) (string, bool) {
		parts := make([]string, 0, n+len(pinned)+1)
		for _, h := range body[:n] {
			parts = append(parts, draw(h))
		}
		if withMore && n < len(body) {
			parts = append(parts, st.More.Render(marker))
		}
		for _, h := range pinned {
			parts = append(parts, draw(h))
		}
		left := strings.Join(parts, sep)
		lw := render.Width(left)
		if !withRight || right == "" {
			return left, lw <= width
		}
		if left == "" {
			return strings.Repeat(" ", max(0, width-rightW)) + right, rightW <= width
		}
		gap := width - lw - rightW
		if gap < render.Width(sep) {
			return left, false
		}
		return left + strings.Repeat(" ", gap) + right, true
	}
	for n := len(body); n >= 0; n-- {
		if l, ok := line(n, true, true); ok {
			return l
		}
	}
	if l, ok := line(0, false, true); ok {
		return l
	}
	l, ok := line(0, false, false)
	if !ok {
		l = render.TruncateMarker(l, width, marker)
	}
	return l
}

// groupLabel is the GroupLabels entry for b's current state, if any.
func groupLabel(b Binding, o Opts) (string, bool) {
	if o.State == nil || len(b.GroupLabels) == 0 {
		return "", false
	}
	l, ok := b.GroupLabels[o.State(b.Action)]
	return l, ok
}

// joinNames joins display names: with sep when set, else concatenated when
// every name is one cell ("hjkl", "↑↓") and slash-separated otherwise.
func joinNames(names []string, sep string) string {
	if sep == "" {
		for _, n := range names {
			if render.Width(n) != 1 {
				sep = "/"
				break
			}
		}
	}
	return strings.Join(names, sep)
}
