package keys

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	ctxGlobal Ctx = "global"
	ctxList   Ctx = "list"
	ctxModal  Ctx = "modal"
	ctxInput  Ctx = "input"
)

func testKeymap() Keymap {
	return New(
		Binding{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: ctxGlobal, Help: "quit", Pinned: true, Footer: 2},
		Binding{Action: "help", Keys: []string{"?"}, Ctx: ctxGlobal, Help: "help", Pinned: true, Footer: 1},
		Binding{Action: "refresh", Keys: []string{"r"}, Ctx: ctxGlobal, Help: "refresh", Footer: 9},
		Binding{Action: "list.down", Keys: []string{"j", "down"}, Ctx: ctxList, Group: "Move", Help: "down", Footer: 1},
		Binding{Action: "list.up", Keys: []string{"k", "up"}, Ctx: ctxList, Group: "Move", Help: "up", Footer: 1},
		Binding{Action: "list.open", Keys: []string{"enter", "l"}, Ctx: ctxList, Group: "Act", Help: "open", Footer: 2},
		Binding{Action: "list.delete", Keys: []string{"d"}, Ctx: ctxList, Group: "Act", Help: "delete", Footer: 5},
		Binding{Action: "modal.close", Keys: []string{"esc", "q"}, Ctx: ctxModal, Help: "close", Footer: 1},
	)
}

func TestResolveInnermostContextWins(t *testing.T) {
	km := testKeymap()
	b, ok := km.Resolve([]Ctx{ctxModal, ctxList, ctxGlobal}, runes("q"), Opts{})
	if !ok || b.Action != "modal.close" {
		t.Fatalf("q in modal = %q, %v; want modal.close", b.Action, ok)
	}
	b, ok = km.Resolve([]Ctx{ctxList, ctxGlobal}, runes("q"), Opts{})
	if !ok || b.Action != "quit" {
		t.Fatalf("q in list = %q, %v; want quit from the outer context", b.Action, ok)
	}
}

func TestResolveNormalizesHangul(t *testing.T) {
	km := testKeymap()
	// ㅓ is the physical j key.
	b, ok := km.Resolve([]Ctx{ctxList, ctxGlobal}, runes("ㅓ"), Opts{})
	if !ok || b.Action != "list.down" {
		t.Fatalf("ㅓ = %q, %v; want list.down", b.Action, ok)
	}
}

func TestResolveRawKeepsHangulForTextInput(t *testing.T) {
	km := testKeymap()
	if b, ok := km.ResolveRaw([]Ctx{ctxInput, ctxList, ctxGlobal}, runes("ㅂ"), Opts{}); ok {
		t.Fatalf("raw ㅂ resolved to %q; a text field must receive the jamo", b.Action)
	}
}

func TestResolveNamedKeys(t *testing.T) {
	km := testKeymap()
	b, ok := km.Resolve([]Ctx{ctxList}, tea.KeyMsg{Type: tea.KeyEnter}, Opts{})
	if !ok || b.Action != "list.open" {
		t.Fatalf("enter = %q, %v", b.Action, ok)
	}
	b, ok = km.Resolve([]Ctx{ctxList, ctxGlobal}, tea.KeyMsg{Type: tea.KeyCtrlC}, Opts{})
	if !ok || b.Action != "quit" {
		t.Fatalf("ctrl+c = %q, %v", b.Action, ok)
	}
}

func TestResolveDisabledFallsThroughToOuterContext(t *testing.T) {
	km := testKeymap()
	off := func(a string) bool { return a != "modal.close" }
	b, ok := km.Resolve([]Ctx{ctxModal, ctxGlobal}, runes("q"), Opts{Enabled: off})
	if !ok || b.Action != "quit" {
		t.Fatalf("disabled modal.close should fall through to quit, got %q, %v", b.Action, ok)
	}
	if _, ok := km.Resolve([]Ctx{ctxModal}, tea.KeyMsg{Type: tea.KeyEsc}, Opts{Enabled: off}); ok {
		t.Fatal("a disabled binding with no outer match must resolve to nothing")
	}
}

func TestResolveUnboundKey(t *testing.T) {
	if _, ok := testKeymap().Resolve([]Ctx{ctxList, ctxGlobal}, runes("z"), Opts{}); ok {
		t.Fatal("z is unbound")
	}
}

func TestNewCopiesCallerSlices(t *testing.T) {
	keysIn := []string{"x"}
	km := New(Binding{Action: "a", Keys: keysIn, Ctx: ctxGlobal})
	keysIn[0] = "y"
	if _, ok := km.Resolve([]Ctx{ctxGlobal}, runes("x"), Opts{}); !ok {
		t.Fatal("mutating the caller's slice changed the keymap")
	}
	bs := km.Bindings()
	bs[0].Keys[0] = "z"
	if _, ok := km.Resolve([]Ctx{ctxGlobal}, runes("x"), Opts{}); !ok {
		t.Fatal("mutating Bindings() output changed the keymap")
	}
}

func TestOverrideReturnsNewKeymapAndLeavesOriginal(t *testing.T) {
	km := testKeymap()
	km2, err := km.Override(map[string][]string{"list.delete": {"x"}})
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := km2.Resolve([]Ctx{ctxList}, runes("x"), Opts{}); !ok || b.Action != "list.delete" {
		t.Fatal("override did not bind x")
	}
	if _, ok := km2.Resolve([]Ctx{ctxList}, runes("d"), Opts{}); ok {
		t.Fatal("override must replace, not add")
	}
	if b, ok := km.Resolve([]Ctx{ctxList}, runes("d"), Opts{}); !ok || b.Action != "list.delete" {
		t.Fatal("the original keymap changed")
	}
}

func TestOverrideUnknownActionsListsThem(t *testing.T) {
	km := testKeymap()
	_, err := km.Override(map[string][]string{"nope": {"x"}, "also.nope": {"y"}, "quit": {"Q"}})
	if err == nil || !strings.Contains(err.Error(), "also.nope, nope") {
		t.Fatalf("err = %v; want both unknown actions, sorted", err)
	}
}

func TestLoadOverrides(t *testing.T) {
	dir := t.TempDir()
	km := testKeymap()

	same, err := km.LoadOverrides(filepath.Join(dir, "missing.yaml"))
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if _, ok := same.Resolve([]Ctx{ctxList}, runes("d"), Opts{}); !ok {
		t.Fatal("missing file must leave the keymap as is")
	}

	good := filepath.Join(dir, "keys.yaml")
	if err := os.WriteFile(good, []byte("list.delete: [x, delete]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	km2, err := km.LoadOverrides(good)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := km2.Resolve([]Ctx{ctxList}, tea.KeyMsg{Type: tea.KeyDelete}, Opts{}); !ok || b.Action != "list.delete" {
		t.Fatal("yaml override did not apply")
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("nope: [x]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := km.LoadOverrides(bad); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unknown action in file: err = %v", err)
	}

	broken := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(broken, []byte("list.delete: {"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := km.LoadOverrides(broken); err == nil {
		t.Fatal("malformed yaml must be an error")
	}
}

func TestBindingKey(t *testing.T) {
	if (Binding{}).Key() != "" || (Binding{Keys: []string{"a", "b"}}).Key() != "a" {
		t.Fatal("Key returns the first key or empty")
	}
}
