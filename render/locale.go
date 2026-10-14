package render

import (
	"os"
	"strings"
)

// Ellipsis is the default truncation marker: `…`, or `~` when the locale is
// CJK (see CJKLocale).
//
// U+2026 is East Asian Ambiguous: a CJK terminal draws it two cells wide
// while every width function here counts one, so a line cut to fit ends one
// cell past the edge and wraps (gcl). `~` is one cell everywhere. Styles
// whose Ellipsis field is empty use this; set the field to override it.
//
// It reads the environment on every call (only when something is actually
// cut), so tests can switch locales with t.Setenv.
func Ellipsis() string {
	if CJKLocale() {
		return "~"
	}
	return "…"
}

// Or returns marker, or the default Ellipsis when marker is empty. Styles use
// it to resolve their Ellipsis field.
func Or(marker string) string {
	if marker != "" {
		return marker
	}
	return Ellipsis()
}

// CJKLocale reports whether the terminal likely draws East Asian Ambiguous
// characters two cells wide: RUNEWIDTH_EASTASIAN (go-runewidth's switch) when
// set, else a ko/ja/zh locale in the first of LC_ALL, LC_CTYPE, LANG that is
// set.
func CJKLocale() bool {
	if v, ok := os.LookupEnv("RUNEWIDTH_EASTASIAN"); ok && v != "" {
		return v != "0" && !strings.EqualFold(v, "false")
	}
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			v = strings.ToLower(v)
			return strings.HasPrefix(v, "ko") || strings.HasPrefix(v, "ja") || strings.HasPrefix(v, "zh")
		}
	}
	return false
}
