package chart

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
)

func TestTimelineGolden(t *testing.T) {
	spans := []Span{
		{Start: hr(2), End: hr(8)},
		{Start: hr(6), End: hr(12)},
		{Start: hr(20), End: hr(28)},
		{Start: hr(-1), End: hr(1)},
		{Start: hr(15.1), End: hr(15.1)},
	}
	o := TimelineOpts{Now: hr(13.5)}
	o.Look = uni
	if got, want := Timeline(spans, day, hr(24), 24, o), "‹ ░░░░██░░░░ ╎ ░    ░░░›"; got != want {
		t.Errorf("unicode\n got %q\nwant %q", got, want)
	}
	o.Look = asc
	if got, want := Timeline(spans, day, hr(24), 24, o), "< ....##.... | .    ...>"; got != want {
		t.Errorf("ascii\n got %q\nwant %q", got, want)
	}
}

// gcl TestTimelineBarsAlignWithTheClock: spans land on the hours they claim,
// with an exclusive end, at every tier width.
func TestTimelineSpansAlignWithTheClock(t *testing.T) {
	for _, width := range []int{24, 48, 96} {
		for _, c := range []struct{ sh, eh float64 }{{2, 8}, {6, 12}, {20, 22}, {0, 24}} {
			lo, hi, _, _, ok := cols(Span{Start: hr(c.sh), End: hr(c.eh)}, day, hr(24), width)
			if !ok {
				t.Fatalf("%v-%vh missed the window", c.sh, c.eh)
			}
			if want := int(c.sh) * width / 24; lo != want {
				t.Errorf("width %d: %v-%vh starts at cell %d, want %d", width, c.sh, c.eh, lo, want)
			}
			if want := int(c.eh)*width/24 - 1; hi != want {
				t.Errorf("width %d: %v-%vh ends at cell %d, want %d (exclusive)", width, c.sh, c.eh, hi, want)
			}
		}
	}
}

// Overlapping spans merge into a denser cell instead of overwriting, and the
// cell takes the most severe kind's style.
func TestTimelineOverlapMerges(t *testing.T) {
	spans := []Span{
		{Start: hr(0), End: hr(12), Kind: Normal},
		{Start: hr(6), End: hr(18), Kind: Error},
		{Start: hr(8), End: hr(10), Kind: Warn},
	}
	d := Density(spans, day, hr(24), 12)
	if want := []int{1, 1, 1, 2, 3, 2, 1, 1, 1, 0, 0, 0}; !slices.Equal(d, want) {
		t.Fatalf("density = %v, want %v", d, want)
	}
	if got := Timeline(spans, day, hr(24), 12, TimelineOpts{Look: asc}); got != "...+#+...   " {
		t.Errorf("merged row = %q", got)
	}
	look := LookFor(colorTheme(theme.Unicode()))
	got := Timeline(spans, day, hr(24), 12, TimelineOpts{Look: look})
	// Cells 3..8 hold the Error span; the first three only the Normal one.
	if !strings.Contains(got, "38;2;239;68;68m▓█▓") || !strings.Contains(got, "38;2;167;139;250m░░░") {
		t.Errorf("styles by kind missing: %q", got)
	}
	if strings.Contains(got, "245;158;11") {
		t.Errorf("the Warn span is inside an Error one; Error wins: %q", got)
	}
}

// A span shorter than one cell, or an instant, still marks the cell it is
// in; one ending exactly on a boundary does not light the next cell.
func TestTimelineSubCellSpans(t *testing.T) {
	o := TimelineOpts{Look: asc}
	if got := Timeline([]Span{{Start: hr(5.2), End: hr(5.3)}}, day, hr(24), 24, o); got != "     #                  " {
		t.Errorf("six-minute span = %q", got)
	}
	if got := Timeline([]Span{{Start: hr(23.99), End: hr(23.99)}}, day, hr(24), 24, o); got != "                       #" {
		t.Errorf("instant = %q", got)
	}
	if got := Timeline([]Span{{Start: hr(3), End: hr(1)}}, day, hr(24), 24, o); got != "   #                    " {
		t.Errorf("end before start is an instant at start: %q", got)
	}
	if got := Timeline([]Span{{Start: hr(1), End: hr(2)}}, day, hr(24), 24, o); got != " #                      " {
		t.Errorf("an hour span on hour cells = %q", got)
	}
	for _, s := range []Span{{Start: hr(24), End: hr(25)}, {Start: hr(-2), End: hr(0)}, {Start: hr(24), End: hr(24)}} {
		if got := Timeline([]Span{s}, day, hr(24), 24, o); strings.TrimSpace(got) != "" {
			t.Errorf("span %v-%v is outside [from,to) but drew %q", s.Start, s.End, got)
		}
	}
}

// gcl TestTimelineMarksMidnightOverflow: a window crossing midnight says so
// on both days. A lone span is the top density level.
func TestTimelineMarksOverflow(t *testing.T) {
	s := []Span{{Start: hr(21), End: hr(28.5)}}
	if got := Timeline(s, day, hr(24), 24, TimelineOpts{Look: uni}); !strings.HasSuffix(got, "██›") {
		t.Errorf("own day = %q, want an outflow arrow", got)
	}
	next := Timeline(s, hr(24), hr(48), 24, TimelineOpts{Look: uni})
	if !strings.HasPrefix(next, "‹███") || render.Width(strings.TrimRight(next, " ")) != 5 {
		t.Errorf("next day = %q, want an inflow arrow and 04:30 as the last cell", next)
	}
}

// gcl TestDensityCountsWindowsInheritedFromYesterday and
// TestDensityDistinguishesQuietAndStackedHours.
func TestDensityLevels(t *testing.T) {
	d := Density([]Span{{Start: hr(21), End: hr(28.5)}}, hr(24), hr(48), 24)
	if d[0] != 1 || d[4] != 1 || d[5] != 0 {
		t.Errorf("inherited window: %v", d)
	}
	spans := []Span{{Start: hr(1), End: hr(2)}}
	for i := 0; i < 8; i++ {
		spans = append(spans, Span{Start: hr(10), End: hr(12)})
	}
	got := Timeline(spans, day, hr(24), 24, TimelineOpts{Look: uni})
	if r := []rune(got); r[1] != '░' || r[10] != '█' || r[0] != ' ' {
		t.Errorf("1 and 8 concurrent must differ and empty stays track: %q", got)
	}
	for c, want := range map[int]int{1: 0, 2: 0, 3: 1, 5: 2, 7: 3, 8: 3} {
		if lvl := densityLevel(c, 8, 4); lvl != want {
			t.Errorf("densityLevel(%d, 8) = %d, want %d", c, lvl, want)
		}
	}
	if densityLevel(0, 8, 4) != -1 || densityLevel(1, 1, 4) != 3 {
		t.Error("zero is no level; a lone span is solid")
	}
	// A shared peak keeps one glyph meaning one load across rows.
	if got := Timeline([]Span{{Start: hr(1), End: hr(2)}}, day, hr(24), 4, TimelineOpts{Look: asc, Peak: 4}); got != ".   " {
		t.Errorf("one of a peak of four = %q", got)
	}
}

func TestTimelineNowMarker(t *testing.T) {
	o := TimelineOpts{Look: asc, Now: hr(12)}
	if got := Timeline(nil, day, hr(24), 4, o); got != "  | " {
		t.Errorf("now = %q", got)
	}
	o.Now = hr(30)
	if got := Timeline(nil, day, hr(24), 4, o); got != "    " {
		t.Errorf("now outside the window = %q", got)
	}
	if got := Timeline(nil, day, day, 4, TimelineOpts{Look: asc, Now: day}); got != "    " {
		t.Errorf("an empty window = %q", got)
	}
}

func TestRulerGolden(t *testing.T) {
	o := TimelineOpts{Look: uni, Now: hr(13.5)}
	if got, want := Ruler(day, hr(24), 48, 3*time.Hour, o), "0     3     6     9     12 ╎  15    18    21    "; got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	// Labels that would touch are skipped; the now marker gives way to a
	// label in its cell.
	if got, want := Ruler(day, hr(24), 24, time.Hour, o), "0 2 4 6 8 10 13 16 19 22"; got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if got := Ruler(day, day.AddDate(0, 0, 7), 21, 24*time.Hour, TimelineOpts{Look: asc}); got != "08-13 08-15 08-17    " {
		t.Errorf("day ticks = %q", got)
	}
	if got := Ruler(day, hr(1), 12, 15*time.Minute, TimelineOpts{Look: asc}); got != "00:00 00:30 " {
		t.Errorf("minute ticks = %q", got)
	}
	if got := Ruler(day, hr(24), 8, 0, TimelineOpts{Look: asc}); got != "        " {
		t.Errorf("no step = %q", got)
	}
	if got := Ruler(day, hr(24), 8, time.Nanosecond, TimelineOpts{Look: asc}); got != "        " {
		t.Errorf("a step too fine to label = %q", got)
	}
}

// Ticks follow the wall clock of from's location: day ticks at local
// midnight, hour ticks on the local hour, even for a half-hour zone.
func TestRulerFollowsTheLocalClock(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	from := time.Date(2026, 8, 13, 0, 0, 0, 0, kst)
	if got := Ruler(from, from.AddDate(0, 0, 2), 12, 24*time.Hour, TimelineOpts{Look: asc}); got != "08-13 08-14 " {
		t.Errorf("KST day ticks = %q", got)
	}
	ist := time.FixedZone("IST", 5*3600+1800)
	from = time.Date(2026, 8, 13, 9, 40, 0, 0, ist)
	if got := Ruler(from, from.Add(3*time.Hour), 18, time.Hour, TimelineOpts{Look: asc}); got != "  10    11    12  " {
		t.Errorf("IST hour ticks = %q", got)
	}
}

// Labels never overlap, never run past the edge, and are never cut, at
// any width and step.
func TestRulerLabelsNeverOverlap(t *testing.T) {
	steps := []time.Duration{time.Minute * 10, time.Hour, 2 * time.Hour, 24 * time.Hour}
	for _, step := range steps {
		for w := 1; w <= 120; w++ {
			got := Ruler(day, day.AddDate(0, 0, 3), w, step, TimelineOpts{Look: asc})
			if render.Width(got) != w {
				t.Fatalf("step %v width %d: %q is %d cells", step, w, got, render.Width(got))
			}
			for _, f := range strings.Fields(got) {
				if !validTick(f, step) {
					t.Errorf("step %v width %d: token %q is not one whole label: %q", step, w, f, got)
				}
			}
		}
	}
}

func validTick(f string, step time.Duration) bool {
	switch {
	case step%(24*time.Hour) == 0:
		_, err := time.Parse("01-02", f)
		return err == nil
	case step%time.Hour == 0:
		h, err := strconv.Atoi(f)
		return err == nil && h >= 0 && h < 24
	}
	_, err := time.Parse("15:04", f)
	return err == nil
}

func TestLanesAlignHangulLabels(t *testing.T) {
	spans := []Span{
		{Start: hr(1), End: hr(3), Label: "배포"},
		{Start: hr(2), End: hr(5), Label: "db"},
		{Start: hr(4), End: hr(6), Label: "배포"},
		{Start: hr(9), End: hr(10), Label: "점검"},
	}
	rows := Lanes(spans, day, hr(12), 18, TimelineOpts{Look: asc, Ruler: 3 * time.Hour})
	if len(rows) != 4 {
		t.Fatalf("rows = %q", rows)
	}
	// Every timeline starts in the same cell whatever script the label is in.
	for _, r := range rows {
		if render.Width(r) != 18 {
			t.Errorf("row %q is %d cells", r, render.Width(r))
		}
		if prefix := ansi.Cut(r, 0, 5); render.Width(prefix) != 5 || !strings.HasSuffix(prefix, " ") {
			t.Errorf("label column of %q is not 4 cells + a space", r)
		}
	}
	// 1-3h and 4-6h on 12h/13 cells: adjacent cells, one span each.
	if got := ansi.Cut(rows[0], 5, 18); got != " ######      " {
		t.Errorf("lane 배포 = %q", got)
	}
	if got := ansi.Cut(rows[3], 5, 18); got != "0  3  6  9   " {
		t.Errorf("ruler under the lanes = %q", got)
	}
}

func TestLanesTruncateAndNarrow(t *testing.T) {
	spans := []Span{{Start: hr(1), End: hr(2), Label: "데이터베이스"}, {Start: hr(3), End: hr(4), Label: "api"}}
	// The ASCII set cuts with ~.
	rows := Lanes(spans, day, hr(24), 12, TimelineOpts{Look: asc, LabelMax: 3})
	if got := ansi.Strip(rows[0]); !strings.HasPrefix(got, "데~ ") || render.Width(got) != 12 {
		t.Errorf("cut label = %q", got)
	}
	if got := ansi.Strip(rows[1]); !strings.HasPrefix(got, "api ") {
		t.Errorf("short label = %q", got)
	}
	// Two cells leave no room for a label column: timelines alone.
	for _, r := range Lanes(spans, day, hr(24), 2, TimelineOpts{Look: asc}) {
		if strings.ContainsAny(r, "데a") || render.Width(r) != 2 {
			t.Errorf("width 2 lane = %q", r)
		}
	}
	// Unlabelled spans share one lane with an empty label.
	if rows := Lanes([]Span{{Start: hr(1), End: hr(2)}, {Start: hr(5), End: hr(6)}}, day, hr(24), 10, TimelineOpts{Look: asc}); len(rows) != 1 || rows[0] != "# #       " {
		t.Errorf("unlabelled = %q", rows)
	}
}
