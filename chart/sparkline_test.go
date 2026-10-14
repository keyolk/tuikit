package chart

import (
	"math"
	"testing"
)

var (
	uni = Look{Glyphs: Unicode()}
	asc = Look{Glyphs: ASCII()}
	nan = math.NaN()
)

func TestSparklineGolden(t *testing.T) {
	ramp := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	cases := []struct {
		name   string
		values []float64
		width  int
		o      SparkOpts
		want   string
	}{
		{"unicode ramp", ramp, 8, SparkOpts{Look: uni}, "▁▂▃▄▅▆▇█"},
		{"ascii ramp", ramp, 8, SparkOpts{Look: asc}, "__.-==*#"},
		// narwhal: scaled to its own peak from a zero baseline. Zero is the
		// lowest block, not a blank: a blank means no sample.
		{"zero baseline", []float64{0, 1, 2, 4}, 4, SparkOpts{Look: uni, Min: Fixed(0)}, "▁▃▅█"},
		{"fixed range clamps", []float64{-5, 50, 150}, 3, SparkOpts{Look: uni, Min: Fixed(0), Max: Fixed(100)}, "▁▅█"},
		{"gap", []float64{1, nan, 3}, 3, SparkOpts{Look: uni}, "▁ █"},
		{"infinite is a gap", []float64{1, math.Inf(1), 3}, 3, SparkOpts{Look: asc}, "_ #"},
		{"all gaps", []float64{nan, nan}, 2, SparkOpts{Look: uni}, "  "},
		{"empty is blank", nil, 3, SparkOpts{Look: uni}, "   "},
		// ccproxy: fewer points than cells pad; they do not stretch.
		{"short pads right", []float64{0, 0, 0}, 6, SparkOpts{Look: uni}, "▄▄▄   "},
		{"short aligned right", []float64{1, 2}, 5, SparkOpts{Look: uni, Right: true}, "   ▁█"},
		{"stretch when asked", []float64{1, 2}, 4, SparkOpts{Look: uni, Stretch: true}, "▁▁██"},
		{"last marker", []float64{1, 2, 3}, 6, SparkOpts{Look: uni, Last: true}, "▁▅█◂  "},
		{"last marker right", []float64{1, 2, 3}, 6, SparkOpts{Look: asc, Last: true, Right: true}, "  _=#<"},
		{"last marker takes a cell", []float64{1, 2, 3, 4}, 4, SparkOpts{Look: uni, Last: true, Agg: Max}, "▁▃█◂"},
		{"width 1", ramp, 1, SparkOpts{Look: uni}, "▄"},
		{"width 1 has no room for the marker", ramp, 1, SparkOpts{Look: uni, Last: true}, "▄"},
		{"width 2", ramp, 2, SparkOpts{Look: uni}, "▁█"},
		{"width 3", ramp, 3, SparkOpts{Look: asc}, "_-#"},
	}
	for _, c := range cases {
		if got := Sparkline(c.values, c.width, c.o); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// A constant series has no range to divide by; it draws flat at mid-level.
func TestSparklineConstantIsFlatMidLevel(t *testing.T) {
	for _, vs := range [][]float64{{3, 3, 3}, {7}, {0, 0}, {-2, -2, nan, -2}} {
		u, a := Sparkline(vs, len(vs), SparkOpts{Look: uni}), Sparkline(vs, len(vs), SparkOpts{Look: asc})
		for i, r := range []rune(u) {
			if vs[i] == vs[i] && r != '▄' { // NaN != NaN
				t.Errorf("constant %v drew %q", vs, u)
			}
		}
		for i, r := range a {
			if vs[i] == vs[i] && r != '-' {
				t.Errorf("constant %v drew %q in ascii", vs, a)
			}
		}
	}
	// Fixed to one value: below it is the floor, above it the top.
	if got := Sparkline([]float64{1, 5, 9}, 3, SparkOpts{Look: uni, Min: Fixed(5), Max: Fixed(5)}); got != "▁▄█" {
		t.Errorf("degenerate fixed range = %q", got)
	}
}

// ccproxy TestRenderSparklineDownsamplesByMax: a spike survives Max.
func TestSparklineDownsamplesByMax(t *testing.T) {
	values := []float64{0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := Sparkline(values, 4, SparkOpts{Look: uni, Agg: Max}); got != "▁█▁▁" {
		t.Errorf("max = %q, want the spike bucket full", got)
	}
	// The mean of the same buckets also puts the spike's bucket on top, the
	// others at the floor; Mean is the shape, Max the peak.
	if got := Sparkline(values, 4, SparkOpts{Look: uni}); got != "▁█▁▁" {
		t.Errorf("mean = %q", got)
	}
	// Mean and Max differ once a bucket is mixed.
	mixed := []float64{0, 8, 4, 4}
	if m, x := Sparkline(mixed, 2, SparkOpts{Look: uni, Min: Fixed(0), Max: Fixed(8)}),
		Sparkline(mixed, 2, SparkOpts{Look: uni, Agg: Max, Min: Fixed(0), Max: Fixed(8)}); m != "▅▅" || x != "█▅" {
		t.Errorf("mean %q max %q", m, x)
	}
}

// ccx's ceil-sized buckets left trailing cells empty (100 points into 30
// cells made 25 buckets). Every cell must cover a share of the series.
func TestResampleCoversEveryCell(t *testing.T) {
	for _, c := range []struct{ n, w int }{{100, 30}, {9, 8}, {10, 4}, {31, 30}, {1000, 7}} {
		vs := make([]float64, c.n)
		for i := range vs {
			vs[i] = float64(i)
		}
		got := resample(vs, c.w, Mean, false)
		if len(got) != c.w {
			t.Fatalf("%d into %d: %d cells", c.n, c.w, len(got))
		}
		for i := 1; i < len(got); i++ {
			if !(got[i] > got[i-1]) {
				t.Errorf("%d into %d: cell %d (%v) not past cell %d (%v)", c.n, c.w, i, got[i], i-1, got[i-1])
			}
		}
		if got[0] > float64(c.n)/float64(c.w) || got[c.w-1] < float64(c.n)-float64(c.n)/float64(c.w)-1 {
			t.Errorf("%d into %d: ends %v..%v do not reach the series' ends", c.n, c.w, got[0], got[c.w-1])
		}
	}
	// 10 into 4 is buckets of 2,3,2,3 — the remainder spread, not piled.
	if got := resample([]float64{1, 1, 2, 2, 2, 3, 3, 4, 4, 4}, 4, Mean, false); got[0] != 1 || got[1] != 2 || got[2] != 3 || got[3] != 4 {
		t.Errorf("10 into 4 = %v", got)
	}
	// A bucket of only NaN is a gap; NaN beside numbers is ignored.
	got := resample([]float64{nan, nan, 1, nan}, 2, Mean, false)
	if !math.IsNaN(got[0]) || got[1] != 1 {
		t.Errorf("NaN buckets = %v", got)
	}
}
