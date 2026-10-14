package chart

import (
	"math"
	"strconv"
	"time"

	"github.com/keyolk/tuikit/render"
)

// Kind is a span's severity. A cell several spans share takes the highest
// kind's style. Kind is color only: the glyph says how many spans share the
// cell, so a kind that must survive NO_COLOR belongs in a lane's label.
type Kind uint8

const (
	Normal Kind = iota // Styles.Line
	Info               // Styles.Info
	OK                 // Styles.OK
	Warn               // Styles.Warn
	Error              // Styles.Error
)

func (k Kind) role() role {
	return [...]role{rLine, rInfo, rOK, rWarn, rError}[min(int(k), int(Error))]
}

// Span is an interval [Start, End). An End at or before Start is an instant,
// which still marks one cell. Label names the span's lane (see Lanes).
type Span struct {
	Start, End time.Time
	Kind       Kind
	Label      string
}

// TimelineOpts configures Timeline, Ruler and Lanes.
type TimelineOpts struct {
	Look
	// Now is drawn as the Now glyph when it falls inside the window; the
	// zero time draws nothing.
	Now time.Time
	// Peak is the span count the density ramp tops out at. Zero scales to
	// the busiest cell drawn (for Lanes, across every lane), so one glyph
	// means one load on every row; pass the peak of a wider range to keep
	// that true across separately drawn rows (gcl timelinePeak).
	Peak int
	// LabelMax caps the Lanes label column in cells; zero is a third of
	// the width.
	LabelMax int
	// Ruler, when positive, appends a Ruler row with this tick step under
	// the lanes, aligned with their timelines.
	Ruler time.Duration
	// TickFormat is the Ruler's time layout. Empty picks one by step: the
	// hour ("9") for whole hours, "01-02" for whole days, "15:04" otherwise.
	TickFormat string
}

// cols maps [start, end) onto the cells of a width-cell window [from, to).
// The end is exclusive, so a span ending exactly on a cell boundary does not
// light the next cell, and a span shorter than a cell still marks the one it
// starts in. before/after report that it runs past the window's edge. ok is
// false when it misses the window (gcl timelineSpan).
func cols(s Span, from, to time.Time, width int) (lo, hi int, before, after, ok bool) {
	if width <= 0 || !to.After(from) {
		return 0, 0, false, false, false
	}
	start, end := s.Start, s.End
	if !end.After(start) {
		end = start
	}
	instant := end.Equal(start)
	if !start.Before(to) || end.Before(from) || (!instant && !end.After(from)) {
		return 0, 0, false, false, false
	}
	span := float64(to.Sub(from))
	col := func(t time.Time) float64 { return float64(t.Sub(from)) / span * float64(width) }
	// The epsilon keeps a boundary that float division lands a hair off
	// (6h of 24h over 48 cells = 11.999…) on the cell it names.
	lo = int(math.Floor(col(start) + 1e-9))
	hi = int(math.Ceil(col(end)-1e-9)) - 1
	lo = max(0, min(lo, width-1))
	hi = max(lo, min(hi, width-1))
	return lo, hi, start.Before(from), end.After(to), true
}

// Density counts, per cell, how many spans are in effect somewhere inside
// that cell's slice of [from, to): overlapping spans merge into a higher
// count rather than overwriting each other. Nil when width <= 0.
func Density(spans []Span, from, to time.Time, width int) []int {
	if width <= 0 {
		return nil
	}
	out := make([]int, width)
	for _, s := range spans {
		lo, hi, _, _, ok := cols(s, from, to, width)
		if !ok {
			continue
		}
		for c := lo; c <= hi; c++ {
			out[c]++
		}
	}
	return out
}

// densityLevel maps a count onto n ramp steps scaled to peak, rounding to
// the nearest step; -1 is "nothing here". A peak of one is the top step, so
// a lone span is solid (gcl densityLevel).
func densityLevel(c, peak, n int) int {
	if c <= 0 {
		return -1
	}
	top := n - 1
	if peak <= 1 {
		return top
	}
	lvl := ((c-1)*top + (peak-1)/2) / (peak - 1)
	return max(0, min(lvl, top))
}

// Timeline draws spans on one row of exactly width cells for [from, to).
// Each cell's glyph is its density level; the cell takes the style of the
// highest Kind in it. A span clipped by the window's edge shows Before or
// After in the edge cell, so a window crossing midnight is never misread as
// ending there. Now, inside the window, is drawn over its cell.
func Timeline(spans []Span, from, to time.Time, width int, o TimelineOpts) string {
	if width <= 0 {
		return ""
	}
	return paint(timelineCells(spans, from, to, width, o, o.Peak), o.Styles)
}

func timelineCells(spans []Span, from, to time.Time, width int, o TimelineOpts, peak int) []cell {
	g := o.Glyphs.set()
	density := Density(spans, from, to, width)
	if peak <= 0 {
		for _, c := range density {
			peak = max(peak, c)
		}
	}
	kinds := make([]Kind, width)
	cells := make([]cell, width)
	for i := range cells {
		cells[i] = cell{g: g.Track, r: rMuted}
	}
	for _, s := range spans {
		lo, hi, _, _, ok := cols(s, from, to, width)
		if !ok {
			continue
		}
		for c := lo; c <= hi; c++ {
			kinds[c] = max(kinds[c], s.Kind)
		}
	}
	for c, n := range density {
		if lvl := densityLevel(n, peak, len(g.Density)); lvl >= 0 {
			cells[c] = cell{g: g.Density[lvl], r: kinds[c].role()}
		}
	}
	for _, s := range spans {
		lo, hi, before, after, ok := cols(s, from, to, width)
		if !ok {
			continue
		}
		if before && lo == 0 {
			cells[0] = cell{g: g.Before, r: kinds[0].role()}
		}
		if after && hi == width-1 {
			cells[hi] = cell{g: g.After, r: kinds[hi].role()}
		}
	}
	if c := nowCol(o.Now, from, to, width); c >= 0 {
		cells[c] = cell{g: g.Now, r: rNow}
	}
	return cells
}

// nowCol is the cell now falls in, or -1 outside [from, to) or when zero.
func nowCol(now, from, to time.Time, width int) int {
	if now.IsZero() || now.Before(from) || !now.Before(to) || width <= 0 {
		return -1
	}
	c := int(float64(now.Sub(from)) / float64(to.Sub(from)) * float64(width))
	return min(c, width-1)
}

// maxTicks bounds the tick walk: a step so small that the window holds more
// ticks than this could never be labelled anyway, and draws none.
const maxTicks = 4096

// Ruler is a time axis of exactly width cells for [from, to): a label at
// each multiple of tickEvery (aligned to the wall clock of from's location;
// whole days to midnight), starting in the tick's cell. A label that would
// touch the previous one or run past the edge is skipped, so labels never
// overlap or get cut. Now is marked when its cell is free.
func Ruler(from, to time.Time, width int, tickEvery time.Duration, o TimelineOpts) string {
	if width <= 0 {
		return ""
	}
	g := o.Glyphs.set()
	cells := make([]cell, width)
	for i := range cells {
		cells[i] = cell{g: " "}
	}
	if tickEvery > 0 && to.After(from) && to.Sub(from)/tickEvery <= maxTicks {
		free := 0 // the first cell a label may start in
		for t, i := firstTick(from, tickEvery), 0; t.Before(to) && i <= maxTicks; t, i = nextTick(t, tickEvery), i+1 {
			label := tickLabel(t, tickEvery, o.TickFormat)
			c := int(math.Floor(float64(t.Sub(from))/float64(to.Sub(from))*float64(width) + 1e-9))
			w := render.Width(label)
			if c < free || c+w > width || w == 0 {
				continue
			}
			// A label is one run of cells; only its first carries text, the
			// rest are empty so the row stays width cells.
			cells[c] = cell{g: label, r: rMuted}
			for k := 1; k < w; k++ {
				cells[c+k] = cell{g: "", r: rMuted}
			}
			free = c + w + 1
		}
	}
	if c := nowCol(o.Now, from, to, width); c >= 0 && cells[c].g == " " {
		cells[c] = cell{g: g.Now, r: rNow}
	}
	return paint(cells, o.Styles)
}

func firstTick(from time.Time, step time.Duration) time.Time {
	if step%(24*time.Hour) == 0 {
		y, m, d := from.Date()
		t := time.Date(y, m, d, 0, 0, 0, 0, from.Location())
		for t.Before(from) {
			t = nextTick(t, step)
		}
		return t
	}
	_, off := from.Zone()
	shift := time.Duration(off) * time.Second
	t := from.Add(shift).Truncate(step).Add(-shift)
	if t.Before(from) {
		t = t.Add(step)
	}
	return t
}

func nextTick(t time.Time, step time.Duration) time.Time {
	if step%(24*time.Hour) == 0 {
		// AddDate, not Add: a DST day is 23 or 25 hours long.
		return t.AddDate(0, 0, int(step/(24*time.Hour)))
	}
	return t.Add(step)
}

func tickLabel(t time.Time, step time.Duration, layout string) string {
	switch {
	case layout != "":
		return t.Format(layout)
	case step%(24*time.Hour) == 0:
		return t.Format("01-02")
	case step%time.Hour == 0:
		return strconv.Itoa(t.Hour())
	}
	return t.Format("15:04")
}

// Lanes draws one Timeline row per lane — spans grouped by Label, lanes in
// first-seen order — behind a label column, each row exactly width cells.
// Labels are padded by cells, so Hangul labels align, and cut with the
// ellipsis past LabelMax. All lanes share one density scale. With too little
// room for a label column the rows are timelines alone.
func Lanes(spans []Span, from, to time.Time, width int, o TimelineOpts) []string {
	if width <= 0 {
		return nil
	}
	g := o.Glyphs.set()
	var order []string
	lanes := map[string][]Span{}
	for _, s := range spans {
		if _, seen := lanes[s.Label]; !seen {
			order = append(order, s.Label)
		}
		lanes[s.Label] = append(lanes[s.Label], s)
	}
	labelW := 0
	for _, l := range order {
		labelW = max(labelW, render.Width(l))
	}
	maxW := o.LabelMax
	if maxW <= 0 {
		maxW = width / 3
	}
	labelW = min(labelW, maxW)
	tlW := width - labelW - 1
	if labelW == 0 || tlW < 1 {
		labelW, tlW = 0, width
	}
	peak := o.Peak
	if peak <= 0 {
		for _, l := range order {
			for _, c := range Density(lanes[l], from, to, tlW) {
				peak = max(peak, c)
			}
		}
	}
	gutter := func(label string) string {
		if labelW == 0 {
			return ""
		}
		return o.Styles.Muted.Render(fitRow(label, labelW, g)) + " "
	}
	out := make([]string, 0, len(order)+1)
	for _, l := range order {
		out = append(out, gutter(l)+paint(timelineCells(lanes[l], from, to, tlW, o, peak), o.Styles))
	}
	if o.Ruler > 0 {
		out = append(out, gutter("")+Ruler(from, to, tlW, o.Ruler, o))
	}
	return out
}
