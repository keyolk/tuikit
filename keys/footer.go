package keys

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// FooterStyle styles the footer. The zero value renders plain text with two
// spaces between hints, which is what NO_COLOR wants anyway.
type FooterStyle struct {
	Key  lipgloss.Style // the key ("q")
	Desc lipgloss.Style // the description ("quit")
	More lipgloss.Style // the "…" that marks dropped hints
	// Separator goes between hints. ASCII by default: `·` and `│` are East
	// Asian ambiguous and render two cells on some CJK terminals, which would
	// push a footer measured as fitting past the edge (gcl hints.go).
	Separator string
}

// Footer renders the one-line hint bar for the active contexts.
//
// Hints come from bindings with Footer > 0 or Pinned, enabled, and not
// shadowed by an inner context. Unpinned hints are ordered by Footer (lower
// first) and dropped from the high-number end until the line fits; pinned
// hints (quit, help) go last and are never dropped, because they are how a
// user finds the keys that did not fit (argx view_chrome.go, gcl hints.go).
// When anything was dropped, `…` says so. The result never exceeds width
// cells; if even the pinned hints do not fit, the line is truncated.
func (km Keymap) Footer(active []Ctx, width int, enabled func(string) bool, st FooterStyle) string {
	if width <= 0 {
		return ""
	}
	sep := st.Separator
	if sep == "" {
		sep = "  "
	}
	var body, pinned []Binding
	for _, b := range km.visible(active, enabled) {
		switch {
		case b.Pinned:
			pinned = append(pinned, b)
		case b.Footer > 0:
			body = append(body, b)
		}
	}
	// Stable, so equal priorities keep innermost-context-first order.
	sort.SliceStable(body, func(i, j int) bool { return body[i].Footer < body[j].Footer })
	sort.SliceStable(pinned, func(i, j int) bool { return pinned[i].Footer < pinned[j].Footer })

	hint := func(b Binding) string {
		if b.Help == "" {
			return st.Key.Render(b.Key())
		}
		return st.Key.Render(b.Key()) + " " + st.Desc.Render(b.Help)
	}
	render := func(n int) string {
		parts := make([]string, 0, n+len(pinned)+1)
		for _, b := range body[:n] {
			parts = append(parts, hint(b))
		}
		if n < len(body) {
			parts = append(parts, st.More.Render("…"))
		}
		for _, b := range pinned {
			parts = append(parts, hint(b))
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
