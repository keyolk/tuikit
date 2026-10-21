package keys

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/fuzzy"
	"github.com/keyolk/tuikit/render"
)

// Section is one group of the help screen.
type Section struct {
	// ID is the key sections merge by: Binding.Section, else the title.
	ID    string
	Title string
	// Entries are the section's actions.
	Entries []Entry
	// Notes are prose paragraphs drawn after the entries, word-wrapped to
	// the help width: what a mode means, a caveat, where a file lives. Apps
	// append them to a section Help built, or add a Section of their own.
	Notes []string
}

// Entry is one action as shown in help and the palette. Help is the label in
// effect (Binding.Label), Keys the raw key strings that reach it; display
// them with Names, or KeyLabel when it is set.
type Entry struct {
	Action   string
	Help     string
	Keys     []string
	KeyLabel string
	Group    string // the section title
	Section  string // the section ID
}

func entryOf(b Binding, o Opts) Entry {
	g := b.Group
	if g == "" {
		g = string(b.Ctx)
	}
	sec := b.Section
	if sec == "" {
		sec = g
	}
	return Entry{Action: b.Action, Help: b.Label(o), Keys: b.Keys, KeyLabel: b.KeyLabel, Group: g, Section: sec}
}

// Help groups the bindings reachable in any of stacks (each innermost-first,
// as for Resolve) into sections keyed by Binding.Section, else Group.
// Sections come in the order they first appear, walking the stacks in order
// and each innermost-first — so the keys of the pane the user is looking at
// come before the global ones. Pass the current stack first, then any other
// the help page should document (a detail pane that is closed right now).
//
// An action reachable from several stacks or contexts is listed once, with
// the union of the keys that reach it in any of them, and sections with the
// same ID merge. Disabled bindings are listed — help documents what exists —
// so o.Enabled and o.Visible are not read. Keys shadowed in every stack are
// not listed, since pressing them never runs the action. Bindings without
// Help (or Labels) are omitted.
func (km Keymap) Help(o Opts, stacks ...[]Ctx) []Section {
	var out []Section
	at := map[string]int{}       // section ID → section
	entry := map[string][2]int{} // action → section, entry
	for _, stack := range stacks {
		for _, s := range km.reachable(stack, nil) {
			if s.Help == "" && len(s.Labels) == 0 {
				continue
			}
			if pos, ok := entry[s.Action]; ok {
				e := &out[pos[0]].Entries[pos[1]]
				for _, k := range s.Keys {
					if !slices.Contains(e.Keys, k) {
						e.Keys = append(e.Keys, k)
					}
				}
				e.Keys = km.inDeclOrder(s.Action, e.Keys)
				continue
			}
			e := entryOf(s.Binding, o)
			si, ok := at[e.Section]
			if !ok {
				si = len(out)
				at[e.Section] = si
				out = append(out, Section{ID: e.Section, Title: e.Group})
			}
			entry[s.Action] = [2]int{si, len(out[si].Entries)}
			out[si].Entries = append(out[si].Entries, e)
		}
	}
	return out
}

// Table is the full binding table as help sections: every context on its
// own, so nothing is hidden by shadowing, each action once. With
// HelpStyle.Actions it is the reference view a user writes keys.yaml from.
func (km Keymap) Table(o Opts) []Section {
	var stacks [][]Ctx
	seen := map[Ctx]bool{}
	for _, b := range km.bindings {
		if !seen[b.Ctx] {
			seen[b.Ctx] = true
			stacks = append(stacks, []Ctx{b.Ctx})
		}
	}
	return km.Help(o, stacks...)
}

// inDeclOrder sorts ks into the order the table declares them for action,
// so a merged entry reads "q/ctrl+c" whichever stack contributed which key.
func (km Keymap) inDeclOrder(action string, ks []string) []string {
	var out []string
	for _, b := range km.bindings {
		if b.Action != action {
			continue
		}
		for _, k := range b.Keys {
			if slices.Contains(ks, k) && !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// HelpStyle styles RenderHelp. The zero value is plain text with word key
// names; theme.Theme.HelpStyle maps it to the theme's roles.
type HelpStyle struct {
	Title lipgloss.Style // section titles
	Key   lipgloss.Style // the key column
	Desc  lipgloss.Style // the description
	Muted lipgloss.Style // the "… N more" line, the action column and notes
	// Actions adds a column with each entry's action name: the name a user
	// writes in keys.yaml (see Keymap.Table).
	Actions bool
	Names   Names // how keys are displayed
	// Ellipsis marks cut text and the "N more" line. Empty uses
	// render.Ellipsis() (`~` under a CJK locale).
	Ellipsis string
}

// helpLine is one laid-out line: a blank separator, a title, an entry or a
// line of a note. Pieces stay apart until they are cut to width.
type helpLine struct {
	title, key, desc, action, note string
	entry                          bool
}

// helpLayout lays sections out for width: the lines and the key and action
// column widths. RenderHelp draws it; HelpLineCount counts it.
func helpLayout(sections []Section, width int, st HelpStyle) (all []helpLine, keyW, actW int) {
	keyW = 0
	for _, s := range sections {
		for _, e := range s.Entries {
			keyW = max(keyW, render.Width(keyText(e, st.Names)))
		}
	}
	keyW = min(keyW, width/3)
	if st.Actions {
		for _, sec := range sections {
			for _, e := range sec.Entries {
				actW = max(actW, render.Width(e.Action))
			}
		}
		actW = min(actW, width/3)
	}
	noteW := max(1, width-2)
	for si, s := range sections {
		if si > 0 {
			all = append(all, helpLine{})
		}
		all = append(all, helpLine{title: s.Title})
		for _, e := range s.Entries {
			all = append(all, helpLine{key: keyText(e, st.Names), desc: e.Help, action: e.Action, entry: true})
		}
		for _, n := range s.Notes {
			for _, l := range strings.Split(ansi.Wrap(n, noteW, ""), "\n") {
				all = append(all, helpLine{note: l})
			}
		}
	}
	return all, keyW, actW
}

func keyText(e Entry, n Names) string {
	if e.KeyLabel != "" {
		return e.KeyLabel
	}
	names := make([]string, len(e.Keys))
	for i, k := range e.Keys {
		names[i] = n.Name(k)
	}
	return strings.Join(names, "/")
}

// HelpLineCount is how many lines RenderHelp would lay sections out in at
// width, before paging: the box height a help overlay wants, and the bound
// an Update that scrolls help clamps against — without counting sections,
// entries, blank lines and wrapped notes by hand (brd).
func HelpLineCount(sections []Section, width int) int {
	if width <= 0 {
		return 0
	}
	all, _, _ := helpLayout(sections, width, HelpStyle{})
	return len(all)
}

// RenderHelp lays sections out as lines of at most width cells, showing at
// most height lines starting at offset. When lines are cut below, the last
// line says `… N more` instead of overflowing the frame (E7). Notes are
// word-wrapped under their section's entries.
//
// offset is clamped to [0, last full page] and returned; store it back:
//
//	lines, m.helpOff = keys.RenderHelp(secs, w, h, m.helpOff, st)
//
// Otherwise a held j keeps growing the stored offset past the end, and the
// user has to press k as many times again before the page moves.
//
// Every piece is cut to its cell budget before it is styled, so a style never
// inflates a line past width.
func RenderHelp(sections []Section, width, height, offset int, st HelpStyle) ([]string, int) {
	if width <= 0 || height <= 0 {
		return nil, 0
	}
	marker := render.Or(st.Ellipsis)
	cut := func(s string, w int) string { return render.TruncateMarker(s, w, marker) }
	all, keyW, actW := helpLayout(sections, width, st)
	descW := max(0, width-keyW-4)
	if actW > 0 {
		descW = max(0, descW-actW-2)
	}
	offset = max(0, min(offset, len(all)-height))
	all = all[offset:]
	var more string
	if len(all) > height {
		more = fmt.Sprintf("%s %d more", marker, len(all)-(height-1))
		all = all[:height-1]
	}
	out := make([]string, 0, len(all)+1)
	for _, l := range all {
		switch {
		case l.entry:
			s := "  " + st.Key.Render(render.PadRight(cut(l.key, keyW), keyW))
			switch d := cut(l.desc, descW); {
			case actW > 0:
				s += "  " + st.Desc.Render(render.PadRight(d, descW)) + "  " + st.Muted.Render(cut(l.action, actW))
			case d != "":
				s += "  " + st.Desc.Render(d)
			}
			// keyW is at most width/3, so only a width under 3 overflows.
			out = append(out, cut(s, width))
		case l.title != "":
			out = append(out, st.Title.Render(cut(l.title, width)))
		case l.note != "":
			out = append(out, st.Muted.Render(cut("  "+l.note, width)))
		default:
			out = append(out, "")
		}
	}
	if more != "" {
		out = append(out, st.Muted.Render(cut(more, width)))
	}
	return out, offset
}

// Entries lists the enabled, unshadowed actions in active for a command
// palette, minus NoPalette bindings: the same table as dispatch, so the palette can never offer an
// action the keys would not run. o.Visible is not consulted: the palette is
// where the keys the footer had no room for are found.
func (km Keymap) Entries(active []Ctx, o Opts) []Entry {
	var out []Entry
	for _, s := range km.reachable(active, o.Enabled) {
		if s.NoPalette {
			continue
		}
		if e := entryOf(s.Binding, o); e.Help != "" {
			out = append(out, e)
		}
	}
	return out
}

// Filter ranks entries against a palette query with fzf-style subsequence
// scoring (smart case), matching the description or the action name, best
// first. An empty query returns entries unchanged.
func Filter(entries []Entry, query string) []Entry {
	if query == "" {
		return entries
	}
	type scored struct {
		e     Entry
		score int
	}
	var hits []scored
	for _, e := range entries {
		s1, _, ok1 := fuzzy.Score(e.Help, query)
		s2, _, ok2 := fuzzy.Score(e.Action, query)
		switch {
		case ok1 && ok2:
			hits = append(hits, scored{e, max(s1, s2)})
		case ok1:
			hits = append(hits, scored{e, s1})
		case ok2:
			hits = append(hits, scored{e, s2})
		}
	}
	// Stable, so equal scores keep the innermost-context-first order.
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := make([]Entry, len(hits))
	for i, h := range hits {
		out[i] = h.e
	}
	return out
}

// HelpLines is every help line at width, with no height cut and no `… N
// more` marker: the full content a scrolling help pane passes as
// selection.Pane.Lines, or a View that windows the help itself.
func HelpLines(sections []Section, width int, st HelpStyle) []string {
	n := HelpLineCount(sections, width)
	if n == 0 {
		return nil
	}
	lines, _ := RenderHelp(sections, width, n, 0, st)
	return lines
}
