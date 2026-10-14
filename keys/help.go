package keys

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/keyolk/tuikit/internal/fuzzy"
)

// Section is one group of the help screen.
type Section struct {
	Title   string
	Entries []Entry
}

// Entry is one action as shown in help and the palette.
type Entry struct {
	Action string
	Help   string
	Keys   []string
	Group  string
}

func entryOf(b Binding) Entry {
	g := b.Group
	if g == "" {
		g = string(b.Ctx)
	}
	return Entry{Action: b.Action, Help: b.Help, Keys: b.Keys, Group: g}
}

// Help groups the bindings reachable in active by Group, sections in the order
// they first appear walking contexts innermost-first — so the keys of the pane
// the user is looking at come before the global ones. Disabled bindings are
// listed (help documents what exists); shadowed ones are not, since pressing
// their key would do something else here. Bindings without Help are omitted.
func (km Keymap) Help(active []Ctx) []Section {
	var out []Section
	at := map[string]int{}
	for _, b := range km.visible(active, nil) {
		if b.Help == "" {
			continue
		}
		e := entryOf(b)
		i, ok := at[e.Group]
		if !ok {
			i = len(out)
			at[e.Group] = i
			out = append(out, Section{Title: e.Group})
		}
		out[i].Entries = append(out[i].Entries, e)
	}
	return out
}

// RenderHelp lays sections out as lines of at most width cells, showing at
// most height lines starting at offset. When lines are cut below, the last
// line says `… N more` instead of overflowing the frame (E7). offset is
// clamped, so a caller can scroll without knowing the total.
func RenderHelp(sections []Section, width, height, offset int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	keyW := 0
	for _, s := range sections {
		for _, e := range s.Entries {
			keyW = max(keyW, ansi.StringWidth(strings.Join(e.Keys, "/")))
		}
	}
	keyW = min(keyW, width/3)

	var all []string
	for si, s := range sections {
		if si > 0 {
			all = append(all, "")
		}
		all = append(all, s.Title)
		for _, e := range s.Entries {
			k := truncPad(strings.Join(e.Keys, "/"), keyW)
			all = append(all, "  "+k+"  "+e.Help)
		}
	}
	offset = max(0, min(offset, len(all)-1))
	all = all[offset:]
	if len(all) > height {
		rest := len(all) - (height - 1)
		all = append(all[:height-1:height-1], fmt.Sprintf("… %d more", rest))
	}
	out := make([]string, len(all))
	for i, l := range all {
		out[i] = ansi.Truncate(l, width, "…")
	}
	return out
}

func truncPad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}

// Entries lists the enabled, unshadowed actions in active for a command
// palette: the same table as dispatch, so the palette can never offer an
// action the keys would not run.
func (km Keymap) Entries(active []Ctx, enabled func(string) bool) []Entry {
	var out []Entry
	for _, b := range km.visible(active, enabled) {
		if b.Help != "" {
			out = append(out, entryOf(b))
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
