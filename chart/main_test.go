package chart

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/keyolk/tuikit/theme"
)

// TestMain pins a non-CJK UTF-8 locale so the ellipsis is `…` and
// DetectGlyphs picks the Unicode set whatever the developer's shell says;
// tests that need CJK set it with t.Setenv.
func TestMain(m *testing.M) {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "RUNEWIDTH_EASTASIAN", "NO_COLOR"} {
		os.Unsetenv(k)
	}
	os.Setenv("LANG", "en_US.UTF-8")
	os.Exit(m.Run())
}

var (
	day = time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	hr  = func(h float64) time.Time { return day.Add(time.Duration(h * float64(time.Hour))) }
)

// colorTheme is a theme that always emits truecolor, as a terminal would.
func colorTheme(g theme.Glyphs) theme.Theme {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
	return theme.NewWith(r, theme.DefaultRoles(), g)
}
