package surface

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/keyolk/tuikit/mouse"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/tuitest"
	"github.com/muesli/termenv"
)

// catalog is one fixture per terminal-catalog component: the components
// (root first) and what the drawn frame must contain.
var catalog = []struct {
	name, comps, want string
}{
	{"Column", `{"id":"root","component":"Column","children":["a","b"]},{"id":"a","component":"Text","text":"first"},{"id":"b","component":"Text","text":"second"}`, "second"},
	{"Row", `{"id":"root","component":"Row","children":["a","b"]},{"id":"a","component":"Text","text":"left"},{"id":"b","component":"Text","text":"right"}`, "right"},
	{"Split", `{"id":"root","component":"Split","left":"a","right":"b","ratio":0.3},{"id":"a","component":"Text","text":"pane A"},{"id":"b","component":"Text","text":"pane B"}`, "pane B"},
	{"Tabs", `{"id":"root","component":"Tabs","tabs":[{"title":"One","child":"a"},{"title":"Two","child":"b"}]},{"id":"a","component":"Text","text":"tab body"},{"id":"b","component":"Text","text":"other"}`, "tab body"},
	{"Text", `{"id":"root","component":"Text","text":"warned","style":"warn"}`, "warned"},
	{"List", `{"id":"root","component":"List","children":{"path":"/items","componentId":"it"},"itemKey":"id"},{"id":"it","component":"Text","text":{"path":"name"}}`, "beta"},
	{"Table", `{"id":"root","component":"Table","rows":{"path":"/items"},"columns":[{"title":"Name","value":{"path":"name"}},{"title":"State","value":{"path":"state"},"status":true}]}`, "failure"},
	{"Detail", `{"id":"root","component":"Detail","fields":[{"label":"Owner","value":{"path":"/owner"}}]}`, "Owner"},
	{"Input", `{"id":"root","component":"Input","label":"Name","value":{"path":"/owner"}}`, "Name"},
	{"Button", `{"id":"root","component":"Button","label":"Deploy","key":"d","action":{"event":{"name":"deploy"}}}`, "[d] Deploy"},
	{"Badge", `{"id":"root","component":"Badge","text":"green","status":"ok"}`, "green"},
	{"Link", `{"id":"root","component":"Link","text":"docs","url":"https://a2ui.org/"}`, "docs"},
	{"Progress", `{"id":"root","component":"Progress","value":0.5,"label":"half"}`, "50%"},
	{"Divider", `{"id":"root","component":"Column","children":["a","d"]},{"id":"a","component":"Text","text":"above"},{"id":"d","component":"Divider"}`, "above"},
	{"Card", `{"id":"root","component":"Card","child":"a"},{"id":"a","component":"Text","text":"in card"}`, "in card"},
}

const fixtureData = `{"version":"v0.9","updateDataModel":{"surfaceId":"s","value":{"owner":"한글 소유자 Kim","items":[` +
	`{"id":"a","name":"alpha 알파","state":"success"},{"id":"b","name":"beta 베타","state":"failure"}]}}}`

func load(h *harness, comps string) {
	h.t.Helper()
	h.lines(`{"version":"v0.9","createSurface":{"surfaceId":"s","catalogId":"tuikit.terminal/v1"}}`,
		`{"version":"v0.9","updateComponents":{"surfaceId":"s","components":[`+comps+`]}}`, fixtureData)
}

func TestEveryCatalogComponentFits(t *testing.T) {
	for _, c := range catalog {
		t.Run(c.name+"/80x24", func(t *testing.T) {
			h := newHarness(t, 80, 24)
			load(h, c.comps)
			frame := h.view()
			checkWidth(t, frame, 80)
			if n := strings.Count(frame, "\n") + 1; n != 24 {
				t.Errorf("frame has %d lines, want 24", n)
			}
			if !h.has(c.want) {
				t.Errorf("missing %q:\n%s", c.want, h.plain())
			}
			if strings.Contains(h.plain(), "unsupported") || len(h.events) != 0 {
				t.Errorf("fixture rejected:\n%s\n%+v", h.plain(), h.events)
			}
		})
		t.Run(c.name+"/60cols", func(t *testing.T) {
			// Below the 80-column floor the app shows "too small", so the
			// component is drawn directly into a 60-cell pane.
			h := newHarness(t, 80, 24)
			load(h, c.comps)
			sf, _ := h.m.st.active()
			out := newDrawer(sf, h.m.th, h.m.ui, h.m.logf, true).draw("root", "", mouse.Rect{W: 60, H: 8})
			checkWidth(t, out, 60)
			if !strings.Contains(ansi.Strip(out), c.want) {
				t.Errorf("missing %q at 60 cols:\n%s", c.want, ansi.Strip(out))
			}
		})
	}
}

func TestTooSmallFloor(t *testing.T) {
	for _, sz := range [][2]int{{79, 24}, {80, 23}, {60, 24}, {20, 5}} {
		h := newHarness(t, sz[0], sz[1])
		h.demo()
		if !h.has("terminal too small") && sz[0] > 30 {
			t.Errorf("%v: no too-small message:\n%s", sz, h.plain())
		}
		checkWidth(t, h.view(), sz[0])
		// Keys act on nothing the user can see: only quit runs.
		h.key("m")
		h.key("y")
		if len(h.events) != 0 || h.m.confirm {
			t.Errorf("%v: blind action ran: %+v", sz, h.events)
		}
		h.key("q")
		if !quits(h.run()) {
			t.Errorf("%v: q must still quit", sz)
		}
	}
}

func TestCJKTextKeepsWidths(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	h.key("]") // the Comments tab carries Hangul
	frame := h.view()
	checkWidth(t, frame, 80)
	if !h.has("한국어 댓글도") {
		t.Fatalf("Hangul comment not drawn:\n%s", h.plain())
	}
	// Every line ends exactly at the terminal edge: wide characters were
	// counted as two cells when padding.
	for i, l := range strings.Split(frame, "\n") {
		if w := render.Width(l); w != 80 {
			t.Errorf("line %d is %d cells, want 80: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestUnknownComponentIsAPlaceholderInItsPane(t *testing.T) {
	h := newHarness(t, 80, 24)
	load(h, `{"id":"root","component":"Split","left":"x","right":"y"},`+
		`{"id":"x","component":"Sparkline3D","values":[1,2]},`+
		`{"id":"y","component":"Column","children":["ok","broken"]},`+
		`{"id":"ok","component":"Text","text":"still here"},`+
		`{"id":"broken","component":"Column","children":42}`)
	_, l := h.row("unsupported component Sparkline3D")
	left, _, _ := strings.Cut(l, "│")
	if !strings.Contains(left, "Sparkline3D") {
		t.Errorf("placeholder is not in the left pane: %q", l)
	}
	if !h.has("still here") {
		t.Errorf("sibling lost:\n%s", h.plain())
	}
	if !h.has("Column \"broken\"") {
		t.Errorf("bad props must show their error in that pane:\n%s", h.plain())
	}
}

func TestCycleStops(t *testing.T) {
	h := newHarness(t, 80, 24)
	load(h, `{"id":"root","component":"Column","children":["loop","tail"]},{"id":"loop","component":"Column","children":["loop"]},{"id":"tail","component":"Text","text":"after the loop"}`)
	if !h.has("cycle at loop") || !h.has("after the loop") {
		t.Fatalf("cycle not contained:\n%s", h.plain())
	}
}

// colorSGR matches an SGR sequence that sets a foreground or background
// color (30-37, 38, 40-47, 48, 90-97, 100-107).
var colorSGR = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*(?:3[0-8]|4[0-8]|9[0-7]|10[0-7])(?:;[0-9]+)*m`)

// ansiTheme draws with a 16-color profile even though tests have no TTY,
// so the test sees the escapes a terminal would get.
func ansiTheme() theme.Theme {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	return theme.New(r)
}

func TestNoColorKeepsTheSelectionVisible(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.m.th = ansiTheme()
	h.demo()
	if !colorSGR.MatchString(h.view()) {
		t.Fatal("control: a colored theme must emit color escapes")
	}

	t.Setenv("NO_COLOR", "1")
	h = newHarness(t, 80, 24)
	h.m.th = ansiTheme()
	h.demo()
	frame := h.view()
	if loc := colorSGR.FindStringIndex(frame); loc != nil {
		t.Errorf("NO_COLOR frame carries a color: %q", frame[loc[0]:loc[1]])
	}
	if rv := tuitest.Reversed(frame); len(rv) == 0 || !strings.Contains(strings.Join(rv, "|"), "#12") {
		t.Errorf("the focused row must stay reverse video under NO_COLOR, reversed runs: %q", rv)
	}
	// Status words keep a glyph lead so the meaning survives without color.
	_, l := h.row("#9")
	if !strings.Contains(l, h.m.th.Glyphs.Cross) {
		t.Errorf("failed PR lost its glyph under NO_COLOR: %q", l)
	}
}

func TestStatusTones(t *testing.T) {
	for in, want := range map[string]tone{"success": toneOK, "FAILURE": toneErr, "pending": toneWarn, "weird": toneMuted, "accent": toneAccent} {
		if got := toneOf(in); got != want {
			t.Errorf("toneOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCatalogDocumentMatchesTheRenderer(t *testing.T) {
	var doc struct {
		CatalogID  string                     `json:"catalogId"`
		Components map[string]json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(CatalogJSON, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.CatalogID != CatalogID {
		t.Errorf("catalogId %q, want %q", doc.CatalogID, CatalogID)
	}
	drawn := map[string]bool{"Status": true, "TextField": true} // aliases
	for _, c := range catalog {
		drawn[c.name] = true
	}
	for name := range doc.Components {
		if !drawn[name] {
			t.Errorf("catalog documents %s but no fixture draws it", name)
		}
		delete(drawn, name)
	}
	for name := range drawn {
		t.Errorf("fixture %s is not in the catalog document", name)
	}
}
