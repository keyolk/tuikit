package surface

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/keyolk/tuikit/link"
	"github.com/keyolk/tuikit/tuitest"
)

func TestOnlyHTTPLinksAreLinkedOrOpened(t *testing.T) {
	for _, bad := range []string{"javascript:alert(1)", "file:///etc/passwd", "data:text/html,x", "ftp://x.io/a", "https://x.io/\x1b]8;;evil"} {
		t.Run(bad, func(t *testing.T) {
			h := newHarness(t, 80, 24)
			load(h, `{"id":"root","component":"Column","children":["l","b"]},`+
				`{"id":"l","component":"Link","text":"click me","url":{"path":"/u"}},`+
				`{"id":"b","component":"Button","label":"Go","key":"x","action":{"functionCall":{"call":"openUrl","args":{"url":{"path":"/u"}}}}}`)
			h.lines(`{"version":"v0.9","updateDataModel":{"surfaceId":"s","path":"/u","value":` + jsonString(bad) + `}}`)
			if ls := tuitest.Links(h.view()); len(ls) != 0 {
				t.Fatalf("frame links %v", ls)
			}
			if !h.has("click me") {
				t.Fatalf("link text must stay as plain text:\n%s", h.plain())
			}
			if _, f := h.row("? help"); strings.Contains(f, "open link") {
				t.Errorf("footer offers o for a refused link: %q", f)
			}
			h.key("o")
			h.key("x")
			msgs := h.run()
			if len(h.opened) != 0 {
				t.Fatalf("opened %v", h.opened)
			}
			var none bool
			for _, m := range msgs {
				if om, ok := m.(link.OpenedMsg); ok && errors.Is(om.Err, link.ErrNoLink) {
					none = true
				}
			}
			if !none {
				t.Errorf("o must answer \"no link here\", got %v", msgs)
			}
		})
	}
}

func jsonString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\x1b", `\u001b`)
	return `"` + r.Replace(s) + `"`
}

func TestOOpensTheSelectedPRLink(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	h.key("j")
	if ls := tuitest.Links(h.view()); len(ls) != 1 || ls[0].URL != "https://github.com/keyolk/tuikit/pull/11" {
		t.Fatalf("frame links %+v", ls)
	}
	h.key("o")
	h.run()
	if len(h.opened) != 1 || h.opened[0] != "https://github.com/keyolk/tuikit/pull/11" {
		t.Fatalf("opened %v", h.opened)
	}
}

// split returns the left and right pane text of each body row, cut at the
// divider as drawn.
func panes(t *testing.T, frame string) (left, right []string) {
	t.Helper()
	for _, l := range strings.Split(frame, "\n") {
		a, b, ok := strings.Cut(l, "│")
		if !ok {
			continue
		}
		left, right = append(left, a), append(right, b)
	}
	if len(left) == 0 {
		t.Fatalf("no split in frame:\n%s", frame)
	}
	return left, right
}

func TestIncrementalUpdateChangesOnlyItsPane(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	h.key("j") // pr-11, whose "test (race)" check is pending
	before := h.plain()
	if _, l := h.row("test (race)"); !strings.Contains(l, "pending") {
		t.Fatalf("fixture: %q", l)
	}
	h.lines(`{"version":"v0.9","updateDataModel":{"surfaceId":"prs","path":"/prs/1/checks/1/state","value":"success"}}`)
	after := h.plain()
	if _, l := h.row("test (race)"); !strings.Contains(l, "success") {
		t.Fatalf("check did not flip: %q", l)
	}
	bl, br := panes(t, before)
	al, ar := panes(t, after)
	for i := range bl {
		if bl[i] != al[i] {
			t.Errorf("left pane row %d changed: %q -> %q", i, bl[i], al[i])
		}
	}
	changed := 0
	for i := range br {
		if br[i] != ar[i] {
			changed++
			if !strings.Contains(ar[i], "test (race)") {
				t.Errorf("unrelated right row %d changed: %q -> %q", i, br[i], ar[i])
			}
		}
	}
	if changed != 1 {
		t.Errorf("%d rows changed, want 1", changed)
	}
	bf, af := strings.Split(before, "\n"), strings.Split(after, "\n")
	if bf[0] != af[0] || bf[len(bf)-1] != af[len(af)-1] {
		t.Error("title or footer changed")
	}
}

func TestCursorFollowsItsItemWhenReordered(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	h.key("j")
	_, l := h.row("#11")
	if !strings.Contains(l, h.m.th.Glyphs.Cursor) {
		t.Fatalf("fixture: cursor not on #11: %q", l)
	}
	// The agent reverses the list: pr-11 stays in the middle, so move it
	// to the front instead to make the index change.
	h.lines(`{"version":"v0.9","updateDataModel":{"surfaceId":"prs","path":"/prs","value":[` +
		`{"id":"pr-11","number":11,"title":"split","state":"open"},` +
		`{"id":"pr-9","number":9,"title":"selection","state":"failed"},` +
		`{"id":"pr-12","number":12,"title":"surface","state":"open"}]}}`)
	cy, l := h.row("#11")
	if !strings.Contains(l, h.m.th.Glyphs.Cursor) {
		t.Fatalf("cursor left pr-11 after reorder:\n%s", h.plain())
	}
	if y9, _ := h.row("#9"); y9 <= cy {
		t.Fatalf("fixture: #11 was not moved above #9")
	}
	if got, _ := lookup(h.m.st.surfaces["prs"].Data, "/ui/selected"); got != "pr-11" {
		t.Fatalf("/ui/selected = %v", got)
	}
	// The selected item vanishing keeps the row position, never panics.
	h.lines(`{"version":"v0.9","updateDataModel":{"surfaceId":"prs","path":"/prs","value":[{"id":"pr-9","number":9,"title":"selection","state":"failed"}]}}`)
	if _, l := h.row("#9"); !strings.Contains(l, h.m.th.Glyphs.Cursor) {
		t.Fatalf("cursor lost after delete:\n%s", h.plain())
	}
}

func TestDemoStreamsEndToEnd(t *testing.T) {
	h := newHarness(t, 100, 30)
	h.file("../examples/surface/pr-dashboard.jsonl")
	if len(h.events) != 0 {
		t.Fatalf("the demo stream must be valid: %+v", h.events)
	}
	h.key("j")
	if _, l := h.row("test (race)"); !strings.Contains(l, "success") {
		t.Fatalf("final update not applied: %q", l)
	}
	if _, err := os.Stat("../examples/surface/pr-dashboard.jsonl"); err != nil {
		t.Fatal(err)
	}
}
