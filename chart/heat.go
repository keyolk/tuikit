package chart

import (
	"math"

	"github.com/keyolk/tuikit/render"
)

// HeatOpts configures Heat. The zero value is ASCII, one cell per column,
// a zero floor and the data's peak.
type HeatOpts struct {
	Look
	// Min and Max fix the scale. Unset, Min is zero (or the data's minimum
	// when that is negative) and Max is the data's maximum, so a count grid
	// shows "none" only for zero.
	Min, Max Bound
	// CellWidth is cells per column (2 reads better for a 24-hour row);
	// zero is 1. It drops to 1 when the grid would not fit.
	CellWidth int
}

// Heat draws a grid (rows of columns, e.g. day × hour) as one line per row,
// each exactly width cells, on one scale for the whole grid. A cell at or
// below the floor is Heat[0]; anything above it is at least Heat[1], so a
// single event never hides; the peak is Heat[4]. NaN, infinite and missing
// cells (a short row) are Gap. labels, when given, head the rows in a
// column padded by cells (Hangul-safe) and capped at a third of the width.
//
// Columns that do not fit are merged by their maximum, as a Max sparkline
// does, so a hot hour survives the squeeze.
func Heat(values [][]float64, labels []string, width int, o HeatOpts) []string {
	if width <= 0 {
		return nil
	}
	g := o.Glyphs.set()
	rows := max(len(values), len(labels))
	labelW := 0
	for _, l := range labels {
		labelW = max(labelW, render.Width(l))
	}
	labelW = min(labelW, width/3)
	area := width
	if labelW > 0 {
		area -= labelW + 1
	}

	ncol := 0
	for _, r := range values {
		ncol = max(ncol, len(r))
	}
	cw := max(1, o.CellWidth)
	if ncol*cw > area {
		cw = 1
	}
	shown := min(ncol, area)

	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range values {
		for _, v := range r {
			if finite(v) {
				lo, hi = min(lo, v), max(hi, v)
			}
		}
	}
	lo = min(lo, 0)
	if o.Min.Set {
		lo = o.Min.V
	}
	if o.Max.Set {
		hi = o.Max.V
	}

	out := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		var row []float64
		if i < len(values) {
			row = make([]float64, ncol)
			for j := range row {
				row[j] = math.NaN()
			}
			copy(row, values[i])
		}
		cells := make([]cell, 0, width)
		for _, v := range resample(row, shown, Max, false) {
			c := cell{g: g.Gap, r: rNone}
			if finite(v) {
				lvl := heatLevel(v, lo, hi, len(g.Heat))
				c = cell{g: g.Heat[lvl], r: rHeat0 + role(lvl)}
			}
			for k := 0; k < cw; k++ {
				cells = append(cells, c)
			}
		}
		line := paint(cells, o.Styles)
		if labelW > 0 {
			label := ""
			if i < len(labels) {
				label = labels[i]
			}
			line = o.Styles.Muted.Render(fitRow(label, labelW, g)) + " " + line
		}
		out = append(out, render.PadRight(line, width))
	}
	return out
}

// heatLevel is 0 at or below lo, else 1..n-1 by the share of (lo, hi]
// rounded up, so any value above the floor is visible.
func heatLevel(v, lo, hi float64, n int) int {
	if v <= lo {
		return 0
	}
	if !(hi > lo) || v >= hi {
		return n - 1
	}
	lvl := int(math.Ceil((v - lo) / (hi - lo) * float64(n-1)))
	return max(1, min(lvl, n-1))
}
