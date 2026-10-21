package surface

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func buttons(bs ...string) string {
	ids := make([]string, len(bs))
	for i := range bs {
		ids[i] = `"b` + string(rune('0'+i)) + `"`
	}
	out := `{"id":"root","component":"Column","children":["list",` + strings.Join(ids, ",") + `]},` +
		`{"id":"list","component":"List","children":{"path":"/items","componentId":"it"},"itemKey":"id","selected":{"path":"/sel"}},` +
		`{"id":"it","component":"Text","text":{"path":"name"}}`
	for i, b := range bs {
		out += `,{"id":"b` + string(rune('0'+i)) + `","component":"Button",` + b + `}`
	}
	return out
}

func quits(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

func TestDeclaredKeyIsBoundAndShown(t *testing.T) {
	h := newHarness(t, 80, 24)
	load(h, buttons(`"label":"Deploy now","key":"d","action":{"event":{"name":"deploy","context":{"item":{"path":"/sel"}}}}`))
	if _, footer := h.row("? help"); !strings.Contains(footer, "d Deploy now") {
		t.Errorf("footer lacks the agent action: %q", footer)
	}
	h.key("?")
	if !h.has("Deploy now") || !h.has("Actions") {
		t.Errorf("help lacks the agent action:\n%s", h.plain())
	}
	h.key("esc")
	h.key("d")
	if len(h.events) != 1 || h.events[0].Action.Name != "deploy" || h.events[0].Action.Context["item"] != "a" {
		t.Fatalf("d must emit deploy with the selected item, got %+v", h.events)
	}
}

func TestAgentCannotTakeBuiltinKeys(t *testing.T) {
	for _, k := range []string{"q", "j", "k", "o", "g", "G", "ctrl+c", "?", ":", "tab", "enter"} {
		t.Run(k, func(t *testing.T) {
			h := newHarness(t, 80, 24)
			load(h, buttons(`"label":"Grab","key":"`+k+`","action":{"event":{"name":"grab"}}`))
			if h.m.ui.bound["b0"] != "" {
				t.Fatalf("agent bound built-in key %q", k)
			}
			if !h.has("action b0:") {
				t.Errorf("clash not toasted:\n%s", h.plain())
			}
			h.key(k)
			if len(h.events) != 0 {
				t.Fatalf("%q ran the agent action: %+v", k, h.events)
			}
			// Still reachable from the palette.
			h.key("esc")
			h.key(":")
			for _, r := range "grab" {
				h.key(string(r))
			}
			h.key("enter")
			if len(h.events) != 1 || h.events[0].Action.Name != "grab" {
				t.Fatalf("palette did not run the action: %+v\n%s", h.events, h.plain())
			}
		})
	}
}

func TestQuitAndCtrlCStayBuiltIn(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		h := newHarness(t, 80, 24)
		load(h, buttons(`"label":"Quit?","key":"`+k+`","action":{"event":{"name":"steal"}}`))
		h.key(k)
		if !quits(h.run()) {
			t.Errorf("%s no longer quits", k)
		}
	}
	// ctrl+c quits even while typing in a field.
	h := newHarness(t, 80, 24)
	load(h, `{"id":"root","component":"Column","children":["l","in"]},{"id":"l","component":"List","children":["x"]},{"id":"x","component":"Text","text":"x"},{"id":"in","component":"Input","value":{"path":"/v"}}`)
	h.key("tab")
	h.key("q")
	if got, _ := lookup(h.m.st.surfaces["s"].Data, "/v"); got != "q" {
		t.Fatalf("q must be text in a field, data is %v", got)
	}
	h.key("ctrl+c")
	if !quits(h.run()) {
		t.Error("ctrl+c does not quit from a field")
	}
}

func TestDuplicateAgentKeysFirstWins(t *testing.T) {
	h := newHarness(t, 80, 24)
	load(h, buttons(`"label":"One","key":"x","action":{"event":{"name":"one"}}`, `"label":"Two","key":"x","action":{"event":{"name":"two"}}`))
	if h.m.ui.bound["b0"] != "x" || h.m.ui.bound["b1"] != "" {
		t.Fatalf("bound = %v", h.m.ui.bound)
	}
	h.key("x")
	if len(h.events) != 1 || h.events[0].Action.Name != "one" {
		t.Fatalf("x ran %+v", h.events)
	}
}

func TestBadKeyShapeFallsBackToPalette(t *testing.T) {
	h := newHarness(t, 80, 24)
	load(h, buttons(`"label":"Jamo","key":"ㅁ","action":{"event":{"name":"j"}}`, `"label":"Chord","key":"ctrl+x ctrl+s","action":{"event":{"name":"c"}}`))
	if h.m.ui.bound["b0"] != "" || h.m.ui.bound["b1"] != "" {
		t.Fatalf("bound = %v", h.m.ui.bound)
	}
	if !h.has("[:] Jamo") {
		t.Errorf("palette-only button must say so:\n%s", h.plain())
	}
}

func TestDangerAlwaysConfirms(t *testing.T) {
	for name, extra := range map[string]string{"danger": `"danger":true`, "danger confirm:false": `"danger":true,"confirm":false`, "confirm": `"confirm":true`} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, 80, 24)
			load(h, buttons(`"label":"Drop","key":"x",`+extra+`,"action":{"event":{"name":"drop","context":{"item":{"path":"/sel"}}}}`))
			h.key("x")
			if len(h.events) != 0 || !h.has(`"Drop"?`) {
				t.Fatalf("no confirm overlay, or event before confirm: %+v\n%s", h.events, h.plain())
			}
			h.key("enter") // only y confirms
			h.key("n")
			if len(h.events) != 0 || h.has(`"Drop"?`) {
				t.Fatalf("cancel did not cancel: %+v", h.events)
			}
			h.key("x")
			h.key("y")
			if len(h.events) != 1 || h.events[0].Action.Name != "drop" {
				t.Fatalf("y must emit exactly once: %+v", h.events)
			}
		})
	}
}

func TestActionEventMatchesSpecShape(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	h.key("j") // pr-11
	h.key("m")
	h.key("y")
	if len(h.events) != 1 {
		t.Fatalf("events: %+v", h.events)
	}
	b, err := json.Marshal(h.events[0])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"version": "v0.9",
		"action": map[string]any{
			"name": "merge", "surfaceId": "prs", "sourceComponentId": "btn_merge",
			"timestamp": "2026-10-22T12:00:00Z", "context": map[string]any{"pr": "pr-11"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event\n got %s\nwant %v", b, want)
	}
}

func TestInputValueReachesTheContext(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	h.key("tab") // tabs
	h.key("tab") // the comment field
	for _, r := range "LGTM 좋아요" {
		h.key(string(r))
	}
	if !h.has("LGTM 좋아요") {
		t.Fatalf("typed text not drawn:\n%s", h.plain())
	}
	h.key("esc")
	h.key("c")
	if len(h.events) != 1 || h.events[0].Action.Context["text"] != "LGTM 좋아요" || h.events[0].Action.Context["pr"] != "pr-12" {
		t.Fatalf("comment event: %+v", h.events)
	}
}

func TestMouseReachesTheSameCommands(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	y, l := h.row("#9")
	x := strings.Index(l, "#9")
	h.send(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got, _ := lookup(h.m.st.surfaces["prs"].Data, "/ui/selected"); got != "pr-9" {
		t.Fatalf("click did not select pr-9: %v", got)
	}
	by, bl := h.row("[m] Merge")
	h.send(tea.MouseMsg{X: strings.Index(bl, "[m]"), Y: by, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if !h.has(`"Merge"?`) || len(h.events) != 0 {
		t.Fatalf("clicking a danger button must open the confirm: %+v\n%s", h.events, h.plain())
	}
}

func TestBuiltinTableHasNoConflicts(t *testing.T) {
	res := bindActions(nil)
	for _, c := range res.km.Conflicts(allStacks...) {
		t.Error(c)
	}
}

func TestSplitKeysResizeThePanes(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	col := func() int {
		_, l := h.row("Author")
		a, _, _ := strings.Cut(l, "│")
		return len([]rune(a))
	}
	before := col()
	h.key(">")
	if col() <= before {
		t.Fatalf("> did not grow the left pane: %d -> %d", before, col())
	}
	h.key("=")
	if col() != before {
		t.Fatalf("= did not reset: %d, want %d", col(), before)
	}
}
