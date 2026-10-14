package theme

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func colorRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
	return r
}

func TestColorThemeUsesRoleColors(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	th := NewWith(colorRenderer(), DefaultRoles(), Unicode())
	if th.NoColor() {
		t.Fatal("NO_COLOR is unset")
	}
	out := th.Warn.Render("w")
	if !strings.Contains(out, "38;2;245;158;11") { // #F59E0B, the dark Warn
		t.Fatalf("Warn rendered %q; want the dark-background warn color", out)
	}
	if sel := th.Selected.Render("row"); !strings.Contains(sel, "48;2;") {
		t.Fatalf("Selected = %q; want a background color", sel)
	}
	if th.Renderer() == nil {
		t.Fatal("Renderer() is nil")
	}
}

// dayx styles.go meant NO_COLOR to drop colors but keep bold/reverse, which
// carry the selection. Forcing termenv's Ascii profile (as dayx did) drops
// the attributes too; this pins the behaviour that was intended.
func TestNoColorKeepsBoldAndReverse(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CI", "")
	// A renderer that believes it writes to a terminal, as a real program does.
	r := lipgloss.NewRenderer(io.Discard, termenv.WithTTY(true))
	th := NewWith(r, DefaultRoles(), Unicode())
	if !th.NoColor() || r.ColorProfile() != termenv.ANSI {
		t.Fatalf("NO_COLOR profile = %v, want ANSI (attributes, no color)", r.ColorProfile())
	}
	for name, s := range map[string]lipgloss.Style{"Warn": th.Warn, "Error": th.Error, "Accent": th.Accent} {
		if out := s.Render("x"); strings.Contains(out, "38;") || strings.Contains(out, "48;") {
			t.Errorf("%s emitted a color under NO_COLOR: %q", name, out)
		}
	}
	sel := th.Selected.Render("row")
	if !strings.Contains(sel, "\x1b[1;7m") {
		t.Fatalf("Selected = %q; want bold+reverse under NO_COLOR", sel)
	}
	if ansi.Strip(sel) != "row" {
		t.Fatalf("Selected text changed: %q", ansi.Strip(sel))
	}
	// Not a terminal: plain text, nothing to keep.
	if NewRenderer(io.Discard).ColorProfile() != termenv.Ascii {
		t.Fatal("a non-terminal writer must stay plain under NO_COLOR")
	}
}

func TestNewDefaultsRenderer(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if New(nil).Renderer() != lipgloss.DefaultRenderer() {
		t.Fatal("nil renderer must fall back to the default renderer")
	}
}

func TestEveryRoleIsSet(t *testing.T) {
	v := reflect.ValueOf(DefaultRoles())
	for i := 0; i < v.NumField(); i++ {
		c := v.Field(i).Interface().(lipgloss.AdaptiveColor)
		if c.Light == "" || c.Dark == "" {
			t.Errorf("role %s has an empty variant", v.Type().Field(i).Name)
		}
	}
}

func TestAdaptersCarryStyles(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	th := NewWith(colorRenderer(), DefaultRoles(), ASCII())
	ts := th.ToastStyles()
	if !ts.ASCII || ts.Error.Render("e") != th.Error.Render("e") {
		t.Fatal("ToastStyles lost the theme")
	}
	if fs := th.FooterStyle(); fs.Key.Render("k") != th.Key.Render("k") {
		t.Fatal("FooterStyle lost the theme")
	}
	if box := th.Box.Render("x"); !strings.Contains(box, "+") || strings.ContainsAny(box, "╭─") {
		t.Fatalf("ASCII theme box = %q", box)
	}
}

func glyphStrings(g Glyphs) map[string]string {
	out := map[string]string{}
	v := reflect.ValueOf(g)
	for i := 0; i < v.NumField(); i++ {
		if s, ok := v.Field(i).Interface().(string); ok {
			out[v.Type().Field(i).Name] = s
		}
	}
	for i, s := range g.Spinner {
		out["Spinner"+string(rune('0'+i))] = s
	}
	return out
}

// From argx glyphs_test.go: a glyph that changes width between sets moves
// every column after it.
func TestGlyphSetsHaveMatchingWidths(t *testing.T) {
	u, a := glyphStrings(Unicode()), glyphStrings(ASCII())
	for name, us := range u {
		if strings.HasPrefix(name, "Spinner") {
			if lipgloss.Width(us) != 1 {
				t.Errorf("unicode %s is %d cells", name, lipgloss.Width(us))
			}
			continue
		}
		if lipgloss.Width(us) != lipgloss.Width(a[name]) {
			t.Errorf("%s: unicode %q is %d cells, ascii %q is %d", name, us, lipgloss.Width(us), a[name], lipgloss.Width(a[name]))
		}
	}
}

func TestASCIISetIsPureASCII(t *testing.T) {
	for name, s := range glyphStrings(ASCII()) {
		for _, r := range s {
			if r > 127 {
				t.Errorf("ascii %s contains %q", name, r)
			}
		}
	}
}

func TestDetectGlyphs(t *testing.T) {
	cases := []struct {
		name, lcAll, lang, override string
		wantASCII                   bool
	}{
		{"utf-8 locale", "", "en_US.UTF-8", "", false},
		{"lowercase utf8", "", "ko_KR.utf8", "", false},
		{"C locale", "", "C", "", true},
		{"LC_ALL wins over LANG", "C", "en_US.UTF-8", "", true},
		{"nothing set means macOS default UTF-8", "", "", "", false},
		{"override", "", "en_US.UTF-8", "1", true},
		{"override off", "", "C", "0", true},
		{"override false keeps unicode", "", "en_US.UTF-8", "false", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LC_ALL", c.lcAll)
			t.Setenv("LC_CTYPE", "")
			t.Setenv("LANG", c.lang)
			t.Setenv("MY_APP_ASCII", c.override)
			if got := DetectGlyphs("my-app").ASCII; got != c.wantASCII {
				t.Errorf("ASCII = %v, want %v", got, c.wantASCII)
			}
		})
	}
}

func TestEnvVar(t *testing.T) {
	for in, want := range map[string]string{"okx": "OKX_ASCII", "my-app": "MY_APP_ASCII", "a.b2": "A_B2_ASCII"} {
		if got := EnvVar(in); got != want {
			t.Errorf("EnvVar(%q) = %q, want %q", in, got, want)
		}
	}
}
