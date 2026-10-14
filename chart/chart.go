// Package chart draws one-line charts for a terminal UI: a Sparkline, a Bar
// or Meter, a Timeline of spans (with a Ruler and one row per lane), and a
// Heat grid.
//
// Every function returns lines of exactly the requested width in cells as
// render.Width counts them, and "" when the width is zero or less. The
// information is in the glyphs: color, taken from theme roles, only
// reinforces it, so a chart under NO_COLOR (or piped to a file) reads the
// same once its escapes are stripped. Every option type is a plain value —
// glyph sets are arrays, not slices — so a copy never aliases the original.
//
// Ported from ccproxy renderSparkline (bucket-max downsampling, padding
// instead of stretching), ccx stats_timeline (8-level ramp, bucket mean),
// narwhal live.go (sparkline scaled to its own peak), gcl timeline.go (span
// columns with an exclusive end, sub-cell spans keep one cell, density
// levels, overflow arrows, the now marker, tick spacing) and okx refresh.go
// (a bar that is full only when the work is done, and a percent suffix).
package chart

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
)

// Glyphs is a chart glyph set. Every glyph is one cell wide in both sets, so
// switching sets never changes a chart's width. Fixed-size arrays keep the
// struct a value: a copied set can be edited without touching the original.
//
// The Unicode set is made of block elements (▁ █ ░ ▒ ▓), which are East Asian
// Ambiguous: a CJK terminal may draw them two cells wide. DetectGlyphs and
// LookFor therefore pick the ASCII set under a CJK locale, the same way
// render.Ellipsis falls back to `~` there.
type Glyphs struct {
	ASCII bool // this is the ASCII set

	// Levels is the sparkline ramp, lowest first.
	Levels [8]string
	// Gap marks a missing (NaN or infinite) sample.
	Gap string
	// Last follows the last point when SparkOpts.Last is set.
	Last string

	// Partial[k-1] is a bar cell filled k/8 (k = 1..7).
	Partial [7]string
	// Full and Empty are whole bar cells; Open and Close bracket the bar.
	Full, Empty, Open, Close string
	// OK, Warn and Error mark a Meter's band, so the band reads without color.
	OK, Warn, Error string

	// Density is the timeline ramp for how many spans share a cell, lowest
	// first; Track is a cell no span touches.
	Density [4]string
	Track   string
	// Now marks the current moment on a timeline or ruler.
	Now string
	// Before and After mark a span that continues past the window's edge.
	Before, After string

	// Heat is the heat-grid ramp: [0] is the floor (zero), [4] the peak.
	Heat [5]string
}

// Unicode is the block-element set.
func Unicode() Glyphs {
	return Glyphs{
		Levels:  [8]string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"},
		Gap:     " ",
		Last:    "◂",
		Partial: [7]string{"▏", "▎", "▍", "▌", "▋", "▊", "▉"},
		Full:    "█", Empty: " ", Open: "▕", Close: "▏",
		OK: "✓", Warn: "!", Error: "✗",
		Density: [4]string{"░", "▒", "▓", "█"},
		Track:   " ", Now: "╎", Before: "‹", After: "›",
		Heat: [5]string{"⋅", "░", "▒", "▓", "█"},
	}
}

// ASCII is the 7-bit set. Its sparkline ramp has six distinct glyphs for
// eight levels (_.-=*#), and a bar cell has half-cell resolution (= is at
// least half full), so some steps the Unicode set shows merge here.
func ASCII() Glyphs {
	return Glyphs{
		ASCII:   true,
		Levels:  [8]string{"_", "_", ".", "-", "=", "=", "*", "#"},
		Gap:     " ",
		Last:    "<",
		Partial: [7]string{"-", "-", "-", "=", "=", "=", "="},
		Full:    "#", Empty: "-", Open: "[", Close: "]",
		OK: "v", Warn: "!", Error: "x",
		Density: [4]string{".", ":", "+", "#"},
		Track:   " ", Now: "|", Before: "<", After: ">",
		Heat: [5]string{"-", ".", ":", "+", "#"},
	}
}

// DetectGlyphs picks a set the way theme.DetectGlyphs does (locale and
// `<APP>_ASCII`), and also picks ASCII under a CJK locale, where the
// Unicode set's block elements draw two cells wide.
func DetectGlyphs(app string) Glyphs {
	if theme.DetectGlyphs(app).ASCII || render.CJKLocale() {
		return ASCII()
	}
	return Unicode()
}

// set is g, or the ASCII set when g is the zero Glyphs: a zero option value
// draws plain ASCII rather than empty strings that shift columns.
func (g Glyphs) set() Glyphs {
	if g.Levels[len(g.Levels)-1] == "" {
		return ASCII()
	}
	return g
}

// ellipsis is the truncation marker for labels: `~` with the ASCII set,
// render.Ellipsis otherwise (which is `~` under a CJK locale too).
func (g Glyphs) ellipsis() string {
	if g.ASCII {
		return "~"
	}
	return render.Ellipsis()
}

// Styles colors chart cells by role. The zero Styles draws no escapes.
type Styles struct {
	Line  lipgloss.Style // sparkline cells, bar fill, a Normal span
	Muted lipgloss.Style // empty bar, track, ruler labels
	Last  lipgloss.Style // the last-point marker
	Now   lipgloss.Style // the now marker
	Info  lipgloss.Style // an Info span
	OK    lipgloss.Style // an OK span, a Meter in its ok band
	Warn  lipgloss.Style // a Warn span, a Meter in its warn band
	Error lipgloss.Style // an Error span, a Meter in its error band
	// Heat colors the Heat ramp, [0] the floor.
	Heat [5]lipgloss.Style
}

// FromTheme maps the theme's roles onto chart cells. Under NO_COLOR the
// theme's styles carry no color, so neither do these.
func FromTheme(th theme.Theme) Styles {
	return Styles{
		Line: th.Accent, Muted: th.Subtle, Last: th.Accent.Bold(true), Now: th.Warn.Bold(true),
		Info: th.Info, OK: th.Success, Warn: th.Warn, Error: th.Error,
		Heat: [5]lipgloss.Style{th.Subtle, th.Info, th.Accent, th.Warn, th.Error},
	}
}

// Look is a glyph set plus styles; every option type embeds one. The zero
// Look draws the ASCII set without color.
type Look struct {
	Glyphs Glyphs
	Styles Styles
}

// LookFor builds a Look from a theme: its styles, and the ASCII set when
// the theme draws ASCII, is the zero Theme, or the locale is CJK.
func LookFor(th theme.Theme) Look {
	g := Unicode()
	if th.Glyphs.ASCII || th.Renderer() == nil || render.CJKLocale() {
		g = ASCII()
	}
	return Look{Glyphs: g, Styles: FromTheme(th)}
}

// Bound is an optional fixed end of a scale; the zero Bound means "from the
// data".
type Bound struct {
	V   float64
	Set bool
}

// Fixed is a Bound at v.
func Fixed(v float64) Bound { return Bound{V: v, Set: true} }

// role names the style a cell is drawn with.
type role uint8

const (
	rNone role = iota
	rLine
	rMuted
	rLast
	rNow
	rInfo
	rOK
	rWarn
	rError
	rHeat0 // rHeat0+i is Styles.Heat[i]
)

func (s Styles) style(r role) (lipgloss.Style, bool) {
	switch r {
	case rLine:
		return s.Line, true
	case rMuted:
		return s.Muted, true
	case rLast:
		return s.Last, true
	case rNow:
		return s.Now, true
	case rInfo:
		return s.Info, true
	case rOK:
		return s.OK, true
	case rWarn:
		return s.Warn, true
	case rError:
		return s.Error, true
	}
	if r >= rHeat0 && int(r-rHeat0) < len(s.Heat) {
		return s.Heat[r-rHeat0], true
	}
	return lipgloss.Style{}, false
}

// cell is one terminal cell of a chart line.
type cell struct {
	g string
	r role
}

// paint renders cells, merging runs that share a role so a 96-cell row costs
// a few escape sequences rather than 96 (gcl renderCells).
func paint(cells []cell, s Styles) string {
	var b strings.Builder
	for i := 0; i < len(cells); {
		j := i
		var run strings.Builder
		for j < len(cells) && cells[j].r == cells[i].r {
			run.WriteString(cells[j].g)
			j++
		}
		if st, ok := s.style(cells[i].r); ok {
			b.WriteString(st.Render(run.String()))
		} else {
			b.WriteString(run.String())
		}
		i = j
	}
	return b.String()
}

// finite reports whether v can be placed on a scale.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// clamp01 clamps f to [0, 1]; NaN becomes 0.
func clamp01(f float64) float64 {
	if !(f > 0) {
		return 0
	}
	return min(f, 1)
}

// fitRow pads or cuts s to exactly w cells, with the set's marker.
func fitRow(s string, w int, g Glyphs) string {
	return render.PadRight(render.TruncateMarker(s, w, g.ellipsis()), w)
}
