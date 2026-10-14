package chart

import (
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
)

// glyphCells lists every glyph of a set by field name, array elements
// included.
func glyphCells(g Glyphs) map[string]string {
	out := map[string]string{}
	v := reflect.ValueOf(g)
	for i := 0; i < v.NumField(); i++ {
		f, name := v.Field(i), v.Type().Field(i).Name
		switch f.Kind() {
		case reflect.String:
			out[name] = f.String()
		case reflect.Array:
			for j := 0; j < f.Len(); j++ {
				out[name+string(rune('0'+j))] = f.Index(j).String()
			}
		}
	}
	return out
}

// From theme TestGlyphSetsHaveMatchingWidths: a glyph that changes width
// between sets changes the width of every chart drawn with it.
func TestGlyphSetsHaveMatchingWidths(t *testing.T) {
	u, a := glyphCells(Unicode()), glyphCells(ASCII())
	if len(u) != len(a) {
		t.Fatalf("sets differ in shape: %d vs %d glyphs", len(u), len(a))
	}
	for name, us := range u {
		if render.Width(us) != 1 || render.Width(a[name]) != 1 {
			t.Errorf("%s: unicode %q is %d cells, ascii %q is %d; every glyph is one cell",
				name, us, render.Width(us), a[name], render.Width(a[name]))
		}
	}
	for name, s := range a {
		for _, r := range s {
			if r > 127 {
				t.Errorf("ascii %s contains %q", name, r)
			}
		}
	}
}

// The glyphs that tell bands, gaps and edges apart must differ from each
// other, or the information is gone once color is stripped.
func TestDistinguishingGlyphsDiffer(t *testing.T) {
	for _, g := range []Glyphs{Unicode(), ASCII()} {
		if g.OK == g.Warn || g.Warn == g.Error || g.OK == g.Error {
			t.Errorf("band marks must differ: %q %q %q", g.OK, g.Warn, g.Error)
		}
		for _, l := range g.Levels {
			if l == g.Gap {
				t.Errorf("gap %q looks like a sparkline level", g.Gap)
			}
		}
		for _, d := range g.Density {
			if d == g.Track || d == g.Now {
				t.Errorf("density %q looks like the track or now marker", d)
			}
		}
		seen := map[string]bool{}
		for _, h := range g.Heat {
			if seen[h] || h == g.Gap {
				t.Errorf("heat glyph %q repeats or looks like a gap", h)
			}
			seen[h] = true
		}
		if g.Full == g.Empty || g.Before == g.After {
			t.Errorf("full/empty or before/after collide in %+v", g)
		}
	}
}

func TestZeroLookDrawsASCII(t *testing.T) {
	if got := Sparkline([]float64{0, 1}, 2, SparkOpts{}); got != "_#" {
		t.Fatalf("zero opts sparkline = %q", got)
	}
	if got := Bar(0.5, 6, BarOpts{}); got != "[##--]" {
		t.Fatalf("zero opts bar = %q", got)
	}
}

func TestDetectGlyphs(t *testing.T) {
	cases := []struct {
		name, lang, override, eastAsian string
		wantASCII                       bool
	}{
		{"utf-8", "en_US.UTF-8", "", "", false},
		{"C locale", "C", "", "", true},
		{"override", "en_US.UTF-8", "1", "", true},
		// Block elements are East Asian Ambiguous: two cells on a CJK terminal.
		{"korean", "ko_KR.UTF-8", "", "", true},
		{"east asian switch", "en_US.UTF-8", "", "1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LANG", c.lang)
			t.Setenv("MY_APP_ASCII", c.override)
			t.Setenv("RUNEWIDTH_EASTASIAN", c.eastAsian)
			if got := DetectGlyphs("my-app").ASCII; got != c.wantASCII {
				t.Errorf("ASCII = %v, want %v", got, c.wantASCII)
			}
		})
	}
}

func TestLookFor(t *testing.T) {
	if LookFor(theme.Theme{}).Glyphs.ASCII != true {
		t.Error("the zero Theme draws plain, so its charts are ASCII")
	}
	if LookFor(colorTheme(theme.ASCII())).Glyphs.ASCII != true {
		t.Error("an ASCII theme gets ASCII charts")
	}
	l := LookFor(colorTheme(theme.Unicode()))
	if l.Glyphs.ASCII {
		t.Error("a Unicode theme gets Unicode charts")
	}
	if out := l.Styles.Error.Render("x"); !strings.Contains(out, "38;2;") {
		t.Errorf("Styles.Error = %q; want the theme's error color", out)
	}
	t.Setenv("LANG", "ko_KR.UTF-8")
	if !LookFor(colorTheme(theme.Unicode())).Glyphs.ASCII {
		t.Error("a CJK locale gets ASCII charts")
	}
}

// sample draws every chart with look, so a property can be checked across
// all of them at once.
func sample(look Look) map[string]string {
	spans := []Span{
		{Start: hr(2), End: hr(8), Kind: Warn, Label: "배포"},
		{Start: hr(6), End: hr(12), Kind: Error, Label: "배포"},
		{Start: hr(21), End: hr(28.5), Label: "점검 창"},
		{Start: hr(15.1), End: hr(15.1), Kind: OK, Label: "x"},
	}
	to := TimelineOpts{Look: look, Now: hr(13.5), Ruler: 3 * time.Hour}
	return map[string]string{
		"spark": Sparkline([]float64{1, 4, math.NaN(), 9, 2, 7}, 10, SparkOpts{Look: look, Last: true, Min: Fixed(0)}),
		"bar":   Bar(0.42, 16, BarOpts{Look: look, Percent: true, Label: "cpu"}),
		"meter": Meter(93, 100, 20, BarOpts{Look: look, Label: "디스크"}),
		"line":  Timeline(spans, day, hr(24), 48, to),
		"ruler": Ruler(day, hr(24), 48, 2*time.Hour, to),
		"lanes": strings.Join(Lanes(spans, day, hr(24), 50, to), "\n"),
		"heat": strings.Join(Heat([][]float64{{0, 1, 5, 9}, {math.NaN(), 0, 2}}, []string{"월", "화"}, 14,
			HeatOpts{Look: look, CellWidth: 2}), "\n"),
	}
}

// Color only reinforces: stripping it leaves exactly the glyphs a
// colorless rendering draws, and no chart relies on color to say anything.
func TestColorStrippedCarriesTheSameGlyphs(t *testing.T) {
	for _, g := range []Glyphs{Unicode(), ASCII()} {
		colored := sample(Look{Glyphs: g, Styles: FromTheme(colorTheme(theme.Unicode()))})
		plain := sample(Look{Glyphs: g})
		for name, c := range colored {
			if !strings.Contains(c, "\x1b[") {
				t.Errorf("%s: colored rendering has no color: %q", name, c)
			}
			if ansi.Strip(c) != plain[name] {
				t.Errorf("%s (ascii=%v): stripped\n%q\nwant\n%q", name, g.ASCII, ansi.Strip(c), plain[name])
			}
		}
	}
}

// Under NO_COLOR the theme's styles carry no color, and the charts drawn
// with them read exactly as the colorless ones.
func TestNoColorThemeDrawsSameGlyphs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CI", "")
	r := lipgloss.NewRenderer(io.Discard, termenv.WithTTY(true))
	th := theme.NewWith(r, theme.DefaultRoles(), theme.Unicode())
	if !th.NoColor() {
		t.Fatal("NO_COLOR is set")
	}
	noColor := sample(Look{Glyphs: Unicode(), Styles: FromTheme(th)})
	plain := sample(Look{Glyphs: Unicode()})
	for name, s := range noColor {
		if strings.Contains(s, "38;") || strings.Contains(s, "48;") {
			t.Errorf("%s emitted a color under NO_COLOR: %q", name, s)
		}
		if ansi.Strip(s) != plain[name] {
			t.Errorf("%s: NO_COLOR %q, plain %q", name, ansi.Strip(s), plain[name])
		}
	}
}

// Every option type is a value: editing a copy's glyph arrays or styles
// leaves what the original draws untouched.
func TestOptionCopiesAreIndependent(t *testing.T) {
	look := Look{Glyphs: Unicode(), Styles: FromTheme(colorTheme(theme.Unicode()))}
	want := sample(look)

	cp := look
	for i := range cp.Glyphs.Levels {
		cp.Glyphs.Levels[i] = "X"
	}
	cp.Glyphs.Partial[3] = "X"
	cp.Glyphs.Density[0] = "X"
	cp.Glyphs.Heat[4] = "X"
	cp.Styles.Heat[1] = lipgloss.NewStyle().Bold(true)
	cp.Styles.Line = lipgloss.NewStyle().Underline(true)

	for name, got := range sample(look) {
		if got != want[name] {
			t.Errorf("%s changed after editing a copy of its Look", name)
		}
	}

	so := SparkOpts{Look: look, Min: Fixed(0)}
	bo := BarOpts{Look: look, Warn: Fixed(0.5)}
	to := TimelineOpts{Look: look}
	ho := HeatOpts{Look: look}
	draw := func() []string {
		return []string{
			Sparkline([]float64{0, 9}, 2, so), Bar(0.4, 6, bo),
			Timeline([]Span{{Start: hr(1), End: hr(2)}}, day, hr(24), 24, to),
			strings.Join(Heat([][]float64{{0, 9}}, nil, 2, ho), "\n"),
		}
	}
	before := draw()
	so2, bo2, to2, ho2 := so, bo, to, ho
	so2.Glyphs.Levels[7], so2.Min = "Y", Fixed(100)
	bo2.Glyphs.Partial[0], bo2.Warn = "Y", Fixed(0.1)
	to2.Glyphs.Density[3], to2.Peak = "Y", 9
	ho2.Glyphs.Heat[0], ho2.Styles.Heat[0] = "Y", lipgloss.NewStyle().Blink(true)
	// The copies did change: the edit is real, only not shared.
	if !strings.Contains(Sparkline([]float64{0, 200}, 2, so2), "Y") || !strings.Contains(Bar(0.07, 6, bo2), "Y") ||
		!strings.Contains(Timeline([]Span{{Start: hr(1), End: hr(2)}}, day, hr(24), 24, TimelineOpts{Look: to2.Look}), "Y") ||
		!strings.Contains(Heat([][]float64{{0, 9}}, nil, 2, ho2)[0], "Y") || to2.Peak != 9 {
		t.Error("an edit to a copy did not take")
	}
	for i, got := range draw() {
		if got != before[i] {
			t.Errorf("chart %d changed after editing a copy of its options: %q -> %q", i, before[i], got)
		}
	}
}

// fits checks one rendered chart line: exactly want cells, "" for want <= 0.
func fits(t *testing.T, name string, got string, want int) {
	t.Helper()
	if want <= 0 {
		if got != "" {
			t.Errorf("%s at width %d = %q, want empty", name, want, got)
		}
		return
	}
	if w := render.Width(got); w != want {
		t.Errorf("%s at width %d is %d cells: %q", name, want, w, got)
	}
}

// The width contract, swept across every function, both glyph sets, color,
// Hangul labels and the narrow widths where layouts usually break.
func TestEveryChartIsExactlyItsWidth(t *testing.T) {
	spans := []Span{
		{Start: hr(-2), End: hr(3), Label: "데이터베이스 점검"},
		{Start: hr(5), End: hr(5)},
		{Start: hr(9), End: hr(30), Kind: Error, Label: "배포"},
	}
	long := make([]float64, 100)
	for i := range long {
		long[i] = float64(i % 7)
	}
	for _, look := range []Look{{Glyphs: Unicode()}, {}, LookFor(colorTheme(theme.Unicode()))} {
		for w := -1; w <= 40; w++ {
			fits(t, "sparkline", Sparkline(long, w, SparkOpts{Look: look, Last: true}), w)
			fits(t, "short sparkline", Sparkline([]float64{1, math.NaN()}, w, SparkOpts{Look: look, Last: true, Right: true}), w)
			fits(t, "empty sparkline", Sparkline(nil, w, SparkOpts{Look: look}), w)
			fits(t, "bar", Bar(0.37, w, BarOpts{Look: look}), w)
			fits(t, "meter", Meter(7, 8, w, BarOpts{Look: look, Label: "메모리 사용량"}), w)
			fits(t, "timeline", Timeline(spans, day, hr(24), w, TimelineOpts{Look: look, Now: hr(12)}), w)
			fits(t, "ruler", Ruler(day, hr(24), w, time.Hour, TimelineOpts{Look: look, Now: hr(12)}), w)
			lanes := Lanes(spans, day, hr(24), w, TimelineOpts{Look: look, Ruler: 6 * time.Hour})
			heat := Heat([][]float64{long[:24], long[:7]}, []string{"월요일", "Tue"}, w, HeatOpts{Look: look, CellWidth: 2})
			if w <= 0 {
				if lanes != nil || heat != nil {
					t.Errorf("width %d: lanes %q heat %q, want nil", w, lanes, heat)
				}
				continue
			}
			if len(lanes) != 4 || len(heat) != 2 {
				t.Fatalf("width %d: %d lane rows, %d heat rows", w, len(lanes), len(heat))
			}
			for _, l := range append(lanes, heat...) {
				fits(t, "lanes/heat row", l, w)
			}
		}
	}
}

// Under a Korean locale the ellipsis is `~`, and widths still hold.
func TestWidthsHoldUnderKoreanLocale(t *testing.T) {
	t.Setenv("LANG", "ko_KR.UTF-8")
	look := LookFor(colorTheme(theme.Unicode()))
	spans := []Span{{Start: hr(1), End: hr(4), Label: "데이터베이스 점검"}, {Start: hr(2), End: hr(3), Label: "배포"}}
	rows := Lanes(spans, day, hr(24), 30, TimelineOpts{Look: look, LabelMax: 8})
	for _, r := range rows {
		fits(t, "lane", r, 30)
	}
	if got := ansi.Strip(rows[0]); !strings.HasPrefix(got, "데이터~ ") {
		t.Errorf("lane label = %q, want cut with ~", got)
	}
}
