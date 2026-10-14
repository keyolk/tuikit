package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestZeroThemeIsPlain(t *testing.T) {
	var th Theme
	if th.KeyNames().Glyphs || th.KeyNames().Name("enter") != "enter" {
		t.Fatal("the zero Theme must name keys in words")
	}
	if !th.ToastStyles().ASCII || th.FooterStyle().Names.Glyphs || th.HelpStyle().Names.Glyphs {
		t.Fatal("the zero Theme's adapters must be plain")
	}
	// A built Unicode theme still gets glyphs.
	if !NewWith(colorRenderer(), DefaultRoles(), Unicode()).KeyNames().Glyphs {
		t.Fatal("a Unicode theme names keys with glyphs")
	}
}

func TestEllipsisFlowsToAdapters(t *testing.T) {
	g := Unicode()
	g.Ellipsis = "~"
	th := NewWith(colorRenderer(), DefaultRoles(), g)
	if th.FooterStyle().Ellipsis != "~" || th.HelpStyle().Ellipsis != "~" || th.ToastStyles().Ellipsis != "~" {
		t.Fatal("Glyphs.Ellipsis must reach footer, help and toast styles")
	}
}

func TestDetectGlyphsCJKEllipsis(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("RUNEWIDTH_EASTASIAN", "")
	t.Setenv("LANG", "ko_KR.UTF-8")
	g := DetectGlyphs("")
	if g.ASCII || g.Ellipsis != "~" || g.Cursor != Unicode().Cursor {
		t.Fatalf("Korean UTF-8 locale: %+v", g)
	}
	t.Setenv("LANG", "en_US.UTF-8")
	if DetectGlyphs("").Ellipsis != "…" {
		t.Fatal("non-CJK keeps …")
	}
}

func TestBarAndSpinnerFrame(t *testing.T) {
	u, a := Unicode(), ASCII()
	for _, c := range []struct {
		frac float64
		w    int
		want string
	}{{0, 4, "----"}, {0.5, 4, "##--"}, {0.99, 4, "###-"}, {1, 4, "####"}, {2, 3, "###"}, {-1, 2, "--"}, {0.5, 0, ""}} {
		if got := a.Bar(c.frac, c.w); got != c.want {
			t.Errorf("Bar(%v,%d) = %q, want %q", c.frac, c.w, got, c.want)
		}
	}
	if got := u.Bar(0.5, 6); got != "▰▰▰▱▱▱" || lipgloss.Width(got) != 6 {
		t.Fatalf("unicode bar = %q", got)
	}
	if (Glyphs{}).Bar(0.5, 2) != "#-" {
		t.Fatal("a set without bar glyphs falls back to ASCII")
	}
	if u.SpinnerFrame(0) != u.Spinner[0] || u.SpinnerFrame(len(u.Spinner)+1) != u.Spinner[1] || u.SpinnerFrame(-1) != u.Spinner[len(u.Spinner)-1] {
		t.Fatal("SpinnerFrame must wrap both ways")
	}
	if (Glyphs{}).SpinnerFrame(3) != "" {
		t.Fatal("no spinner, no frame")
	}
	if strings.ContainsAny(a.Bar(0.3, 10), "▰▱") {
		t.Fatal("ASCII bar must be ASCII")
	}
}
