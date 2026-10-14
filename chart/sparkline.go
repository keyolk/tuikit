package chart

import "math"

// Agg is how several samples that land in one cell become one value.
type Agg uint8

const (
	// Mean averages a cell's samples (ccx): the shape of the series.
	Mean Agg = iota
	// Max keeps a cell's peak (ccproxy): a spike survives the downscale.
	Max
)

// SparkOpts configures Sparkline. The zero value is ASCII, mean buckets,
// automatic scale, left-aligned.
type SparkOpts struct {
	Look
	Agg Agg
	// Min and Max fix the scale's ends; unset ends follow the data. Values
	// outside a fixed range are clamped to it. Fixed(0) as Min gives a zero
	// baseline, so a quiet series stays low instead of filling the height.
	Min, Max Bound
	// Last draws the Last glyph right after the last point, in one cell
	// taken from the width (only when width >= 2).
	Last bool
	// Right aligns a short series to the right edge, so the newest point
	// sits in the last cell; by default it starts at the left edge.
	Right bool
	// Stretch spreads a series shorter than the width across every cell by
	// repeating points. By default each point is one cell and the rest is
	// blank: a sparkline is a hint, not a stretched bar chart (ccproxy).
	Stretch bool
}

// Sparkline draws values as one line of exactly width cells.
//
// More points than cells are bucketed so every cell covers an equal share of
// the series (Agg picks mean or max). Fewer points are drawn one per cell
// with blank padding unless Stretch is set. A NaN or infinite value is a Gap
// cell, as is a bucket with nothing finite in it. A constant series draws a
// flat mid-level line rather than dividing by zero.
func Sparkline(values []float64, width int, o SparkOpts) string {
	if width <= 0 {
		return ""
	}
	g := o.Glyphs.set()
	dataW := width
	if o.Last && width >= 2 && len(values) > 0 {
		dataW--
	}
	samples := resample(values, dataW, o.Agg, o.Stretch)
	lo, hi := scale(samples, o.Min, o.Max)

	cells := make([]cell, 0, width)
	pad := width - len(samples)
	if o.Last && dataW < width && len(samples) > 0 {
		pad--
	}
	blank := make([]cell, pad)
	for i := range blank {
		blank[i] = cell{g: " "}
	}
	if o.Right {
		cells = append(cells, blank...)
	}
	for _, v := range samples {
		if !finite(v) {
			cells = append(cells, cell{g: g.Gap, r: rMuted})
			continue
		}
		cells = append(cells, cell{g: g.Levels[level(v, lo, hi, len(g.Levels))], r: rLine})
	}
	if o.Last && dataW < width && len(samples) > 0 {
		cells = append(cells, cell{g: g.Last, r: rLast})
	}
	if !o.Right {
		cells = append(cells, blank...)
	}
	return paint(cells, o.Styles)
}

// resample maps values onto at most w cells. With more values than cells,
// cell i covers values[i*n/w : (i+1)*n/w], which is never empty and spreads
// the remainder evenly (ccx's ceil-sized buckets left the last cells empty).
func resample(values []float64, w int, agg Agg, stretch bool) []float64 {
	n := len(values)
	switch {
	case n == 0 || w <= 0:
		return nil
	case n == w:
		return values
	case n < w:
		if !stretch {
			return values
		}
		out := make([]float64, w)
		for i := range out {
			out[i] = values[i*n/w]
		}
		return out
	}
	out := make([]float64, w)
	for i := range out {
		out[i] = bucket(values[i*n/w:(i+1)*n/w], agg)
	}
	return out
}

// bucket aggregates the finite values in vs, or NaN when there are none.
func bucket(vs []float64, agg Agg) float64 {
	acc, k := 0.0, 0
	for _, v := range vs {
		if !finite(v) {
			continue
		}
		switch {
		case k == 0:
			acc = v
		case agg == Max:
			acc = max(acc, v)
		default:
			acc += v
		}
		k++
	}
	if k == 0 {
		return math.NaN()
	}
	if agg == Mean {
		acc /= float64(k)
	}
	return acc
}

// scale is the range values are drawn against: fixed ends where set, the
// finite extremes otherwise.
func scale(values []float64, minB, maxB Bound) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range values {
		if finite(v) {
			lo, hi = min(lo, v), max(hi, v)
		}
	}
	if minB.Set {
		lo = minB.V
	}
	if maxB.Set {
		hi = maxB.V
	}
	return lo, hi
}

// level maps v onto n equal bins of [lo, hi]; the top of the range is the
// top bin. An empty range (a constant series) is the middle; a value outside
// an empty or inverted range clamps to the end it is past.
func level(v, lo, hi float64, n int) int {
	if !(hi > lo) {
		switch {
		case v < lo:
			return 0
		case v > hi:
			return n - 1
		}
		return n/2 - 1
	}
	i := int(clamp01((v-lo)/(hi-lo)) * float64(n))
	return min(i, n-1)
}
