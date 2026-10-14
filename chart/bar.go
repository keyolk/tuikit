package chart

import (
	"fmt"
	"math"

	"github.com/keyolk/tuikit/render"
)

// BarOpts configures Bar and Meter. The zero value is an ASCII bar without
// bands or suffix.
type BarOpts struct {
	Look
	// Warn and Error are band thresholds on the fraction: at or above Error
	// the bar is in its error band, else at or above Warn in its warn band,
	// else ok. Setting either turns bands on: the bar is then led by the
	// band's glyph (OK/Warn/Error) and filled in the band's style, so the
	// band reads with color stripped.
	Warn, Error Bound
	// Percent appends the fraction as " 42%", rounded down like the bar.
	Percent bool
	// Label is appended after the percent, cut with the ellipsis to fit.
	Label string
}

// eps absorbs float error before rounding down: 0.29*100 is 28.999…, which
// must still read 29%. It is far too small to make an unfinished bar full.
const eps = 1e-9

// band is a Meter band.
type band uint8

const (
	bandNone band = iota
	bandOK
	bandWarn
	bandError
)

// Bar draws frac (clamped to [0, 1]; NaN is 0) as exactly width cells, at
// 1/8-cell resolution with the Unicode set and half-cell with ASCII. It is
// full only at 1, so work that is not done never looks done (okx).
//
// Layout, as room allows: [band glyph] [Open]cells[Close] [percent] [label].
// The bar keeps at least one cell; the label goes first, then the percent,
// then the brackets, then the space after the band glyph. At width 1 a
// banded bar is its band glyph alone, which says more than one cell of fill.
func Bar(frac float64, width int, o BarOpts) string {
	if width <= 0 {
		return ""
	}
	g := o.Glyphs.set()
	unknown := math.IsNaN(frac)
	frac = clamp01(frac)
	b := o.band(frac)

	var lead []cell
	if b != bandNone {
		mark := map[band]cell{bandOK: {g.OK, rOK}, bandWarn: {g.Warn, rWarn}, bandError: {g.Error, rError}}[b]
		lead = append(lead, mark)
		if width == 1 {
			return paint(lead, o.Styles)
		}
		if width >= 4 {
			lead = append(lead, cell{g: " "})
		}
	}
	avail := width - len(lead)

	const minBar = 3 // with brackets: one cell of fill between them
	suffix := ""
	if o.Percent {
		pct := "?%"
		if !unknown {
			pct = fmt.Sprintf("%d%%", int(frac*100+eps))
		}
		pct = " " + fmt.Sprintf("%4s", pct)
		if avail-len(pct) >= minBar {
			suffix = pct
		}
	}
	if o.Label != "" {
		room := (avail - len(suffix) - 1) / 2 // the label never takes more than the bar
		if room >= 2 {
			suffix += " " + render.TruncateMarker(o.Label, room, g.ellipsis())
		}
	}
	barW := avail - render.Width(suffix)

	cells := append(lead, barCells(frac, barW, g, b)...)
	out := paint(cells, o.Styles)
	if suffix != "" {
		out += o.Styles.Muted.Render(suffix)
	}
	return out
}

// Meter is Bar for a usage reading: value of limit, with the percent shown
// and bands at 75% and 90% unless BarOpts sets its own. A limit of zero or
// less, or a NaN value, is an unknown reading: an empty bar and " ?%".
func Meter(value, limit float64, width int, o BarOpts) string {
	if !o.Warn.Set && !o.Error.Set {
		o.Warn, o.Error = Fixed(0.75), Fixed(0.9)
	}
	o.Percent = true
	frac := math.NaN()
	if limit > 0 {
		frac = value / limit
	}
	return Bar(frac, width, o)
}

func (o BarOpts) band(frac float64) band {
	switch {
	case !o.Warn.Set && !o.Error.Set:
		return bandNone
	case o.Error.Set && frac >= o.Error.V:
		return bandError
	case o.Warn.Set && frac >= o.Warn.V:
		return bandWarn
	}
	return bandOK
}

// barCells is the bar itself in w cells, bracketed when w >= 3.
func barCells(frac float64, w int, g Glyphs, b band) []cell {
	if w <= 0 {
		return nil
	}
	fill := map[band]role{bandNone: rLine, bandOK: rOK, bandWarn: rWarn, bandError: rError}[b]
	n := w
	if w >= 3 {
		n -= 2
	}
	eighths := int(frac*float64(n)*8 + eps)
	if frac >= 1 {
		eighths = n * 8
	}
	cells := make([]cell, 0, w)
	if w >= 3 {
		cells = append(cells, cell{g: g.Open, r: rMuted})
	}
	for i := 0; i < n; i++ {
		switch k := eighths - i*8; {
		case k >= 8:
			cells = append(cells, cell{g: g.Full, r: fill})
		case k > 0:
			cells = append(cells, cell{g: g.Partial[k-1], r: fill})
		default:
			cells = append(cells, cell{g: g.Empty, r: rMuted})
		}
	}
	if w >= 3 {
		cells = append(cells, cell{g: g.Close, r: rMuted})
	}
	return cells
}
