package surface

import (
	"slices"
	"strings"

	"github.com/keyolk/tuikit/render"
)

// share splits total cells among weights, largest remainder last so the sum
// is exact. Zero weights count as 1.
func share(total int, weights []float64) []int {
	out := make([]int, len(weights))
	if len(weights) == 0 || total <= 0 {
		return out
	}
	weights = slices.Clone(weights)
	var sum float64
	for i, w := range weights {
		if w <= 0 {
			weights[i] = 1
		}
		sum += weights[i]
	}
	used := 0
	for i, w := range weights {
		out[i] = int(float64(total) * w / sum)
		used += out[i]
	}
	for i := 0; used < total; i = (i + 1) % len(out) {
		out[i]++
		used++
	}
	return out
}

// block returns s as exactly w×h cells: h lines, each padded or cut to w.
// Every component draws through it, so a neighbor can be placed by line
// concatenation without measuring.
func block(s string, w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	lines := strings.Split(render.FitLines(s, w, h), "\n")
	for i, l := range lines {
		lines[i] = render.PadRight(l, w)
	}
	return strings.Join(lines, "\n")
}

// beside joins equal-height blocks left to right.
func beside(blocks ...string) string {
	var rows [][]string
	h := 0
	for _, b := range blocks {
		ls := strings.Split(b, "\n")
		rows = append(rows, ls)
		h = max(h, len(ls))
	}
	out := make([]string, h)
	for i := range out {
		var sb strings.Builder
		for _, ls := range rows {
			if i < len(ls) {
				sb.WriteString(ls[i])
			}
		}
		out[i] = sb.String()
	}
	return strings.Join(out, "\n")
}

func blank(w, h int) string { return block("", w, h) }
