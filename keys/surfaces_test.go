package keys

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestConflictsDuplicate(t *testing.T) {
	km := New(
		Binding{Action: "a", Keys: []string{"x"}, Ctx: ctxList},
		Binding{Action: "b", Keys: []string{"x"}, Ctx: ctxList},
		Binding{Action: "c", Keys: []string{"x"}, Ctx: ctxModal}, // other ctx: fine
	)
	cs := km.Conflicts()
	if len(cs) != 1 || cs[0].Kind != Duplicate || cs[0].Ctx != ctxList || strings.Join(cs[0].Actions, ",") != "a,b" {
		t.Fatalf("conflicts = %v", cs)
	}
	if !strings.Contains(cs[0].String(), "duplicate") {
		t.Errorf("String() = %q", cs[0].String())
	}
}

func TestConflictsShadowedPinned(t *testing.T) {
	km := New(
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal, Pinned: true},
		Binding{Action: "refresh", Keys: []string{"r"}, Ctx: ctxGlobal},
		Binding{Action: "list.queue", Keys: []string{"q"}, Ctx: ctxList},
		Binding{Action: "list.reload", Keys: []string{"r"}, Ctx: ctxList}, // unpinned outer: intended override
	)
	if cs := km.Conflicts(); len(cs) != 0 {
		t.Fatalf("without stacks no shadowing can be judged: %v", cs)
	}
	cs := km.Conflicts([]Ctx{ctxList, ctxGlobal}, []Ctx{ctxList, ctxGlobal})
	if len(cs) != 1 || cs[0].Kind != Shadowed || cs[0].Key != "q" || cs[0].Outer != ctxGlobal {
		t.Fatalf("conflicts = %v; want one shadowed q, reported once", cs)
	}
	if !strings.Contains(cs[0].String(), "hides pinned quit") {
		t.Errorf("String() = %q", cs[0].String())
	}
}

func TestConflictsUnreachableUppercase(t *testing.T) {
	km := New(
		Binding{Action: "expand", Keys: []string{"F"}, Ctx: ctxList},
		Binding{Action: "refresh", Keys: []string{"R"}, Ctx: ctxList}, // ㄲ produces R
	)
	cs := km.Conflicts()
	if len(cs) != 1 || cs[0].Kind != Unreachable || cs[0].Key != "F" {
		t.Fatalf("conflicts = %v", cs)
	}
}

func TestTestKeymapIsClean(t *testing.T) {
	for _, c := range testKeymap().Conflicts([]Ctx{ctxList, ctxGlobal}) {
		t.Error(c)
	}
}

func TestFooterOrdersByPriorityAndPinsLast(t *testing.T) {
	got := testKeymap().Footer([]Ctx{ctxList, ctxGlobal}, 200, Opts{}, FooterStyle{})
	want := "j down  k up  enter open  d delete  r refresh  ? help  q quit"
	if got != want {
		t.Fatalf("footer =\n%q\nwant\n%q", got, want)
	}
}

func TestFooterShowsOnlyUnshadowedKeys(t *testing.T) {
	// enter belongs to list.open; in the modal, q is claimed by modal.close,
	// so quit is advertised by its other key.
	got := testKeymap().Footer([]Ctx{ctxModal, ctxGlobal}, 200, Opts{}, FooterStyle{})
	if !strings.Contains(got, "ctrl+c quit") || strings.Contains(got, "q quit") {
		t.Fatalf("footer = %q; quit must be shown by a key that still reaches it", got)
	}
}

func TestFooterDropsLowPriorityKeepsPinned(t *testing.T) {
	km := testKeymap()
	for w := 1; w <= 80; w++ {
		got := km.Footer([]Ctx{ctxList, ctxGlobal}, w, Opts{}, FooterStyle{})
		if ansi.StringWidth(got) > w {
			t.Fatalf("width %d: footer is %d cells: %q", w, ansi.StringWidth(got), got)
		}
		if w >= 30 && (!strings.Contains(got, "? help") || !strings.Contains(got, "q quit")) {
			t.Fatalf("width %d: pinned hint dropped: %q", w, got)
		}
	}
	got := km.Footer([]Ctx{ctxList, ctxGlobal}, 30, Opts{}, FooterStyle{})
	if !strings.Contains(got, "…") || strings.Contains(got, "refresh") {
		t.Fatalf("narrow footer = %q; want refresh dropped first and … shown", got)
	}
}

func TestFooterHidesDisabled(t *testing.T) {
	got := testKeymap().Footer([]Ctx{ctxList, ctxGlobal}, 200, Opts{Enabled: func(a string) bool { return a != "list.delete" }}, FooterStyle{})
	if strings.Contains(got, "delete") {
		t.Fatalf("footer = %q; disabled action advertised", got)
	}
}

func TestFooterWideRunes(t *testing.T) {
	km := New(
		Binding{Action: "a", Keys: []string{"a"}, Ctx: ctxList, Help: "배포 상태 보기", Footer: 1},
		Binding{Action: "b", Keys: []string{"b"}, Ctx: ctxList, Help: "🚀 launch", Footer: 2},
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxList, Help: "종료", Pinned: true},
	)
	for w := 0; w <= 40; w++ {
		if got := km.Footer([]Ctx{ctxList}, w, Opts{}, FooterStyle{Separator: " | "}); ansi.StringWidth(got) > w {
			t.Fatalf("width %d: %q is %d cells", w, got, ansi.StringWidth(got))
		}
	}
}

func TestHelpGroupsInContextOrder(t *testing.T) {
	secs := testKeymap().Help(Opts{}, []Ctx{ctxList, ctxGlobal})
	var titles []string
	for _, s := range secs {
		titles = append(titles, s.Title)
	}
	if strings.Join(titles, ",") != "Move,Act,global" {
		t.Fatalf("sections = %v", titles)
	}
	if secs[0].Entries[0].Action != "list.down" || strings.Join(secs[0].Entries[0].Keys, "/") != "j/down" {
		t.Fatalf("first entry = %+v", secs[0].Entries[0])
	}
}

func TestRenderHelpCutsHonestly(t *testing.T) {
	secs := testKeymap().Help(Opts{}, []Ctx{ctxList, ctxGlobal})
	full := rh(secs, 40, 100, 0, HelpStyle{})
	if len(full) != 12 { // 3 titles + 7 entries + 2 blank separators
		t.Fatalf("full help is %d lines:\n%s", len(full), strings.Join(full, "\n"))
	}
	cut := rh(secs, 40, 5, 0, HelpStyle{})
	if len(cut) != 5 || !strings.HasPrefix(cut[4], "… ") || !strings.HasSuffix(cut[4], "more") {
		t.Fatalf("cut = %q", cut)
	}
	if want := len(full) - 4; !strings.Contains(cut[4], strconv.Itoa(want)) {
		t.Fatalf("cut says %q, want %d more", cut[4], want)
	}
	for _, l := range rh(secs, 12, 100, 0, HelpStyle{}) {
		if ansi.StringWidth(l) > 12 {
			t.Fatalf("line %q exceeds width", l)
		}
	}
	if scrolled := rh(secs, 40, 100, 1000, HelpStyle{}); len(scrolled) != len(full) {
		t.Fatalf("a page taller than the help must not scroll, got %q", scrolled)
	}
	if rh(secs, 0, 10, 0, HelpStyle{}) != nil || rh(secs, 10, 0, 0, HelpStyle{}) != nil {
		t.Fatal("zero size renders nothing")
	}
}

func TestPaletteEntriesAndFilter(t *testing.T) {
	km := testKeymap()
	es := km.Entries([]Ctx{ctxList, ctxGlobal}, Opts{Enabled: func(a string) bool { return a != "refresh" }})
	for _, e := range es {
		if e.Action == "refresh" {
			t.Fatal("disabled action offered in the palette")
		}
	}
	got := Filter(es, "del")
	if len(got) == 0 || got[0].Action != "list.delete" {
		t.Fatalf("Filter(del) = %+v", got)
	}
	if got := Filter(es, "list.op"); len(got) == 0 || got[0].Action != "list.open" {
		t.Fatalf("Filter by action name = %+v", got)
	}
	if got := Filter(es, ""); len(got) != len(es) {
		t.Fatal("empty query returns everything")
	}
	if got := Filter(es, "zzz"); len(got) != 0 {
		t.Fatalf("no match expected, got %+v", got)
	}
}

// rh is RenderHelp without the clamped offset.
func rh(secs []Section, w, h, off int, st HelpStyle) []string {
	lines, _ := RenderHelp(secs, w, h, off, st)
	return lines
}

func TestRenderHelpClampsAndReturnsOffset(t *testing.T) {
	secs := testKeymap().Help(Opts{}, []Ctx{ctxList, ctxGlobal}) // 12 lines
	const h = 5
	// A held j: the stored offset must stop at the last full page.
	off := 0
	for range 100 {
		_, off = RenderHelp(secs, 40, h, off+1, HelpStyle{})
	}
	if off != 12-h {
		t.Fatalf("offset after 100 downs = %d, want %d", off, 12-h)
	}
	lines, _ := RenderHelp(secs, 40, h, off, HelpStyle{})
	if len(lines) != h || strings.Contains(lines[h-1], "more") {
		t.Fatalf("last page = %q; want %d full lines, nothing more below", lines, h)
	}
	// One k moves the page at once.
	if _, back := RenderHelp(secs, 40, h, off-1, HelpStyle{}); back != off-1 {
		t.Fatalf("one up = %d", back)
	}
	if _, neg := RenderHelp(secs, 40, h, -3, HelpStyle{}); neg != 0 {
		t.Fatalf("negative offset clamps to 0, got %d", neg)
	}
	if _, tall := RenderHelp(secs, 40, 100, 7, HelpStyle{}); tall != 0 {
		t.Fatalf("everything fits: offset %d, want 0", tall)
	}
}
