package keys

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/keyolk/tuikit/internal/fuzzy"
)

// Section is one group of the help screen.
type Section struct {
	Title   string
	Entries []Entry
}

// Entry is one action as shown in help and the palette. Help is the label in
// effect (Binding.Label), Keys the raw key strings that reach it; display
// them with Names.
type Entry struct {
	Action string
	Help   string
	Keys   []string
	Group  string
}

func entryOf(b Binding, state func(string) string) Entry {
	g := b.Group
	if g == "" {
		g = string(b.Ctx)
	}
	return Entry{Action: b.Action, Help: b.Label(state), Keys: b.Keys, Group: g}
}

// Help groups the bindings reachable in any of stacks (each innermost-first,
// as for Resolve) by Group. Sections come in the order they first appear,
// walking the stacks in order and each innermost-first — so the keys of the
// pane the user is looking at come before the global ones. Pass the current
// stack first, then any other the help page should document (a detail pane
// that is closed right now).
//
// A binding reachable from several stacks is listed once, with the union of
// the keys that reach it in any of them, and sections with the same title
// merge. Disabled bindings are listed — help documents what exists — so only
// o.State is read. Keys shadowed in every stack are not listed, since
// pressing them never runs the action. Bindings without Help are omitted.
func (km Keymap) Help(o Opts, stacks ...[]Ctx) []Section {
	var out []Section
	at := map[string]int{}    // group → section
	entry := map[int][2]int{} // binding → section, entry
	for _, stack := range stacks {
		for _, s := range km.reachable(stack, nil) {
			if s.Help == "" && len(s.Labels) == 0 {
				continue
			}
			if pos, ok := entry[s.i]; ok {
				e := &out[pos[0]].Entries[pos[1]]
				for _, k := range s.Keys {
					if !slices.Contains(e.Keys, k) {
						e.Keys = append(e.Keys, k)
					}
				}
				e.Keys = km.inDeclOrder(s.i, e.Keys)
				continue
			}
			e := entryOf(s.Binding, o.State)
			si, ok := at[e.Group]
			if !ok {
				si = len(out)
				at[e.Group] = si
				out = append(out, Section{Title: e.Group})
			}
			entry[s.i] = [2]int{si, len(out[si].Entries)}
			out[si].Entries = append(out[si].Entries, e)
		}
	}
	return out
}

// inDeclOrder sorts ks into the order binding i declares them, so a merged
// entry reads "q/ctrl+c" whichever stack contributed which key.
func (km Keymap) inDeclOrder(i int, ks []string) []string {
	var out []string
	for _, k := range km.bindings[i].Keys {
		if slices.Contains(ks, k) {
			out = append(out, k)
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
	Muted lipgloss.Style // the "… N more" line
	Names Names          // how keys are displayed
}

// RenderHelp lays sections out as lines of at most width cells, showing at
// most height lines starting at offset. When lines are cut below, the last
// line says `… N more` instead of overflowing the frame (E7). offset is
// clamped, so a caller can scroll without knowing the total.
//
// Every piece is cut to its cell budget before it is styled, so a style never
// inflates a line past width.
func RenderHelp(sections []Section, width, height, offset int, st HelpStyle) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	keyText := func(e Entry) string {
		names := make([]string, len(e.Keys))
		for i, k := range e.Keys {
			names[i] = st.Names.Name(k)
		}
		return strings.Join(names, "/")
	}
	keyW := 0
	for _, s := range sections {
		for _, e := range s.Entries {
			keyW = max(keyW, ansi.StringWidth(keyText(e)))
		}
	}
	keyW = min(keyW, width/3)
	descW := max(0, width-keyW-4)

	// A line is a blank separator, a title, or an entry (key set); pieces
	// stay apart until they are cut to width.
	type line struct {
		title, key, desc string
		entry            bool
	}
	var all []line
	for si, s := range sections {
		if si > 0 {
			all = append(all, line{})
		}
		all = append(all, line{title: s.Title})
		for _, e := range s.Entries {
			all = append(all, line{key: keyText(e), desc: e.Help, entry: true})
		}
	}
	offset = max(0, min(offset, len(all)-1))
	all = all[offset:]
	var more string
	if len(all) > height {
		more = fmt.Sprintf("… %d more", len(all)-(height-1))
		all = all[:height-1]
	}
	out := make([]string, 0, len(all)+1)
	for _, l := range all {
		switch {
		case l.entry:
			s := "  " + st.Key.Render(truncPad(l.key, keyW))
			if d := ansi.Truncate(l.desc, descW, "…"); d != "" {
				s += "  " + st.Desc.Render(d)
			}
			// keyW is at most width/3, so only a width under 3 overflows.
			out = append(out, ansi.Truncate(s, width, "…"))
		case l.title != "":
			out = append(out, st.Title.Render(ansi.Truncate(l.title, width, "…")))
		default:
			out = append(out, "")
		}
	}
	if more != "" {
		out = append(out, st.Muted.Render(ansi.Truncate(more, width, "…")))
	}
	return out
}

func truncPad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}

// Entries lists the enabled, unshadowed actions in active for a command
// palette: the same table as dispatch, so the palette can never offer an
// action the keys would not run. o.Visible is not consulted: the palette is
// where the keys the footer had no room for are found.
func (km Keymap) Entries(active []Ctx, o Opts) []Entry {
	var out []Entry
	for _, s := range km.reachable(active, o.Enabled) {
		if e := entryOf(s.Binding, o.State); e.Help != "" {
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
