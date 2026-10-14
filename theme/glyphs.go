package theme

import (
	"os"
	"strings"
	"unicode"

	"github.com/keyolk/tuikit/render"
)

// Glyphs is every symbol the kit's users draw with. It is a flat struct, not
// a map, so a missing glyph is a compile error rather than an empty string
// that shifts a column (argx glyphs.go). Every glyph is one cell wide.
type Glyphs struct {
	ASCII bool // this is the ASCII set

	Cursor, Marked, Unmarked string
	Check, Cross, Warn, Info string
	Bullet, Ellipsis, Sep    string
	Up, Down                 string
	Branch, Corner, Pipe     string // tree connectors, three cells each
	// Spinner frames, one cell each; draw Spinner[tick%len(Spinner)] (see
	// SpinnerFrame).
	Spinner []string
	// BarFull and BarEmpty draw a progress bar (see Bar).
	BarFull, BarEmpty string
}

// SpinnerFrame is the spinner frame for tick i (any int, negative too), or
// "" when the set has no spinner.
func (g Glyphs) SpinnerFrame(i int) string {
	n := len(g.Spinner)
	if n == 0 {
		return ""
	}
	return g.Spinner[((i%n)+n)%n]
}

// Bar draws frac (clamped to 0..1) of a w-cell progress bar, rounding down
// so a bar is only full when the work is done. Empty when w <= 0.
func (g Glyphs) Bar(frac float64, w int) string {
	if w <= 0 {
		return ""
	}
	full, empty := g.BarFull, g.BarEmpty
	if full == "" {
		full, empty = "#", "-"
	}
	frac = max(0, min(1, frac))
	n := int(frac * float64(w))
	return strings.Repeat(full, n) + strings.Repeat(empty, w-n)
}

// Unicode is the default set: legible in any UTF-8 terminal without a
// patched font. The fold mark must not look like the cursor, or a folded
// line reads as selected (dayx).
func Unicode() Glyphs {
	return Glyphs{
		Cursor: "▸", Marked: "◉", Unmarked: "○",
		Check: "✓", Cross: "✗", Warn: "!", Info: "i",
		Bullet: "·", Ellipsis: "…", Sep: " · ",
		Up: "↑", Down: "↓",
		Branch: "├─ ", Corner: "└─ ", Pipe: "│  ",
		Spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		// ▰▱, not █░: █ is East Asian Ambiguous and doubles on CJK terminals.
		BarFull: "▰", BarEmpty: "▱",
	}
}

// ASCII avoids everything outside 7-bit ASCII, for SSH into minimal images
// and consoles that mangle box drawing. Each glyph keeps the width of its
// Unicode counterpart, so switching sets never moves a column.
func ASCII() Glyphs {
	return Glyphs{
		ASCII:  true,
		Cursor: ">", Marked: "*", Unmarked: " ",
		Check: "v", Cross: "x", Warn: "!", Info: "i",
		Bullet: "-", Ellipsis: "~", Sep: " | ",
		Up: "^", Down: "v",
		Branch: "|- ", Corner: "`- ", Pipe: "|  ",
		Spinner: []string{"|", "/", "-", "\\"},
		BarFull: "#", BarEmpty: "-",
	}
}

// DetectGlyphs picks a set: `<APP>_ASCII` (app upper-cased, non-alphanumerics
// as `_`) forces ASCII when set to anything but "", "0" or "false"; otherwise
// the locale decides. app may be empty, which skips the override.
//
// Under a CJK locale (render.CJKLocale) the Unicode set's Ellipsis is `~`:
// `…` is East Asian Ambiguous and draws two cells there, so every line cut
// to fit would overflow by one (gcl). The other glyphs are kept; ↑ ↓ · ○
// are ambiguous too, but they are not used to fit a line to its width.
func DetectGlyphs(app string) Glyphs {
	if app != "" && truthy(os.Getenv(EnvVar(app))) {
		return ASCII()
	}
	if utf8Locale() {
		g := Unicode()
		if render.CJKLocale() {
			g.Ellipsis = "~"
		}
		return g
	}
	return ASCII()
}

// EnvVar is the override variable for app, e.g. "my-app" → "MY_APP_ASCII".
func EnvVar(app string) string {
	var b strings.Builder
	for _, r := range app {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteByte('_')
		}
	}
	return b.String() + "_ASCII"
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no":
		return false
	}
	return true
}

// utf8Locale follows POSIX precedence: the first of LC_ALL, LC_CTYPE, LANG
// that is set decides. When none is set it answers true, as dayx does: macOS
// Terminal and iTerm often leave all three unset while rendering UTF-8 fine,
// and okx's "unset means ASCII" made those users look at ">" and "[x]".
func utf8Locale() bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			v = strings.ToLower(v)
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	return true
}
