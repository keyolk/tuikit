package render

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestEllipsisFollowsLocale(t *testing.T) {
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"LANG": "en_US.UTF-8"}, "…"},
		{map[string]string{"LANG": "ko_KR.UTF-8"}, "~"},
		{map[string]string{"LANG": "en_US.UTF-8", "LC_CTYPE": "ja_JP.UTF-8"}, "~"},
		{map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "C"}, "…"}, // LC_ALL wins
		{map[string]string{"LANG": "en_US.UTF-8", "RUNEWIDTH_EASTASIAN": "1"}, "~"},
		{map[string]string{"LANG": "ko_KR.UTF-8", "RUNEWIDTH_EASTASIAN": "0"}, "…"},
	} {
		for k, v := range c.env {
			t.Setenv(k, v)
		}
		if got := Ellipsis(); got != c.want {
			t.Errorf("%v: Ellipsis() = %q, want %q", c.env, got, c.want)
		}
		if got := Truncate("hello", 3); got != "he"+c.want {
			t.Errorf("%v: Truncate = %q", c.env, got)
		}
		for _, k := range []string{"LANG", "LC_CTYPE", "LC_ALL", "RUNEWIDTH_EASTASIAN"} {
			t.Setenv(k, "")
		}
		t.Setenv("LANG", "en_US.UTF-8")
	}
	if Or("~") != "~" || Or("") != "…" {
		t.Error("Or must keep a set marker and default an empty one")
	}
}

func TestTruncateMarker(t *testing.T) {
	cases := []struct {
		in, marker string
		w          int
		want       string
	}{
		{"hello", "~", 4, "hel~"},
		{"hello", "...", 4, "h..."},
		{"hello", "", 4, "hell"},
		{"hello", "...", 2, "he"}, // marker wider than w is dropped, not ""
		{"hello", "~", 5, "hello"},
		{"배포상태", "~", 5, "배포~"},
		{"\x1b[31mredred\x1b[0m", "~", 4, "red~"},
	}
	for _, c := range cases {
		got := TruncateMarker(c.in, c.w, c.marker)
		if ansi.Strip(got) != c.want || Width(got) > c.w {
			t.Errorf("TruncateMarker(%q, %d, %q) = %q, want %q", c.in, c.w, c.marker, got, c.want)
		}
	}
}

// The ASCII fast path must agree with the grapheme path cell for cell.
func TestASCIIFastPathMatchesSlowPath(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for range 2000 {
		b := make([]byte, r.Intn(30))
		for i := range b {
			b[i] = byte(0x20 + r.Intn(0x7f-0x20))
		}
		s := string(b)
		if Width(s) != ansi.StringWidth(s) {
			t.Fatalf("Width(%q) = %d, ansi says %d", s, Width(s), ansi.StringWidth(s))
		}
		w := r.Intn(32)
		for _, m := range []string{"…", "~", "..."} {
			fast := TruncateMarker(s, w, m)
			slow := s
			if ansi.StringWidth(s) > w && w > 0 {
				mm := m
				if ansi.StringWidth(m) > w {
					mm = ""
				}
				slow = ansi.Truncate(s, w, mm)
			} else if w <= 0 {
				slow = ""
			}
			if fast != slow {
				t.Fatalf("TruncateMarker(%q, %d, %q) = %q, slow path %q", s, w, m, fast, slow)
			}
		}
	}
	// Controls and escapes take the slow path.
	if Width("a\x1b[1mb") != 2 || Width("a\tb") != ansi.StringWidth("a\tb") {
		t.Fatal("non-printable input must not use the byte count")
	}
}

func TestPadRightLeftNeverTruncate(t *testing.T) {
	if got := PadRight("한글", 6); got != "한글  " {
		t.Errorf("PadRight = %q", got)
	}
	if got := PadLeft("42", 5); got != "   42" {
		t.Errorf("PadLeft = %q", got)
	}
	if PadRight("longer", 3) != "longer" || PadLeft("longer", 3) != "longer" {
		t.Error("PadRight/PadLeft must not truncate")
	}
	if Pad("longer", 3) != "lo…" {
		t.Errorf("Pad still truncates: %q", Pad("longer", 3))
	}
}

func TestMoreLine(t *testing.T) {
	for _, c := range []struct {
		a, b int
		want string
	}{{0, 0, ""}, {3, 0, "↑ 3 more"}, {0, 12, "↓ 12 more"}, {3, 12, "↑ 3 ↓ 12 more"}} {
		if got := MoreLine(c.a, c.b); got != c.want {
			t.Errorf("MoreLine(%d,%d) = %q", c.a, c.b, got)
		}
	}
}

func TestBoxIsExactlyWByH(t *testing.T) {
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	cw, ch := BoxContent(20, 6, st)
	if cw != 16 || ch != 4 {
		t.Fatalf("BoxContent = %d×%d, want 16×4", cw, ch)
	}
	for _, body := range []string{"", "short", strings.Repeat("long line ", 10) + "\n" + strings.Repeat("x\n", 20), "한글 본문\n둘째 줄"} {
		got := Box("Help", body, 20, 6, st)
		lines := strings.Split(got, "\n")
		if len(lines) != 6 {
			t.Fatalf("body %q: %d lines", body, len(lines))
		}
		for i, l := range lines {
			if Width(l) != 20 {
				t.Fatalf("body %q: line %d is %d cells: %q", body, i, Width(l), l)
			}
		}
		if !strings.HasPrefix(ansi.Strip(lines[0]), "╭─ Help ─") {
			t.Fatalf("title not in border: %q", ansi.Strip(lines[0]))
		}
	}
	// A title wider than the border is cut, never pushing the corner out.
	top := ansi.Strip(strings.Split(Box(strings.Repeat("T", 50), "", 12, 3, st), "\n")[0])
	if Width(top) != 12 || !strings.HasSuffix(top, "─╮") {
		t.Fatalf("long title = %q", top)
	}
	// Smaller than the frame: still w×h.
	if got := Box("t", "b", 3, 2, st.Padding(0, 2)); got != "   \n   " {
		t.Fatalf("tiny box = %q", got)
	}
	// The caller's Width is ignored; the outer size is Box's.
	if got := Box("", "x", 10, 3, st.Width(40)); Width(strings.Split(got, "\n")[0]) != 10 {
		t.Fatalf("style Width leaked: %q", got)
	}
	// Margins are part of the frame.
	m := st.Margin(1, 2)
	cw, ch = BoxContent(20, 8, m)
	if cw != 12 || ch != 4 {
		t.Fatalf("BoxContent with margin = %d×%d", cw, ch)
	}
	if got := strings.Split(Box("T", "", 20, 8, m), "\n"); len(got) != 8 || Width(got[0]) != 20 || !strings.Contains(ansi.Strip(got[1]), "─ T ─") {
		t.Fatalf("margined box = %q", got)
	}
}
