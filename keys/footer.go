package keys

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// FooterStyle styles the footer. The zero value renders plain text with two
// spaces between hints and word key names, which is what NO_COLOR wants
// anyway.
type FooterStyle struct {
	Key  lipgloss.Style // the key ("q")
	Desc lipgloss.Style // the description ("quit")
	More lipgloss.Style // the "…" that marks dropped hints
	// Separator goes between hints. ASCII by default: `·` and `│` are East
	// Asian ambiguous and render two cells on some CJK terminals, which would
	// push a footer measured as fitting past the edge (gcl hints.go).
	Separator string
	Names     Names // how keys are displayed
	// GroupSeparator joins the keys of a FooterGroup hint ("j/k"). Empty
	// concatenates single-cell names ("hjkl", "↑↓") and uses "/" otherwise.
	GroupSeparator string
}

// hint is one footer entry: a binding, or a FooterGroup of them.
type hint struct {
	keys   []string
	label  string
	prio   int
	pinned bool
}

// Footer renders the one-line hint bar for the active contexts.
//
// Hints come from bindings with Footer > 0 or Pinned that o.Enabled and
// o.Visible both allow and that no inner context shadows. Bindings sharing a
// FooterGroup collapse into one hint. Unpinned hints are ordered by Footer
// (lower first) and dropped from the high-number end until the line fits;
// pinned hints (quit, help) go last and are never dropped, because they are
// how a user finds the keys that did not fit (argx view_chrome.go, gcl
// hints.go). When anything was dropped, `…` says so. The result never exceeds
// width cells; if even the pinned hints do not fit, the line is truncated.
func (km Keymap) Footer(active []Ctx, width int, o Opts, st FooterStyle) string {
	if width <= 0 {
		return ""
	}
	sep := st.Separator
	if sep == "" {
		sep = "  "
	}
	var hints []hint
	at := map[string]int{}
	for _, s := range km.reachable(active, o.Enabled) {
		b := s.Binding
		if (!b.Pinned && b.Footer <= 0) || !o.visible(b.Action) {
			continue
		}
		if i, ok := at[b.FooterGroup]; ok && b.FooterGroup != "" {
			h := &hints[i]
			h.keys = append(h.keys, b.Key())
			h.prio = min(h.prio, b.Footer)
			h.pinned = h.pinned || b.Pinned
			continue
		}
		h := hint{keys: []string{b.Key()}, label: b.footerLabel(o.State), prio: b.Footer, pinned: b.Pinned}
		if b.FooterGroup != "" {
			h.label = b.FooterGroup
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
		ks := st.Names.Join(h.keys)
		if st.GroupSeparator != "" {
			names := make([]string, len(h.keys))
			for i, k := range h.keys {
				names[i] = st.Names.Name(k)
			}
			ks = strings.Join(names, st.GroupSeparator)
		}
		k := st.Key.Render(ks)
		if h.label == "" {
			return k
		}
		return k + " " + st.Desc.Render(h.label)
	}
	render := func(n int) string {
		parts := make([]string, 0, n+len(pinned)+1)
		for _, h := range body[:n] {
			parts = append(parts, draw(h))
		}
		if n < len(body) {
			parts = append(parts, st.More.Render("…"))
		}
		for _, h := range pinned {
			parts = append(parts, draw(h))
		}
		return strings.Join(parts, sep)
	}

	n := len(body)
	line := render(n)
	for n > 0 && ansi.StringWidth(line) > width {
		n--
		line = render(n)
	}
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "…")
	}
	return line
}
