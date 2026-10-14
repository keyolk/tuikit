package keys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func quitTwiceKeymap() Keymap {
	return New(
		Binding{Action: "app.quit", Keys: []string{"q"}, Ctx: ctxList, Help: "quit"},
		Binding{Action: "app.quit", Keys: []string{"ctrl+c"}, Ctx: ctxGlobal, Help: "quit", Pinned: true},
		Binding{Action: "help", Keys: []string{"?"}, Ctx: ctxGlobal, Help: "help", Pinned: true},
		Binding{Action: "list.open", Keys: []string{"enter"}, Ctx: ctxList, Help: "open", Footer: 1},
	)
}

// ccproxy: the same action in two contexts must not lose its pinned hint
// behind the unpinned inner binding.
func TestActionInTwoContextsKeepsPinnedHint(t *testing.T) {
	km := quitTwiceKeymap()
	stack := []Ctx{ctxList, ctxGlobal}
	got := km.Footer(stack, 80, Opts{}, FooterStyle{})
	if got != "enter open  q quit  ? help" {
		t.Fatalf("footer = %q; quit is pinned through its global binding", got)
	}
	// Narrow: the merged quit hint survives like any pinned one.
	if got := km.Footer(stack, 14, Opts{}, FooterStyle{}); !strings.Contains(got, "q quit") {
		t.Fatalf("narrow footer = %q", got)
	}
	secs := km.Help(Opts{}, stack)
	n := 0
	for _, s := range secs {
		for _, e := range s.Entries {
			if e.Action == "app.quit" {
				n++
				if strings.Join(e.Keys, "/") != "q/ctrl+c" {
					t.Fatalf("quit keys = %v", e.Keys)
				}
			}
		}
	}
	if n != 1 {
		t.Fatalf("app.quit listed %d times", n)
	}
	if es := km.Entries(stack, Opts{}); len(es) != 3 {
		t.Fatalf("palette = %+v; one entry per action", es)
	}
}

func TestOverridePerContext(t *testing.T) {
	km := quitTwiceKeymap()
	stack := []Ctx{ctxList, ctxGlobal}

	// Plain: every context.
	all := Must(km.Override(map[string][]string{"app.quit": {"x"}}))
	if _, ok := all.Resolve(stack, ctrl('c'), Opts{}); ok {
		t.Fatal("a plain override rebinds every context")
	}

	// Qualified: one context only; ctrl+c survives.
	one := Must(km.Override(map[string][]string{"list.app.quit": {"x"}}))
	if b, ok := one.Resolve(stack, runes("x"), Opts{}); !ok || b.Action != "app.quit" {
		t.Fatal("qualified override did not bind x in list")
	}
	if b, ok := one.Resolve(stack, ctrl('c'), Opts{}); !ok || b.Action != "app.quit" {
		t.Fatal("qualified override must leave the global binding alone")
	}

	// Qualified beats plain for its context.
	mixed := Must(km.Override(map[string][]string{"app.quit": {"x"}, "global.app.quit": {"ctrl+c", "ctrl+d"}}))
	if _, ok := mixed.Resolve([]Ctx{ctxGlobal}, ctrl('c'), Opts{}); !ok {
		t.Fatal("the qualified global entry must win over the plain one")
	}
	if b, _ := mixed.Resolve([]Ctx{ctxList}, runes("x"), Opts{}); b.Action != "app.quit" {
		t.Fatal("the plain entry still applies to list")
	}

	// "list.open" is an action; read as context "list" + action "open" it
	// matches nothing, so it is plain.
	if _, err := km.Override(map[string][]string{"list.open": {"o"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := km.Override(map[string][]string{"modal.app.quit": {"x"}}); err == nil {
		t.Fatal("a context the action is not bound in is unknown")
	}
}

func TestOverrideAmbiguousName(t *testing.T) {
	km := New(
		Binding{Action: "list.open", Keys: []string{"enter"}, Ctx: ctxGlobal},
		Binding{Action: "open", Keys: []string{"o"}, Ctx: ctxList},
	)
	_, err := km.Override(map[string][]string{"list.open": {"x"}})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
}

func TestOverrideRejectsNewDuplicates(t *testing.T) {
	km := quitTwiceKeymap()
	_, err := km.Override(map[string][]string{"list.open": {"q"}})
	if err == nil || !strings.Contains(err.Error(), `duplicate: "q" in list`) {
		t.Fatalf("err = %v; want the new duplicate listed", err)
	}
	if b, _ := km.Resolve([]Ctx{ctxList}, runes("q"), Opts{}); b.Action != "app.quit" {
		t.Fatal("a rejected override must leave km unchanged")
	}
	// A duplicate across contexts is a shadow, not a duplicate.
	if _, err := km.Override(map[string][]string{"list.open": {"?"}}); err != nil {
		t.Fatalf("inner-over-outer is allowed: %v", err)
	}
	// A duplicate the app already had is not blamed on the user.
	dup := New(
		Binding{Action: "a", Keys: []string{"x"}, Ctx: ctxList},
		Binding{Action: "b", Keys: []string{"x"}, Ctx: ctxList},
		Binding{Action: "c", Keys: []string{"y"}, Ctx: ctxList},
	)
	if _, err := dup.Override(map[string][]string{"c": {"z"}}); err != nil {
		t.Fatalf("pre-existing duplicate blamed on the override: %v", err)
	}

	path := filepath.Join(t.TempDir(), "keys.yaml")
	if err := os.WriteFile(path, []byte("list.open: [q]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := km.LoadOverrides(path); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("LoadOverrides err = %v", err)
	}
}

// --- ccproxy double-checks: short labels, key pairs, action column ---

func TestShortLabelInFooterOnly(t *testing.T) {
	km := New(
		Binding{Action: "lens", Keys: []string{"L"}, Ctx: ctxList, Help: "toggle request timing", Short: "timing",
			Labels: map[string]string{"on": "timing off"}, Footer: 1},
	)
	stack := []Ctx{ctxList}
	if got := km.Footer(stack, 80, Opts{}, FooterStyle{}); got != "L timing" {
		t.Fatalf("footer = %q", got)
	}
	on := Opts{State: func(string) string { return "on" }}
	if got := km.Footer(stack, 80, on, FooterStyle{}); got != "L timing off" {
		t.Fatalf("state label must beat Short: %q", got)
	}
	if e := km.Help(Opts{}, stack)[0].Entries[0]; e.Help != "toggle request timing" {
		t.Fatalf("help keeps the long label: %+v", e)
	}
}

func TestGroupSeparator(t *testing.T) {
	km := New(
		Binding{Action: "down", Keys: []string{"j"}, Ctx: ctxList, Help: "down", Footer: 1, FooterGroup: "scroll"},
		Binding{Action: "up", Keys: []string{"k"}, Ctx: ctxList, Help: "up", Footer: 1, FooterGroup: "scroll"},
	)
	if got := km.Footer([]Ctx{ctxList}, 80, Opts{}, FooterStyle{GroupSeparator: "/"}); got != "j/k scroll" {
		t.Fatalf("footer = %q", got)
	}
}

func TestTableWithActionColumn(t *testing.T) {
	km := New(
		Binding{Action: "quit", Keys: []string{"q"}, Ctx: ctxGlobal, Help: "quit"},
		Binding{Action: "modal.close", Keys: []string{"q"}, Ctx: ctxModal, Help: "close"},
	)
	secs := km.Table(Opts{})
	if len(secs) != 2 {
		t.Fatalf("table sections = %+v; shadowing must not hide modal or global", secs)
	}
	lines := rh(secs, 40, 20, 0, HelpStyle{Actions: true})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "  q  close ") || !strings.HasSuffix(lines[4], " modal.close") || !strings.HasSuffix(lines[1], " quit") {
		t.Fatalf("table =\n%s", joined)
	}
	for w := 1; w <= 40; w++ {
		for _, l := range rh(secs, w, 20, 0, HelpStyle{Actions: true}) {
			if len([]rune(l)) > w {
				t.Fatalf("width %d: %q", w, l)
			}
		}
	}
}

// ctrl builds the key message for ctrl+<r>; tea numbers ctrl+a..ctrl+z from 1.
func ctrl(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyType(r - 'a' + 1)} }

func TestOverrideRejectsNewPinnedShadow(t *testing.T) {
	km := New(
		Binding{Action: "app.help", Keys: []string{"?"}, Ctx: ctxGlobal, Help: "help", Pinned: true},
		Binding{Action: "down", Keys: []string{"j"}, Ctx: ctxList, Help: "down", Footer: 1},
		Binding{Action: "modal.close", Keys: []string{"esc"}, Ctx: ctxModal, Help: "close"},
	)
	stacks := [][]Ctx{{ctxList, ctxGlobal}, {ctxModal, ctxGlobal}}
	o := map[string][]string{"app.help": {"j"}}

	_, err := km.Override(o, stacks...)
	if err == nil || !strings.Contains(err.Error(), `shadowed: "j" in list (down) hides pinned app.help`) {
		t.Fatalf("err = %v; want the new pinned shadow listed", err)
	}
	if b, _ := km.Resolve([]Ctx{ctxGlobal}, runes("?"), Opts{}); b.Action != "app.help" {
		t.Fatal("a rejected override must leave km unchanged")
	}
	// Without stacks the shadow cannot be judged, as for Conflicts.
	if _, err := km.Override(o); err != nil {
		t.Fatalf("no stacks: %v", err)
	}
	// Only stacks where the contexts meet count: j is free in the modal.
	if _, err := km.Override(o, []Ctx{ctxModal, ctxGlobal}); err != nil {
		t.Fatalf("modal stack: %v", err)
	}
	// A declared shadow is still allowed after an override.
	decl := New(
		Binding{Action: "app.help", Keys: []string{"?"}, Ctx: ctxGlobal, Pinned: true},
		Binding{Action: "down", Keys: []string{"j"}, Ctx: ctxList, Shadows: []string{"app.help"}},
	)
	if _, err := decl.Override(o, stacks...); err != nil {
		t.Fatalf("declared shadow: %v", err)
	}

	path := filepath.Join(t.TempDir(), "keys.yaml")
	if err := os.WriteFile(path, []byte("app.help: [j]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := km.LoadOverrides(path, stacks...); err == nil || !strings.Contains(err.Error(), "shadowed") {
		t.Fatalf("LoadOverrides err = %v", err)
	}
}
