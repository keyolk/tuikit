package chart

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
)

func TestBarGolden(t *testing.T) {
	cases := []struct {
		frac      float64
		uni, ascW string
	}{
		{0, "▕          ▏", "[----------]"},
		{0.29, "▕██▉       ▏", "[##=-------]"},
		{0.5, "▕█████     ▏", "[#####-----]"},
		{0.77, "▕███████▋  ▏", "[#######=--]"},
		{1.0 / 80, "▕▏         ▏", "[----------]"},
		{1, "▕██████████▏", "[##########]"},
	}
	for _, c := range cases {
		if got := Bar(c.frac, 12, BarOpts{Look: uni}); got != c.uni {
			t.Errorf("unicode Bar(%v) = %q, want %q", c.frac, got, c.uni)
		}
		if got := Bar(c.frac, 12, BarOpts{Look: asc}); got != c.ascW {
			t.Errorf("ascii Bar(%v) = %q, want %q", c.frac, got, c.ascW)
		}
	}
}

// okx: a bar is full only when the work is done, and the percent agrees.
func TestBarIsFullOnlyAtOne(t *testing.T) {
	for _, f := range []float64{0.999, 0.99999999} {
		if got := Bar(f, 12, BarOpts{Look: uni, Percent: true}); strings.Contains(got, "██████████▏") || !strings.HasSuffix(got, "99%") {
			t.Errorf("Bar(%v) = %q; must not look done", f, got)
		}
	}
	// 0.29*100 is 28.999… in floating point; it still reads 29%.
	if got := Bar(0.29, 12, BarOpts{Look: asc, Percent: true}); !strings.HasSuffix(got, " 29%") {
		t.Errorf("Bar(0.29) = %q, want 29%%", got)
	}
}

func TestBarClamps(t *testing.T) {
	for _, c := range []struct {
		frac float64
		want string
	}{{-1, "[---]   0%"}, {2, "[###] 100%"}, {math.Inf(1), "[###] 100%"}, {math.NaN(), "[---]   ?%"}} {
		if got := Bar(c.frac, 10, BarOpts{Look: asc, Percent: true}); got != c.want {
			t.Errorf("Bar(%v) = %q, want %q", c.frac, got, c.want)
		}
	}
}

// Each band leads with its own glyph, so the band survives NO_COLOR, and
// fills in its own role.
func TestMeterBandsCarryAGlyph(t *testing.T) {
	th := colorTheme(theme.Unicode())
	look := Look{Glyphs: Unicode(), Styles: FromTheme(th)}
	cases := []struct {
		value float64
		mark  string
		fill  string // the role's foreground in the rendered fill
	}{
		{10, "✓", "38;2;34;197;94"},  // Success
		{80, "!", "38;2;245;158;11"}, // Warn
		{95, "✗", "38;2;239;68;68"},  // Error
	}
	for _, c := range cases {
		got := Meter(c.value, 100, 16, BarOpts{Look: look})
		if plain := ansi.Strip(got); !strings.HasPrefix(plain, c.mark+" ") {
			t.Errorf("Meter(%v) = %q, want it to lead with %q", c.value, plain, c.mark)
		}
		if !strings.Contains(got, c.fill) {
			t.Errorf("Meter(%v) = %q, want the fill in %s", c.value, got, c.fill)
		}
	}
	for v, want := range map[float64]string{10: "v", 80: "!", 95: "x"} {
		if got := Meter(v, 100, 16, BarOpts{Look: asc}); !strings.HasPrefix(got, want+" [") {
			t.Errorf("ascii Meter(%v) = %q, want %q first", v, got, want)
		}
	}
	// Thresholds of its own: only Error set means no warn band.
	if got := Bar(0.8, 10, BarOpts{Look: asc, Error: Fixed(0.5)}); got[0] != 'x' {
		t.Errorf("Error at 0.5 = %q", got)
	}
	if got := Bar(0.8, 10, BarOpts{Look: asc, Warn: Fixed(0.9)}); got[0] != 'v' {
		t.Errorf("Warn at 0.9 = %q", got)
	}
	// An unknown reading is not a full one.
	if got := Meter(5, 0, 12, BarOpts{Look: asc}); got != "v [---]   ?%" {
		t.Errorf("Meter over a zero limit = %q", got)
	}
}

func TestBarNarrowWidths(t *testing.T) {
	cases := []struct {
		frac  float64
		width int
		o     BarOpts
		want  string
	}{
		{1, 1, BarOpts{Look: uni}, "█"},
		{0.5, 2, BarOpts{Look: uni}, "█ "},
		{0.5, 3, BarOpts{Look: asc}, "[=]"},
		{0.95, 1, BarOpts{Look: uni, Error: Fixed(0.9)}, "✗"},
		{0.95, 2, BarOpts{Look: uni, Error: Fixed(0.9)}, "✗▉"},
		{0.5, 3, BarOpts{Look: asc, Warn: Fixed(0.9)}, "v#-"},
		{0.5, 4, BarOpts{Look: asc, Warn: Fixed(0.9)}, "v #-"},
		// The percent drops before the bar shrinks below three cells.
		{0.5, 7, BarOpts{Look: asc, Percent: true}, "[##=--]"},
		{0.5, 8, BarOpts{Look: asc, Percent: true}, "[=]  50%"},
	}
	for _, c := range cases {
		if got := Bar(c.frac, c.width, c.o); got != c.want {
			t.Errorf("Bar(%v, %d) = %q, want %q", c.frac, c.width, got, c.want)
		}
	}
}

// The label never takes more than the bar, is cut with the ellipsis, and
// never splits a Hangul syllable.
func TestBarLabelFits(t *testing.T) {
	got := Bar(0.5, 24, BarOpts{Look: uni, Percent: true, Label: "디스크 사용량 (root)"})
	if render.Width(got) != 24 {
		t.Fatalf("%q is %d cells", got, render.Width(got))
	}
	if !strings.HasSuffix(got, " 50% 디스크 …") {
		t.Errorf("label = %q", got)
	}
	short := Bar(0.5, 24, BarOpts{Look: asc, Label: "cpu"})
	if !strings.HasSuffix(short, "] cpu") || render.Width(short) != 24 {
		t.Errorf("short label = %q", short)
	}
}
