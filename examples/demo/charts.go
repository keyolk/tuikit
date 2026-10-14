package main

import (
	"hash/fnv"

	"github.com/keyolk/tuikit/chart"
)

// chartRows is the small chart row under the detail: the selected item's
// load over the last 48 samples and its disk use, from made-up numbers.
// The chart glyphs carry the reading; the theme's roles only color it, so
// the row reads the same under NO_COLOR.
func (m model) chartRows(w int) string {
	if m.rows.Len() == 0 {
		return ""
	}
	look := chart.LookFor(m.th) // ASCII under a CJK locale, where blocks draw two cells
	h := fnv.New32a()
	h.Write([]byte(m.rows.SelectedID()))
	seed := int(h.Sum32() % 97)
	load := make([]float64, 48)
	for i := range load {
		load[i] = float64((i*7 + seed*13) % 17)
	}
	const label = 5 // "load " and "disk "
	spark := chart.Sparkline(load, w-label, chart.SparkOpts{Look: look, Agg: chart.Max, Min: chart.Fixed(0), Last: true})
	disk := chart.Meter(float64(30+seed%70), 100, w-label, chart.BarOpts{Look: look, Label: "/var"})
	return m.th.Muted.Render("load ") + spark + "\n" + m.th.Muted.Render("disk ") + disk
}
