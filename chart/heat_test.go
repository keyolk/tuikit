package chart

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
)

func TestHeatGolden(t *testing.T) {
	grid := [][]float64{{0, 1, 3, 5, 8}, {0, 0, math.NaN(), 2}}
	labels := []string{"월", "Tue"}
	cases := []struct {
		look Look
		want []string
	}{
		{uni, []string{"월  ⋅⋅░░▒▒▓▓██    ", "Tue ⋅⋅⋅⋅  ░░      "}},
		{asc, []string{"월  --..::++##    ", "Tue ----  ..      "}},
	}
	for _, c := range cases {
		got := Heat(grid, labels, 18, HeatOpts{Look: c.look, CellWidth: 2})
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("ascii=%v\n got %q\nwant %q", c.look.Glyphs.ASCII, got, c.want)
		}
	}
}

// Zero is the floor; anything above it is visible; the peak is the top.
func TestHeatLevels(t *testing.T) {
	for _, c := range []struct {
		v, lo, hi float64
		want      int
	}{{0, 0, 100, 0}, {-1, 0, 100, 0}, {0.1, 0, 100, 1}, {25, 0, 100, 1}, {26, 0, 100, 2}, {75, 0, 100, 3}, {99, 0, 100, 4}, {100, 0, 100, 4}, {200, 0, 100, 4}, {5, 5, 5, 0}, {6, 5, 5, 4}} {
		if got := heatLevel(c.v, c.lo, c.hi, 5); got != c.want {
			t.Errorf("heatLevel(%v, %v, %v) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
	// One scale for the grid: a quiet row stays quiet beside a busy one.
	got := Heat([][]float64{{1, 1}, {100, 100}}, nil, 2, HeatOpts{Look: asc})
	if got[0] != ".." || got[1] != "##" {
		t.Errorf("shared scale = %q", got)
	}
	// Fixed bounds clamp.
	got = Heat([][]float64{{5, 50, 500}}, nil, 3, HeatOpts{Look: asc, Min: Fixed(10), Max: Fixed(100)})
	if got[0] != "-:#" {
		t.Errorf("fixed bounds = %q", got)
	}
}

// 24 hours squeezed into 6 cells merge by max: the one busy hour survives.
func TestHeatSqueezeKeepsThePeak(t *testing.T) {
	row := make([]float64, 24)
	row[13] = 9
	got := Heat([][]float64{row}, []string{"수"}, 9, HeatOpts{Look: asc, CellWidth: 2})
	if got[0] != "수 ---#--" || render.Width(got[0]) != 9 {
		t.Errorf("squeezed = %q", got)
	}
}

func TestHeatLabelsAndColor(t *testing.T) {
	look := LookFor(colorTheme(theme.Unicode()))
	got := Heat([][]float64{{0, 4}, {4, 0}}, []string{"일요일", "월"}, 10, HeatOpts{Look: look})
	for i, r := range got {
		if render.Width(r) != 10 {
			t.Errorf("row %d is %d cells", i, render.Width(r))
		}
		// The cells start in one column whatever the label's script.
		if c := ansi.Cut(r, 0, 4); render.Width(ansi.Strip(c)) != 4 {
			t.Errorf("row %d label column = %q", i, ansi.Strip(c))
		}
	}
	if !strings.Contains(got[0], "38;2;239;68;68") {
		t.Errorf("the peak takes the hottest role: %q", got[0])
	}
	// Labels past a third of the width are cut with the ellipsis.
	if got := Heat([][]float64{{1}}, []string{"월요일입니다"}, 9, HeatOpts{Look: asc}); ansi.Strip(got[0]) != "월~ #    " {
		t.Errorf("cut label = %q", got)
	}
	// More labels than rows draw empty rows; empty input is blank rows.
	if got := Heat(nil, []string{"a", "b"}, 4, HeatOpts{}); len(got) != 2 || got[1] != "b   " {
		t.Errorf("label-only rows = %q", got)
	}
}
