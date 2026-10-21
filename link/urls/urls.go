// Package urls is the part of link that needs nothing but the standard
// library: which URLs a TUI may link or open (Valid), where they sit in text
// (Find, FindIndex), which of them carry a credential (Sensitive), and the
// percent-encoding OSC 8 needs (Encode).
//
// It is a separate package so a config loader or a CLI can check a URL
// without importing Bubble Tea and the keys package along with link (okx's
// config package did, for Valid alone). link.Valid, link.Find and friends
// are thin wrappers over this package and behave the same.
package urls

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// MaxLen is the longest URL that is linked. Terminals cap OSC 8 URIs (VTE
// at 2083 bytes) and drop longer ones, sometimes with the text.
const MaxLen = 2048

// Valid reports whether u is an absolute http or https URL with a host and
// nothing a terminal escape could be smuggled through (control characters,
// spaces).
//
// Valid says nothing about what the URL carries: a presigned URL or an OAuth
// callback is Valid. Callers that show captured traffic decide with
// Sensitive whether to act on it.
func Valid(u string) bool {
	if u == "" || len(u) > MaxLen {
		return false
	}
	for i := 0; i < len(u); i++ {
		if c := u[i]; c <= ' ' || c == 0x7f {
			return false
		}
	}
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return false
	}
	return p.Scheme == "http" || p.Scheme == "https"
}

// Encode percent-encodes every non-ASCII byte of u, as browsers do. Width
// counters measure non-ASCII bytes inside an OSC 8 escape as text, so a URL
// with Hangul in it would widen the row it links; the encoded URL is the
// same URL to a browser.
func Encode(u string) string {
	for i := 0; i < len(u); i++ {
		if u[i] >= 0x80 {
			var b strings.Builder
			for j := 0; j < len(u); j++ {
				if c := u[j]; c >= 0x80 {
					fmt.Fprintf(&b, "%%%02X", c)
				} else {
					b.WriteByte(c)
				}
			}
			return b.String()
		}
	}
	return u
}

var pattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// FindIndex returns the byte ranges [start, end) of the http and https URLs
// in text, in order, by the rules of Find. text must be plain: an escape
// sequence next to a URL makes it fail Valid and drops it, so strip styling
// first (link.Find does). The ranges index text itself, which is what a
// caller needs to link URLs inside an already wrapped line: wrap the plain
// text first, then find the URLs in each line and link them (link.Wrap)
// after. Linking first and wrapping after splits an escape across lines.
func FindIndex(text string) [][2]int {
	var out [][2]int
	for _, loc := range pattern.FindAllStringIndex(text, -1) {
		u := trim(text[loc[0]:loc[1]])
		if Valid(u) {
			out = append(out, [2]int{loc[0], loc[0] + len(u)})
		}
	}
	return out
}

// Find returns the http and https URLs in plain text, in order. Trailing
// sentence punctuation is not part of a URL ("see https://x.io/a."), and
// neither is a closing bracket that has no opening one in the URL
// ("(https://x.io/a)"). link.Find strips escapes before calling it.
func Find(text string) []string {
	var out []string
	for _, r := range FindIndex(text) {
		out = append(out, text[r[0]:r[1]])
	}
	return out
}

func trim(u string) string {
	for u != "" {
		last := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?", last) >= 0:
			u = u[:len(u)-1]
		case last == ')' && strings.Count(u, "(") < strings.Count(u, ")"),
			last == ']' && strings.Count(u, "[") < strings.Count(u, "]"):
			u = u[:len(u)-1]
		default:
			return u
		}
	}
	return u
}
