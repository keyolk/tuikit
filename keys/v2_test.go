package keys

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

const ctxDetail Ctx = "detail"

// --- gap 1: declared shadows ---

func TestConflictsHonourDeclaredShadow(t *testing.T) {
	base := []Binding{
		{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: ctxGlobal, Pinned: true},
		{Action: "help", Keys: []string{"?"}, Ctx: ctxGlobal, Pinned: true},
	}
	stack := []Ctx{ctxDetail, ctxGlobal}

	declared := New(append(base, Binding{Action: "detail.close", Keys: []string{"esc", "q"}, Ctx: ctxDetail, Shadows: []string{"quit"}})...)
	if cs := declared.Conflicts(stack); len(cs) != 0 {
		t.Fatalf("declared shadow still reported: %v", cs)
	}

	undeclared := New(append(base, Binding{Action: "detail.close", Keys: []string{"esc", "q"}, Ctx: ctxDetail})...)
	if cs := undeclared.Conflicts(stack); len(cs) != 1 || cs[0].Kind != Shadowed {
		t.Fatalf("undeclared shadow = %v; want one Shadowed", cs)
	}

	// Declaring quit does not excuse hiding help.
	other := New(append(base, Binding{Action: "detail.close", Keys: []string{"q", "?"}, Ctx: ctxDetail, Shadows: []string{"quit"}})...)
	if cs := other.Conflicts(stack); len(cs) != 1 || cs[0].Key != "?" {
		t.Fatalf("conflicts = %v; want only ? hiding help", cs)
	}
}

func TestDeclaredShadowSurvivesOverride(t *testing.T) {
	km := New(
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal, Pinned: true},
		Binding{Action: "detail.close", Keys: []string{"esc"}, Ctx: ctxDetail, Shadows: []string{"quit"}},
	)
	km = Must(km.Override(map[string][]string{"quit": {"x"}, "detail.close": {"x"}}))
	if cs := km.Conflicts([]Ctx{ctxDetail, ctxGlobal}); len(cs) != 0 {
		t.Fatalf("shadow declared by action must follow rebinding: %v", cs)
	}
}

// --- gap 2: state-dependent labels ---

func groupKeymap() Keymap {
	return New(
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal, Help: "quit", Pinned: true},
		Binding{Action: "group", Keys: []string{"t"}, Ctx: ctxList, Help: "group",
			Labels: map[string]string{"on": "ungroup"}, Footer: 1},
	)
}

func TestLabelsFollowStateOnEverySurface(t *testing.T) {
	km := groupKeymap()
	stack := []Ctx{ctxList, ctxGlobal}
	grouped := false
	o := Opts{State: func(a string) string {
		if a == "group" && grouped {
			return "on"
		}
		return ""
	}}
	if got := km.Footer(stack, 80, o, FooterStyle{}); !strings.HasPrefix(got, "t group ") {
		t.Fatalf("footer = %q", got)
	}
	grouped = true
	if got := km.Footer(stack, 80, o, FooterStyle{}); !strings.HasPrefix(got, "t ungroup ") {
		t.Fatalf("footer = %q", got)
	}
	if e := km.Help(o, stack)[0].Entries[0]; e.Help != "ungroup" {
		t.Fatalf("help entry = %+v", e)
	}
	if es := km.Entries(stack, o); es[0].Help != "ungroup" {
		t.Fatalf("palette entry = %+v", es[0])
	}
	if got := km.Footer(stack, 80, Opts{}, FooterStyle{}); !strings.HasPrefix(got, "t group ") {
		t.Fatalf("nil State must show Help, footer = %q", got)
	}
	if cs := km.Conflicts(stack); len(cs) != 0 {
		t.Fatalf("one binding with labels is not a conflict: %v", cs)
	}
	if (Binding{Help: "h", Labels: map[string]string{"x": "y"}}).Label(Opts{State: func(string) string { return "nope" }}) != "h" {
		t.Fatal("an unlabelled state shows Help")
	}
}

// --- gap 3: key display names ---

func TestNames(t *testing.T) {
	var words Names
	glyphs := Names{Glyphs: true, Caret: true}
	cases := []struct {
		n         Names
		key, want string
	}{
		{words, " ", "space"},
		{words, "enter", "enter"},
		{words, "ctrl+x", "ctrl+x"},
		{glyphs, "enter", "↵"},
		{glyphs, "up", "↑"},
		{glyphs, " ", "space"},
		{glyphs, "ctrl+x", "^x"},
		{glyphs, "q", "q"},
		{words.With("enter", "RET"), "enter", "RET"},
		{glyphs.With(" ", "␣"), " ", "␣"},
	}
	for _, c := range cases {
		if got := c.n.Name(c.key); got != c.want {
			t.Errorf("%+v.Name(%q) = %q, want %q", c.n, c.key, got, c.want)
		}
	}
	if got := glyphs.Join([]string{"up", "down"}); got != "↑↓" {
		t.Errorf("Join(up,down) = %q", got)
	}
	if got := words.Join([]string{"h", "j", "k", "l"}); got != "hjkl" {
		t.Errorf("Join(hjkl) = %q", got)
	}
	if got := words.Join([]string{"enter", "o"}); got != "enter/o" {
		t.Errorf("Join(enter,o) = %q", got)
	}
}

func TestNamesWithDoesNotAliasCopies(t *testing.T) {
	a := Names{}.With("x", "1")
	b := a
	b = b.With("x", "2")
	c := a.With("y", "3")
	if a.Name("x") != "1" || b.Name("x") != "2" || a.Name("y") != "y" || c.Name("y") != "3" {
		t.Fatalf("With leaked between copies: a.x=%q b.x=%q a.y=%q", a.Name("x"), b.Name("x"), a.Name("y"))
	}
}

func TestNamesUsedByFooterAndHelp(t *testing.T) {
	km := New(
		Binding{Action: "open", Keys: []string{" ", "enter"}, Ctx: ctxList, Help: "open", Footer: 1},
		Binding{Action: "quit", Keys: []string{"ctrl+c"}, Ctx: ctxGlobal, Help: "quit", Pinned: true},
	)
	stack := []Ctx{ctxList, ctxGlobal}
	if got := km.Footer(stack, 80, Opts{}, FooterStyle{}); got != "space open  ctrl+c quit" {
		t.Fatalf("footer = %q", got)
	}
	got := km.Footer(stack, 80, Opts{}, FooterStyle{Names: Names{Caret: true}})
	if got != "space open  ^c quit" {
		t.Fatalf("caret footer = %q", got)
	}
	lines := rh(km.Help(Opts{}, stack), 40, 20, 0, HelpStyle{Names: Names{Glyphs: true}})
	if !strings.Contains(strings.Join(lines, "\n"), "space/↵  open") {
		t.Fatalf("help = %q", lines)
	}
}

// --- gap 4: grouped footer hints ---

func moveKeymap() Keymap {
	return New(
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal, Help: "quit", Pinned: true},
		Binding{Action: "left", Keys: []string{"h", "left"}, Ctx: ctxList, Help: "left", Footer: 3, FooterGroup: "move"},
		Binding{Action: "down", Keys: []string{"j", "down"}, Ctx: ctxList, Help: "down", Footer: 2, FooterGroup: "move"},
		Binding{Action: "up", Keys: []string{"k", "up"}, Ctx: ctxList, Help: "up", Footer: 3, FooterGroup: "move"},
		Binding{Action: "right", Keys: []string{"l", "right"}, Ctx: ctxList, Help: "right", Footer: 3, FooterGroup: "move"},
		Binding{Action: "open", Keys: []string{"enter"}, Ctx: ctxList, Help: "open", Footer: 1},
		Binding{Action: "del", Keys: []string{"d"}, Ctx: ctxList, Help: "delete", Footer: 5},
	)
}

func TestFooterGroupCollapsesHints(t *testing.T) {
	km := moveKeymap()
	stack := []Ctx{ctxList, ctxGlobal}
	if got := km.Footer(stack, 80, Opts{}, FooterStyle{}); got != "enter open  hjkl move  d delete  q quit" {
		t.Fatalf("footer = %q", got)
	}
	// The group takes its best member's priority, so it outlives delete.
	if got := km.Footer(stack, 32, Opts{}, FooterStyle{}); got != "enter open  hjkl move  …  q quit" {
		t.Fatalf("narrow footer = %q", got)
	}
	noUp := Opts{Enabled: func(a string) bool { return a != "up" }}
	if got := km.Footer(stack, 80, noUp, FooterStyle{}); !strings.Contains(got, "hjl move") {
		t.Fatalf("disabled member must leave the group, footer = %q", got)
	}
	// Help still documents every member on its own.
	if n := len(km.Help(Opts{}, stack)[0].Entries); n != 6 {
		t.Fatalf("help lists %d list entries, want 6", n)
	}
}

// --- gap 5: footer visibility separate from runnable ---

func TestVisibleNarrowsFooterOnly(t *testing.T) {
	km := New(
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal, Help: "quit", Pinned: true},
		Binding{Action: "refresh", Keys: []string{"r"}, Ctx: ctxGlobal, Help: "refresh", Footer: 2},
		Binding{Action: "list.reload", Keys: []string{"r"}, Ctx: ctxList, Help: "reload", Footer: 1},
	)
	stack := []Ctx{ctxList, ctxGlobal}
	o := Opts{Visible: func(a string) bool { return a != "list.reload" }}
	got := km.Footer(stack, 80, o, FooterStyle{})
	if strings.Contains(got, "reload") || strings.Contains(got, "refresh") {
		t.Fatalf("footer = %q; a hidden binding still owns r, so refresh must not be offered on it", got)
	}
	if b, ok := km.Resolve(stack, runes("r"), o); !ok || b.Action != "list.reload" {
		t.Fatalf("hidden binding must still run: %q %v", b.Action, ok)
	}
	if es := km.Entries(stack, o); len(es) != 2 || es[0].Action != "list.reload" {
		t.Fatalf("palette ignores Visible: %+v", es)
	}
	// Enabled is still applied without being repeated in Visible.
	o.Enabled = func(a string) bool { return a != "list.reload" }
	o.Visible = func(string) bool { return true }
	if got := km.Footer(stack, 80, o, FooterStyle{}); !strings.Contains(got, "r refresh") {
		t.Fatalf("footer = %q; disabled reload must free r for refresh", got)
	}
}

// --- gap 6: zero Keymap ---

func TestZeroKeymapIsSafe(t *testing.T) {
	var km Keymap
	stack := []Ctx{ctxList, ctxGlobal}
	if !km.IsZero() || testKeymap().IsZero() || New().IsZero() {
		t.Fatal("IsZero is true only for the zero value")
	}
	if _, ok := km.Resolve(stack, runes("q"), Opts{}); ok {
		t.Fatal("zero keymap resolved a key")
	}
	if _, ok := km.ResolveRaw(stack, tea.KeyMsg{Type: tea.KeyEnter}, Opts{}); ok {
		t.Fatal("zero keymap resolved a key")
	}
	if km.Footer(stack, 80, Opts{}, FooterStyle{}) != "" || len(km.Help(Opts{}, stack)) != 0 ||
		len(km.Entries(stack, Opts{})) != 0 || len(km.Conflicts(stack)) != 0 || len(km.Bindings()) != 0 {
		t.Fatal("zero keymap surfaces must be empty")
	}
	if same, err := km.Override(nil); err != nil || !same.IsZero() {
		t.Fatalf("empty override on zero keymap: %v", err)
	}
	if _, err := km.Override(map[string][]string{"quit": {"x"}}); err == nil {
		t.Fatal("zero keymap knows no actions")
	}
	if b, ok := km.Or(testKeymap()).Resolve(stack, runes("q"), Opts{}); !ok || b.Action != "quit" {
		t.Fatal("Or must fall back for the zero keymap")
	}
	if b, _ := groupKeymap().Or(testKeymap()).Resolve(stack, runes("t"), Opts{}); b.Action != "group" {
		t.Fatal("Or must keep a built keymap")
	}
}

func TestMustPanicsOnError(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Must did not panic")
		}
	}()
	Must(testKeymap().Override(map[string][]string{"nope": {"x"}}))
}

// --- gap 8 (keys half): ConfigPath ---

func TestConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if got := ConfigPath("brd"); got != filepath.Join(dir, "brd", "keys.yaml") {
		t.Fatalf("ConfigPath = %q", got)
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	if got := ConfigPath("brd"); got != filepath.Join(home, ".config", "brd", "keys.yaml") {
		t.Fatalf("ConfigPath fallback = %q", got)
	}
}

// --- gap 9: help across stacks, styled ---

func TestHelpMergesStacks(t *testing.T) {
	km := New(
		Binding{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: ctxGlobal, Group: "App", Help: "quit", Pinned: true},
		Binding{Action: "open", Keys: []string{"enter"}, Ctx: ctxList, Group: "List", Help: "open"},
		Binding{Action: "detail.close", Keys: []string{"esc", "q"}, Ctx: ctxDetail, Group: "Detail", Help: "close"},
		Binding{Action: "detail.copy", Keys: []string{"y"}, Ctx: ctxDetail, Group: "App", Help: "copy"},
	)
	// The detail stack is listed second: its q hides quit there, but the
	// board stack reaches quit by q, so the merged entry keeps both keys.
	secs := km.Help(Opts{}, []Ctx{ctxDetail, ctxGlobal}, []Ctx{ctxList, ctxGlobal})
	var titles []string
	count := map[string]int{}
	var quitKeys string
	for _, s := range secs {
		titles = append(titles, s.Title)
		for _, e := range s.Entries {
			count[e.Action]++
			if e.Action == "quit" {
				quitKeys = strings.Join(e.Keys, "/")
			}
		}
	}
	if strings.Join(titles, ",") != "Detail,App,List" {
		t.Fatalf("sections = %v; want Detail first and App merged", titles)
	}
	for a, n := range count {
		if n != 1 {
			t.Errorf("%s listed %d times", a, n)
		}
	}
	if quitKeys != "q/ctrl+c" {
		t.Fatalf("quit keys = %q; want the union in declaration order", quitKeys)
	}
	if len(km.Help(Opts{})) != 0 {
		t.Fatal("no stacks, no help")
	}
}

func TestRenderHelpStyled(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	st := HelpStyle{
		Title: r.NewStyle().Bold(true),
		Key:   r.NewStyle().Foreground(lipgloss.Color("4")),
		Desc:  r.NewStyle().Italic(true),
		Muted: r.NewStyle().Faint(true),
	}
	secs := testKeymap().Help(Opts{}, []Ctx{ctxList, ctxGlobal})
	plain := rh(secs, 40, 5, 0, HelpStyle{})
	styled := rh(secs, 40, 5, 0, st)
	if len(plain) != len(styled) {
		t.Fatalf("styling changed the line count: %d vs %d", len(plain), len(styled))
	}
	for i := range plain {
		if ansi.Strip(styled[i]) != plain[i] {
			t.Errorf("line %d: %q vs %q", i, ansi.Strip(styled[i]), plain[i])
		}
	}
	if styled[0] == plain[0] || styled[4] == plain[4] {
		t.Fatalf("title and more line must be styled: %q %q", styled[0], styled[4])
	}
	for w := 1; w <= 40; w++ {
		for _, l := range rh(secs, w, 50, 0, st) {
			if ansi.StringWidth(l) > w {
				t.Fatalf("width %d: %q is %d cells", w, l, ansi.StringWidth(l))
			}
		}
	}
}

// --- value semantics of the new reference fields ---

func TestBindingReferenceFieldsDoNotAlias(t *testing.T) {
	labels := map[string]string{"on": "ungroup"}
	shadows := []string{"quit"}
	km := New(Binding{Action: "group", Keys: []string{"t"}, Ctx: ctxList, Help: "group", Labels: labels, Shadows: shadows})
	labels["on"] = "changed"
	shadows[0] = "changed"

	check := func(when string) {
		t.Helper()
		b := km.Bindings()[0]
		if b.Labels["on"] != "ungroup" || b.Shadows[0] != "quit" {
			t.Fatalf("%s: table changed: %+v", when, b)
		}
	}
	check("after mutating New's input")

	bs := km.Bindings()
	bs[0].Labels["on"] = "x"
	bs[0].Shadows[0] = "x"
	check("after mutating Bindings output")

	b, _ := km.Resolve([]Ctx{ctxList}, runes("t"), Opts{})
	b.Labels["on"] = "x"
	b.Keys[0] = "x"
	check("after mutating Resolve output")

	cp := km
	cp, _ = cp.Override(map[string][]string{"group": {"g"}})
	if _, ok := km.Resolve([]Ctx{ctxList}, runes("t"), Opts{}); !ok {
		t.Fatal("overriding a copy changed the original")
	}
}
