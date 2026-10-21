package surface

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const (
	create = `{"version":"v0.9","createSurface":{"surfaceId":"s","catalogId":"tuikit.terminal/v1","theme":{"agentDisplayName":"Agent X"}}}`
	tree   = `{"version":"v0.9","updateComponents":{"surfaceId":"s","components":[` +
		`{"id":"root","component":"Column","children":["hello","name"]},` +
		`{"id":"hello","component":"Text","text":"hello surface"},` +
		`{"id":"name","component":"Text","text":{"path":"/user/name"}}]}}`
)

func TestCreateUpdateDataDelete(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.lines(create)
	if !h.has("Agent X") || !h.has("waiting for the root component") {
		t.Fatalf("after createSurface:\n%s", h.plain())
	}
	h.lines(tree)
	if !h.has("hello surface") {
		t.Fatalf("after updateComponents:\n%s", h.plain())
	}
	h.lines(`{"version":"v0.9","updateDataModel":{"surfaceId":"s","path":"/user/name","value":"Ada"}}`)
	if !h.has("Ada") {
		t.Fatalf("data binding not drawn:\n%s", h.plain())
	}
	// A value omitted removes the key.
	h.lines(`{"version":"v0.9","updateDataModel":{"surfaceId":"s","path":"/user/name"}}`)
	if h.has("Ada") {
		t.Fatalf("removed key still drawn:\n%s", h.plain())
	}
	// Replacing a component by id.
	h.lines(`{"version":"v0.9","updateComponents":{"surfaceId":"s","components":[{"id":"hello","component":"Text","text":"replaced"}]}}`)
	if h.has("hello surface") || !h.has("replaced") {
		t.Fatalf("component update not applied:\n%s", h.plain())
	}
	h.lines(`{"version":"v0.9","deleteSurface":{"surfaceId":"s"}}`)
	if !h.has("waiting for a surface") {
		t.Fatalf("after deleteSurface:\n%s", h.plain())
	}
	if len(h.events) != 0 {
		t.Errorf("valid messages emitted events: %+v", h.events)
	}
}

func TestDataModelPointerRules(t *testing.T) {
	var root any
	root, err := setPath(root, "/a/b", 1.0)
	if err != nil {
		t.Fatal(err)
	}
	root, err = setPath(root, "/list", []any{"x", "y", "z"})
	if err != nil {
		t.Fatal(err)
	}
	before := root
	root, err = removePath(root, "/list/1")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := lookup(root, "/list"); !reflect.DeepEqual(got, []any{"x", nil, "z"}) {
		t.Errorf("array remove must keep the length, got %v", got)
	}
	if got, _ := lookup(before, "/list/1"); got != "y" {
		t.Errorf("write aliased an older copy: %v", got)
	}
	if _, err := setPath(root, "/list/name", 1.0); err == nil {
		t.Error("non-numeric index on an array must fail")
	}
	if got, _ := lookup(root, "/a/b"); got != 1.0 {
		t.Errorf("/a/b = %v", got)
	}
	if join("/items/3", "name") != "/items/3/name" || join("/items/3", "/abs") != "/abs" {
		t.Error("relative path resolution")
	}
}

func TestUnknownComponentIDIsAPendingSlot(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.lines(create, `{"version":"v0.9","updateComponents":{"surfaceId":"s","components":[`+
		`{"id":"root","component":"Column","children":["later","ok"]},{"id":"ok","component":"Text","text":"sibling ok"}]}}`)
	if !h.has("… later") || !h.has("sibling ok") {
		t.Fatalf("a missing child must show a pending slot and leave siblings alone:\n%s", h.plain())
	}
	// An update to an id nothing references is stored, harmlessly.
	h.lines(`{"version":"v0.9","updateComponents":{"surfaceId":"s","components":[{"id":"orphan","component":"Text","text":"orphan"}]}}`)
	if h.has("orphan") || len(h.events) != 0 {
		t.Fatalf("orphan drawn or rejected:\n%s %+v", h.plain(), h.events)
	}
	h.lines(`{"version":"v0.9","updateComponents":{"surfaceId":"s","components":[{"id":"later","component":"Text","text":"arrived"}]}}`)
	if !h.has("arrived") {
		t.Fatalf("late component not drawn:\n%s", h.plain())
	}
}

func TestMalformedMessageKeepsTheLastGoodFrame(t *testing.T) {
	cases := map[string]string{
		"json":         `{"version":"v0.9","updateComponents":{`,
		"version":      `{"version":"v0.7","deleteSurface":{"surfaceId":"s"}}`,
		"no type":      `{"version":"v0.9","hello":{}}`,
		"two types":    `{"version":"v0.9","deleteSurface":{"surfaceId":"s"},"createSurface":{"surfaceId":"t","catalogId":"x"}}`,
		"unknown surf": `{"version":"v0.9","updateDataModel":{"surfaceId":"nope","value":1}}`,
		"dup create":   create,
		"catalog":      `{"version":"v0.9","createSurface":{"surfaceId":"t","catalogId":"https://example.com/other"}}`,
		// One good and one bad component: the message is the unit, so the
		// good one must not land either.
		"atomic": `{"version":"v0.9","updateComponents":{"surfaceId":"s","components":[` +
			`{"id":"hello","component":"Text","text":"half applied"},{"component":"Text","text":"no id"}]}}`,
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, 80, 24)
			h.lines(create, tree)
			good := h.m.st
			h.lines(bad)
			if !reflect.DeepEqual(h.m.st, good) {
				t.Fatalf("state changed by a rejected message")
			}
			if !h.has("hello surface") || h.has("half applied") {
				t.Fatalf("last good frame lost:\n%s", h.plain())
			}
			if !h.has("bad message") {
				t.Errorf("no toast:\n%s", h.plain())
			}
			if len(h.logs) == 0 {
				t.Error("not logged")
			}
			if len(h.events) != 1 || h.events[0].Error == nil || h.events[0].Error.Code != "VALIDATION_FAILED" {
				t.Fatalf("want one VALIDATION_FAILED event, got %+v", h.events)
			}
		})
	}
}

func TestMessageArrayIsNotATransaction(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.lines(create, tree)
	h.lines(`[{"version":"v0.9","updateDataModel":{"surfaceId":"s","path":"/user/name","value":"Grace"}},` +
		`{"version":"v0.9","updateDataModel":{"surfaceId":"missing","value":1}}]`)
	if !h.has("Grace") {
		t.Fatalf("the good element of an array must apply:\n%s", h.plain())
	}
	if len(h.events) != 1 || !strings.HasPrefix(h.events[0].Error.Path, "/1") {
		t.Fatalf("the bad element must be reported at /1…, got %+v", h.events)
	}
}

func TestErrorEventMatchesClientSchema(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.lines(`not json`)
	b, err := json.Marshal(h.events[0])
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]map[string]any
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2 || string(raw["version"]) != `"v0.9"` {
		t.Fatalf("envelope must be exactly version + error: %s", b)
	}
	delete(raw, "version")
	nb, _ := json.Marshal(raw)
	if err := json.Unmarshal(nb, &top); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"code", "surfaceId", "path", "message"} {
		if _, ok := top["error"][k]; !ok {
			t.Errorf("error lacks %q: %s", k, b)
		}
	}
}
