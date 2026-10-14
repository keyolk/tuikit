package theme

import (
	"os"
	"strings"
	"unicode"
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
	Spinner                  []string
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
	}
}

// DetectGlyphs picks a set: `<APP>_ASCII` (app upper-cased, non-alphanumerics
// as `_`) forces ASCII when set to anything but "", "0" or "false"; otherwise
// the locale decides. app may be empty, which skips the override.
func DetectGlyphs(app string) Glyphs {
	if app != "" && truthy(os.Getenv(EnvVar(app))) {
		return ASCII()
	}
	if utf8Locale() {
		return Unicode()
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
