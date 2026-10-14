package keys

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// --- P0: reachability per action, LatinOnly opt-out ---

func TestUnreachableIsJudgedPerAction(t *testing.T) {
	km := New(
		// G alone: dead under a Korean layout.
		Binding{Action: "last", Keys: []string{"G"}, Ctx: ctxList},
		// G with an end alias (okx, argx): reachable.
		Binding{Action: "bottom", Keys: []string{"G", "end"}, Ctx: ctxModal},
		// The alias lives in a second binding of the same action and context.
		Binding{Action: "yank", Keys: []string{"Y"}, Ctx: ctxGlobal},
		Binding{Action: "yank", Keys: []string{"y"}, Ctx: ctxGlobal},
		// An intended uppercase-only key (dayx D/G).
		Binding{Action: "delete.all", Keys: []string{"D"}, Ctx: ctxList, LatinOnly: true},
		// An alias in another context does not help this one.
		Binding{Action: "first", Keys: []string{"H"}, Ctx: ctxList},
		Binding{Action: "first", Keys: []string{"home"}, Ctx: ctxModal},
		// Two unreachable keys: one report, keyed by the first.
		Binding{Action: "both", Keys: []string{"J", "K"}, Ctx: ctxList},
	)
	var got []string
	for _, c := range km.Conflicts() {
		if c.Kind != Unreachable {
			t.Errorf("unexpected %v", c)
			continue
		}
		got = append(got, c.Actions[0]+"@"+string(c.Ctx)+"="+c.Key)
	}
	want := []string{"last@list=G", "first@list=H", "both@list=J"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unreachable = %v, want %v", got, want)
	}
}

// --- P0: state-dependent FooterGroup label ---

func TestFooterGroupLabelFollowsState(t *testing.T) {
	km := New(
		Binding{Action: "prev", Keys: []string{"h"}, Ctx: ctxList, Help: "previous", Footer: 1, FooterGroup: "day",
			GroupLabels: map[string]string{"week": "week"}},
		Binding{Action: "next", Keys: []string{"l"}, Ctx: ctxList, Help: "next", Footer: 1, FooterGroup: "day"},
	)
	stack := []Ctx{ctxList}
	if got := km.Footer(stack, 80, Opts{}, FooterStyle{}); got != "hl day" {
		t.Fatalf("footer = %q", got)
	}
	week := Opts{State: func(string) string { return "week" }}
	if got := km.Footer(stack, 80, week, FooterStyle{}); got != "hl week" {
		t.Fatalf("week footer = %q", got)
	}
	// A later member's label is used when the first has none for the state.
	km2 := New(
		Binding{Action: "prev", Keys: []string{"h"}, Ctx: ctxList, Help: "p", Footer: 1, FooterGroup: "day"},
		Binding{Action: "next", Keys: []string{"l"}, Ctx: ctxList, Help: "n", Footer: 1, FooterGroup: "day",
			GroupLabels: map[string]string{"week": "week"}},
	)
	if got := km2.Footer(stack, 80, week, FooterStyle{}); got != "hl week" {
		t.Fatalf("later member footer = %q", got)
	}
	// Help is per member, unaffected.
	if es := km.Help(week, stack)[0].Entries; es[0].Help != "previous" {
		t.Fatalf("help = %+v", es)
	}
}

// --- P0: KeyLabel ---

func TestKeyLabelInFooterAndHelpAndKeyIndex(t *testing.T) {
	digits := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}
	km := New(
		Binding{Action: "tab", Keys: digits, KeyLabel: "1-9", Ctx: ctxList, Help: "go to tab", Short: "tab", Footer: 1},
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal, Help: "quit", Pinned: true},
	)
	stack := []Ctx{ctxList, ctxGlobal}
	if got := km.Footer(stack, 80, Opts{}, FooterStyle{}); got != "1-9 tab  q quit" {
		t.Fatalf("footer = %q", got)
	}
	lines, _ := RenderHelp(km.Help(Opts{}, stack), 60, 20, 0, HelpStyle{})
	if !strings.Contains(strings.Join(lines, "\n"), "1-9  go to tab") {
		t.Fatalf("help = %q", lines)
	}
	b, ok := km.Resolve(stack, runes("7"), Opts{})
	if !ok || b.KeyIndex(runes("7")) != 6 {
		t.Fatalf("KeyIndex = %d", b.KeyIndex(runes("7")))
	}
	if b.KeyIndex(runes("x")) != -1 {
		t.Fatal("a key not in the binding is -1")
	}
	// Under a Korean layout too.
	jk := New(Binding{Action: "move", Keys: []string{"j", "k"}, Ctx: ctxList})
	b, _ = jk.Resolve([]Ctx{ctxList}, runes("ㅏ"), Opts{})
	if b.KeyIndex(runes("ㅏ")) != 1 {
		t.Fatal("KeyIndex must normalize jamo like Resolve")
	}
	// Rebinding drops the label: "1-9" would name dead keys.
	over, err := km.Override(map[string][]string{"tab": {"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := over.Footer(stack, 80, Opts{}, FooterStyle{}); got != "a tab  q quit" {
		t.Fatalf("overridden footer = %q", got)
	}
	same, _ := km.Override(map[string][]string{"tab": digits})
	if same.Bindings()[0].KeyLabel != "1-9" {
		t.Fatal("an override to the same keys keeps the label")
	}
}

// --- P0 + P1: ellipsis marker, pinned over marker, Right ---

func TestFooterEllipsisAndNarrowBars(t *testing.T) {
	km := testKeymap()
	stack := []Ctx{ctxList, ctxGlobal}
	got := km.Footer(stack, 30, Opts{}, FooterStyle{Ellipsis: "~"})
	if !strings.Contains(got, "~") || strings.Contains(got, "…") {
		t.Fatalf("custom marker footer = %q", got)
	}
	t.Setenv("LANG", "ko_KR.UTF-8")
	if got := km.Footer(stack, 30, Opts{}, FooterStyle{}); strings.Contains(got, "…") || !strings.Contains(got, "~") {
		t.Fatalf("CJK default footer = %q", got)
	}
	t.Setenv("LANG", "en_US.UTF-8")

	// The pinned hints are 14 cells; with the marker 17.
	pinned := "? help  q quit"
	for w := len(pinned); w < len(pinned)+3; w++ {
		if got := km.Footer(stack, w, Opts{}, FooterStyle{}); got != pinned {
			t.Fatalf("width %d: the marker pushed pinned hints off: %q", w, got)
		}
	}
	if got := km.Footer(stack, len(pinned)+3, Opts{}, FooterStyle{}); got != "…  "+pinned {
		t.Fatalf("width %d: %q", len(pinned)+3, got)
	}
	// Narrower than the pinned hints: cut, still within width.
	if got := km.Footer(stack, 10, Opts{}, FooterStyle{}); ansi.StringWidth(got) > 10 {
		t.Fatalf("narrow footer = %q", got)
	}
}

func TestFooterRight(t *testing.T) {
	km := testKeymap()
	stack := []Ctx{ctxList, ctxGlobal}
	st := FooterStyle{Right: "12/40"}
	full := km.Footer(stack, 80, Opts{}, FooterStyle{})
	if got := km.Footer(stack, 80, Opts{}, st); got != full+strings.Repeat(" ", 80-len(full)-5)+"12/40" {
		t.Fatalf("footer with Right = %q", got)
	}
	// Hints drop before Right does.
	narrow := km.Footer(stack, 40, Opts{}, st)
	if narrow != "j down  k up  …  ? help  q quit    12/40" {
		t.Fatalf("narrow with Right = %q", narrow)
	}
	// Only pinned left: Right goes before the pinned hints are cut.
	pinned := "? help  q quit"
	if got := km.Footer(stack, len(pinned)+2, Opts{}, st); got != pinned {
		t.Fatalf("Right must yield to pinned hints: %q", got)
	}
	// No hints at all: Right alone, flush right.
	empty := New()
	if got := empty.Footer(nil, 10, Opts{}, st); got != "     12/40" {
		t.Fatalf("Right alone = %q", got)
	}
}

// --- P1: Opts.Label ---

func TestOptsLabelIsLiveAndWins(t *testing.T) {
	km := New(
		Binding{Action: "sched", Keys: []string{"W"}, Ctx: ctxList, Help: "scheduled", Short: "sched",
			Labels: map[string]string{"on": "unschedule"}, Footer: 1, LatinOnly: true},
		Binding{Action: "other", Keys: []string{"o"}, Ctx: ctxList, Help: "other", Footer: 2},
	)
	n := 3
	o := Opts{
		State: func(string) string { return "on" },
		Label: func(a string) (string, bool) {
			if a == "sched" {
				return strings.Repeat("x", 0) + itoa(n) + " scheduled", true
			}
			return "", false
		},
	}
	stack := []Ctx{ctxList}
	if got := km.Footer(stack, 80, o, FooterStyle{}); got != "W 3 scheduled  o other" {
		t.Fatalf("footer = %q", got)
	}
	n = 4 // read per call, never cached
	if got := km.Footer(stack, 80, o, FooterStyle{}); !strings.HasPrefix(got, "W 4 scheduled") {
		t.Fatalf("footer after change = %q", got)
	}
	if e := km.Entries(stack, o)[0]; e.Help != "4 scheduled" {
		t.Fatalf("palette = %+v", e)
	}
	if e := km.Help(o, stack)[0].Entries[0]; e.Help != "4 scheduled" {
		t.Fatalf("help = %+v", e)
	}
	// Label declining falls back to Labels.
	o.Label = func(string) (string, bool) { return "", false }
	if got := km.Footer(stack, 80, o, FooterStyle{}); !strings.HasPrefix(got, "W unschedule") {
		t.Fatalf("fallback = %q", got)
	}
}

func itoa(n int) string { return string(rune('0' + n)) }

// --- P0: KeyFor ---

func TestKeyForHonoursOptsShadowsAndOverrides(t *testing.T) {
	km := New(
		Binding{Action: "refresh", Keys: []string{"r", "ctrl+r"}, Ctx: ctxGlobal, Help: "refresh"},
		Binding{Action: "rename", Keys: []string{"r"}, Ctx: ctxList, Help: "rename"},
	)
	stack := []Ctx{ctxList, ctxGlobal}
	if k, ok := km.KeyFor(stack, "refresh", Opts{}); !ok || k != "ctrl+r" {
		t.Fatalf("KeyFor refresh = %q %v; r is claimed by rename", k, ok)
	}
	off := Opts{Enabled: func(a string) bool { return a != "rename" }}
	if k, _ := km.KeyFor(stack, "refresh", off); k != "r" {
		t.Fatalf("with rename disabled, r reaches refresh again: %q", k)
	}
	if _, ok := km.KeyFor(stack, "rename", off); ok {
		t.Fatal("a disabled action has no key")
	}
	over, err := km.Override(map[string][]string{"refresh": {"F5"}})
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := over.KeyFor(stack, "refresh", Opts{}); k != "F5" {
		t.Fatalf("KeyFor after override = %q", k)
	}
	if _, ok := (Keymap{}).KeyFor(stack, "refresh", Opts{}); ok {
		t.Fatal("zero Keymap binds nothing")
	}
}

// --- P2: ResolveAll ---

func TestResolveAllLetsAHandlerDecline(t *testing.T) {
	km := New(
		Binding{Action: "row.open", Keys: []string{"enter"}, Ctx: ctxList},
		Binding{Action: "row.toggle", Keys: []string{"enter"}, Ctx: ctxList}, // a Duplicate, but ordered
		Binding{Action: "global.open", Keys: []string{"enter"}, Ctx: ctxGlobal},
	)
	stack := []Ctx{ctxList, ctxGlobal}
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	var got []string
	for _, b := range km.ResolveAll(stack, enter, Opts{Enabled: func(a string) bool { return a != "row.toggle" }}) {
		got = append(got, b.Action)
	}
	if !slices.Equal(got, []string{"row.open", "global.open"}) {
		t.Fatalf("ResolveAll = %v", got)
	}
	if bs := km.ResolveAll(stack, runes("x"), Opts{}); len(bs) != 0 {
		t.Fatalf("unbound key = %v", bs)
	}
	// Copies: editing a result never reaches the table.
	bs := km.ResolveAll(stack, enter, Opts{})
	bs[0].Keys[0] = "mutated"
	if km.Bindings()[0].Keys[0] != "enter" {
		t.Fatal("ResolveAll leaked the table")
	}
}

// --- P1: NoPalette ---

func TestNoPaletteKeepsBindingOutOfEntriesOnly(t *testing.T) {
	km := New(
		Binding{Action: "jump", Keys: []string{"1"}, Ctx: ctxList, Help: "jump", NoPalette: true, Footer: 1},
		Binding{Action: "open", Keys: []string{"o"}, Ctx: ctxList, Help: "open"},
	)
	stack := []Ctx{ctxList}
	if es := km.Entries(stack, Opts{}); len(es) != 1 || es[0].Action != "open" {
		t.Fatalf("Entries = %+v", es)
	}
	if len(km.Help(Opts{}, stack)[0].Entries) != 2 {
		t.Fatal("NoPalette bindings still belong in help")
	}
	if _, ok := km.Resolve(stack, runes("1"), Opts{}); !ok {
		t.Fatal("NoPalette bindings still resolve")
	}
}

// --- P1: sections by explicit key, notes, HelpLineCount, marker ---

func TestHelpSectionsKeyedBySection(t *testing.T) {
	km := New(
		Binding{Action: "list.down", Keys: []string{"j"}, Ctx: ctxList, Group: "Move", Section: "list.move", Help: "down"},
		Binding{Action: "modal.down", Keys: []string{"j"}, Ctx: ctxModal, Group: "Move", Section: "modal.move", Help: "scroll"},
		Binding{Action: "list.top", Keys: []string{"g"}, Ctx: ctxList, Group: "Move", Section: "list.move", Help: "top"},
	)
	secs := km.Help(Opts{}, []Ctx{ctxList}, []Ctx{ctxModal})
	if len(secs) != 2 || secs[0].ID != "list.move" || secs[1].ID != "modal.move" ||
		secs[0].Title != "Move" || secs[1].Title != "Move" || len(secs[0].Entries) != 2 {
		t.Fatalf("sections = %+v", secs)
	}
	// Without Section the title is the key, as before.
	plain := testKeymap().Help(Opts{}, []Ctx{ctxList})
	if plain[0].ID != "Move" || plain[0].Entries[0].Section != "Move" {
		t.Fatalf("default section ID = %+v", plain[0])
	}
}

func TestRenderHelpNotesAndLineCount(t *testing.T) {
	secs := testKeymap().Help(Opts{}, []Ctx{ctxList, ctxGlobal})
	secs = append(secs, Section{Title: "Notes", Notes: []string{
		"Filters persist across restarts in ui.json under the state directory.",
		"한글 설명도 셀 너비로 줄바꿈된다.",
	}})
	const w = 24
	n := HelpLineCount(secs, w)
	lines, off := RenderHelp(secs, w, 100, 0, HelpStyle{})
	if len(lines) != n || off != 0 {
		t.Fatalf("HelpLineCount = %d, RenderHelp drew %d", n, len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "  Filters persist") || !strings.Contains(joined, "한글") {
		t.Fatalf("notes missing: %q", lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > w {
			t.Fatalf("line over width: %q", l)
		}
	}
	// Two notes plus title and blank are 4 lines unwrapped; wrapping adds more.
	if n <= HelpLineCount(secs[:len(secs)-1], w)+3 {
		t.Fatalf("notes did not wrap: %d lines", n)
	}
	// Clamping agrees with the count.
	_, off = RenderHelp(secs, w, 5, 1000, HelpStyle{})
	if off != n-5 {
		t.Fatalf("offset clamp %d, want %d", off, n-5)
	}
	if HelpLineCount(secs, 0) != 0 || HelpLineCount(nil, 10) != 0 {
		t.Fatal("empty layouts count 0")
	}
}

func TestRenderHelpMarker(t *testing.T) {
	secs := testKeymap().Help(Opts{}, []Ctx{ctxList, ctxGlobal})
	lines, _ := RenderHelp(secs, 30, 4, 0, HelpStyle{Ellipsis: "~"})
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "~ ") || !strings.HasSuffix(last, " more") {
		t.Fatalf("more line = %q", last)
	}
	t.Setenv("LC_ALL", "ja_JP.UTF-8")
	lines, _ = RenderHelp(secs, 30, 4, 0, HelpStyle{})
	if strings.Contains(strings.Join(lines, ""), "…") {
		t.Fatalf("CJK locale must not draw U+2026: %q", lines)
	}
}

// --- P1: LoadOverrides across tables ---

func TestKnownSplitsSharedOverrides(t *testing.T) {
	main := New(Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal})
	admin := New(Binding{Action: "ban", Keys: []string{"b"}, Ctx: ctxList})
	dir := t.TempDir()
	p := filepath.Join(dir, "keys.yaml")
	if err := os.WriteFile(p, []byte("quit: [x]\nlist.ban: [B]\ntypo: [z]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := main.LoadOverrides(p); err == nil {
		t.Fatal("LoadOverrides stays strict")
	}
	o, err := ReadOverrides(p)
	if err != nil {
		t.Fatal(err)
	}
	mine, unknown := admin.Known(o)
	if !reflect.DeepEqual(mine, map[string][]string{"list.ban": {"B"}}) || !slices.Equal(unknown, []string{"quit", "typo"}) {
		t.Fatalf("Known = %v, %v", mine, unknown)
	}
	admin, err = admin.Override(mine)
	if err != nil || admin.Bindings()[0].Keys[0] != "B" {
		t.Fatalf("admin override = %v %v", admin.Bindings(), err)
	}
	mine["list.ban"][0] = "mutated"
	if o["list.ban"][0] != "B" {
		t.Fatal("Known must copy the key lists")
	}
	if o, err := ReadOverrides(filepath.Join(dir, "missing.yaml")); o != nil || err != nil {
		t.Fatalf("missing file = %v, %v", o, err)
	}
}

func TestConfigPathSub(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	for _, c := range []struct {
		sub  []string
		want string
	}{
		{nil, "/cfg/ghx/keys.yaml"},
		{[]string{"admin"}, "/cfg/ghx/keys-admin.yaml"},
		{[]string{""}, "/cfg/ghx/keys.yaml"},
		{[]string{"a", "b"}, "/cfg/ghx/keys-a-b.yaml"},
	} {
		if got := ConfigPath("ghx", c.sub...); got != c.want {
			t.Errorf("ConfigPath(ghx, %v) = %q", c.sub, got)
		}
	}
}

// --- P1: TypedText ---

func TestTypedText(t *testing.T) {
	for _, c := range []struct {
		k    tea.KeyMsg
		want string
		ok   bool
	}{
		{runes("a"), "a", true},
		{runes("ㅂ"), "ㅂ", true}, // never normalized
		{runes("abc"), "abc", true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("붙여\n넣기"), Paste: true}, "붙여\n넣기", true},
		{tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, " ", true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f"), Alt: true}, "", false},
		{tea.KeyMsg{Type: tea.KeyEnter}, "", false},
		{tea.KeyMsg{Type: tea.KeyCtrlU}, "", false},
	} {
		if got, ok := TypedText(c.k); got != c.want || ok != c.ok {
			t.Errorf("TypedText(%v) = %q %v", c.k, got, ok)
		}
	}
}

// --- value independence for the new reference fields ---

func TestNewFieldsDoNotAlias(t *testing.T) {
	gl := map[string]string{"week": "week"}
	in := Binding{Action: "a", Keys: []string{"h"}, Ctx: ctxList, Help: "h", Footer: 1, FooterGroup: "day", GroupLabels: gl}
	km := New(in)
	gl["week"] = "mutated"
	got := km.Bindings()
	if got[0].GroupLabels["week"] != "week" {
		t.Fatal("New must copy GroupLabels")
	}
	got[0].GroupLabels["week"] = "mutated"
	if km.Bindings()[0].GroupLabels["week"] != "week" {
		t.Fatal("Bindings must copy GroupLabels")
	}
	secs := km.Help(Opts{}, []Ctx{ctxList})
	secs[0].Entries[0].Keys[0] = "mutated"
	if km.Bindings()[0].Keys[0] != "h" {
		t.Fatal("Help must not hand out the table's keys")
	}
}
