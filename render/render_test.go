package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		w    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"배포상태", 8, "배포상태"},
		{"배포상태", 5, "배포…"}, // a wide char never straddles the edge
		{"ab😀de", 4, "ab…"},
	}
	for _, c := range cases {
		if got := Truncate(c.in, c.w); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.in, c.w, got, c.want)
		}
		if got := Truncate(c.in, c.w); Width(got) > max(c.w, 0) {
			t.Errorf("Truncate(%q, %d) is %d cells", c.in, c.w, Width(got))
		}
	}
}

// From argx render_util: rune-slicing a styled string cut an escape sequence
// and dropped the reset, bleeding color into every row below.
func TestTruncateKeepsEscapesWhole(t *testing.T) {
	in := "\x1b[31mredredred\x1b[0m"
	got := Truncate(in, 5)
	if Width(got) != 5 || ansi.Strip(got) != "redr…" {
		t.Fatalf("Truncate styled = %q", got)
	}
	if !strings.HasPrefix(got, "\x1b[31m") {
		t.Fatalf("style lost: %q", got)
	}
}

func TestPad(t *testing.T) {
	for _, c := range []struct {
		in string
		w  int
	}{{"ab", 5}, {"한글", 6}, {"한글", 3}, {"😀x", 4}, {"\x1b[1mbold\x1b[0m", 6}, {"long text", 4}} {
		if got := Pad(c.in, c.w); Width(got) != c.w {
			t.Errorf("Pad(%q, %d) = %q is %d cells", c.in, c.w, got, Width(got))
		}
	}
	if Pad("x", 0) != "" {
		t.Error("Pad to 0 must be empty")
	}
}

func TestFitLines(t *testing.T) {
	got := FitLines("a\n한국어 텍스트입니다\nc\nd", 6, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 || lines[0] != "a" || lines[2] != "c" {
		t.Fatalf("FitLines = %q", lines)
	}
	if Width(lines[1]) > 6 {
		t.Fatalf("line 1 is %d cells", Width(lines[1]))
	}
	if got := strings.Split(FitLines("x", 5, 4), "\n"); len(got) != 4 {
		t.Fatalf("short input not padded to 4 lines: %q", got)
	}
	if FitLines("x", 5, 0) != "" {
		t.Fatal("zero height must be empty")
	}
}

func TestDim(t *testing.T) {
	got := Dim("\x1b[31mred\x1b[0m\n\nplain")
	lines := strings.Split(got, "\n")
	if lines[0] != "\x1b[2mred\x1b[0m" || lines[1] != "" || lines[2] != "\x1b[2mplain\x1b[0m" {
		t.Fatalf("Dim = %q", lines)
	}
}

func TestOverlayKeepsBaseAround(t *testing.T) {
	base := "aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc"
	got := ansi.Strip(Overlay(base, "XX\nYY", 3, 1))
	want := "aaaaaaaaaa\nbbbXXbbbbb\ncccYYccccc"
	if got != want {
		t.Fatalf("Overlay =\n%s\nwant\n%s", got, want)
	}
}

// Ported from argx TestSpliceOverlayHandlesWideCharacterAtTheEdge: a wide
// character under the box's edge must not push the line past the frame.
func TestOverlayWideCharacterAtEitherSeam(t *testing.T) {
	// "한" occupies cells 2-3.
	for _, c := range []struct {
		name string
		x    int
		box  string
		want string
	}{
		{"left seam inside the wide char", 3, "XXXX", "ab XXXXghi"},
		{"right seam inside the wide char", 0, "XXX", "XXX defghi"},
		{"aligned", 2, "XX", "abXXdefghi"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := Overlay("ab한defghi", c.box, c.x, 0)
			if w := Width(got); w != 10 {
				t.Errorf("spliced line is %d cells, want 10: %q", w, ansi.Strip(got))
			}
			if ansi.Strip(got) != c.want {
				t.Errorf("got %q, want %q", ansi.Strip(got), c.want)
			}
		})
	}
}

func TestOverlayNeverGrowsLines(t *testing.T) {
	bases := []string{
		"ab한defghi\n한한한한한\n😀😀😀😀😀\nshort",
		"\x1b[31m한글\x1b[0m텍스트입니다",
	}
	boxes := []string{"XXXX", "한글", "a\nbb\nccc", "\x1b[7mreverse\x1b[0m", strings.Repeat("W", 30)}
	for _, base := range bases {
		frameW := 0
		for _, l := range strings.Split(base, "\n") {
			frameW = max(frameW, Width(l))
		}
		for _, box := range boxes {
			for x := 0; x <= frameW; x++ {
				for y := -1; y <= 3; y++ {
					got := Overlay(base, box, x, y)
					lines := strings.Split(got, "\n")
					if len(lines) != strings.Count(base, "\n")+1 {
						t.Fatalf("line count changed for x=%d y=%d", x, y)
					}
					for i, l := range lines {
						if Width(l) > frameW {
							t.Fatalf("base %q box %q x=%d y=%d: line %d is %d cells > %d: %q",
								base, box, x, y, i, Width(l), frameW, ansi.Strip(l))
						}
					}
				}
			}
		}
	}
}

func TestOverlayResetsAtSeams(t *testing.T) {
	got := Overlay("\x1b[31mredredred\x1b[0m", "X", 3, 0)
	if !strings.Contains(got, reset+"X"+reset) {
		t.Fatalf("no reset around the box: %q", got)
	}
	// The tail keeps the base's color.
	if !strings.Contains(got[strings.LastIndex(got, "X"):], "\x1b[31m") {
		t.Fatalf("tail lost its style: %q", got)
	}
}

func TestCenter(t *testing.T) {
	got := Center("list row one\nlist row two", "[box]", 20, 5)
	lines := strings.Split(ansi.Strip(got), "\n")
	if len(lines) != 5 {
		t.Fatalf("Center height = %d", len(lines))
	}
	for _, l := range lines {
		if Width(l) != 20 {
			t.Fatalf("Center line %q is %d cells", l, Width(l))
		}
	}
	if !strings.Contains(lines[2], "[box]") || !strings.HasPrefix(lines[0], "list row one") {
		t.Fatalf("Center = %q", lines)
	}
	// A box larger than the frame is clipped, not grown.
	big := Center("", strings.Repeat("Z", 50)+"\n"+strings.Repeat("Z", 50), 10, 1)
	if Width(big) != 10 || strings.Count(big, "\n") != 0 {
		t.Fatalf("oversized box: %q", big)
	}
}

func TestSafeRecoversAndLogs(t *testing.T) {
	var logged string
	logf := func(f string, a ...any) { logged = fmt.Sprintf(f, a...) }
	got := Safe("detail", 30, 3, logf, func() string { panic("boom") })
	lines := strings.Split(got, "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "detail failed to draw") {
		t.Fatalf("fallback = %q", lines)
	}
	if Width(lines[0]) > 30 {
		t.Fatalf("fallback is %d cells", Width(lines[0]))
	}
	if !strings.Contains(logged, "boom") || !strings.Contains(logged, "goroutine") {
		t.Fatalf("log missing panic or stack: %q", logged)
	}
	if got := Safe("ok", 10, 1, logf, func() string { return "fine" }); got != "fine" {
		t.Fatalf("Safe passthrough = %q", got)
	}
	if narrow := Safe("detail", 8, 0, logf, func() string { panic(1) }); Width(narrow) > 8 || narrow == "" {
		t.Fatalf("narrow fallback = %q", narrow)
	}
}

func TestWindowKeepsCursorVisible(t *testing.T) {
	cases := []struct {
		total, height, cursor, offset int
		start, end                    int
	}{
		{100, 10, 0, 0, 0, 10},
		{100, 10, 9, 0, 0, 10},    // last visible row: no scroll
		{100, 10, 10, 0, 1, 11},   // one past: minimal scroll
		{100, 10, 50, 45, 45, 55}, // inside: unchanged
		{100, 10, 40, 45, 40, 50}, // above: scroll up to it
		{100, 10, 99, 0, 90, 100},
		{5, 10, 3, 2, 0, 5},        // fits: offset clamps to 0
		{100, 10, 500, 0, 90, 100}, // cursor clamped
		{100, 10, -3, 7, 0, 10},
		{0, 10, 0, 0, 0, 0},
		{10, 0, 0, 0, 0, 0},
	}
	for _, c := range cases {
		s, e, o := Window(c.total, c.height, c.cursor, c.offset)
		if s != c.start || e != c.end || o != s {
			t.Errorf("Window(%d,%d,%d,%d) = %d,%d,%d; want %d,%d", c.total, c.height, c.cursor, c.offset, s, e, o, c.start, c.end)
		}
	}
}

func TestMore(t *testing.T) {
	if top, bot := More(0, 0, 10); top != "" || bot != "" {
		t.Fatalf("nothing hidden: %q %q", top, bot)
	}
	if top, bot := More(3, 12, 10); top != "↑ 3 more" || bot != "↓ 12 more" {
		t.Fatalf("More(3,12) = %q %q", top, bot)
	}
	if top, bot := More(3, 12, 2); top != "" || bot != "↑ 3 ↓ 12 more" {
		t.Fatalf("collapsed = %q %q", top, bot)
	}
	if top, bot := More(3, 0, 2); top != "" || bot != "↑ 3 more" {
		t.Fatalf("collapsed above only = %q %q", top, bot)
	}
	if top, bot := More(0, 4, 1); top != "" || bot != "↓ 4 more" {
		t.Fatalf("collapsed below only = %q %q", top, bot)
	}
}
