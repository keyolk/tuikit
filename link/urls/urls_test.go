package urls

import (
	"slices"
	"testing"
)

func TestFindIndexMatchesFind(t *testing.T) {
	text := "see https://x.io/a. and (https://y.io/b) or https://z.io/한글?q=1, file:///etc"
	got := FindIndex(text)
	want := [][2]int{{4, 18}, {25, 39}, {44, 67}}
	if !slices.Equal(got, want) {
		t.Fatalf("FindIndex = %v, want %v", got, want)
	}
	var subs []string
	for _, r := range got {
		subs = append(subs, text[r[0]:r[1]])
	}
	if f := Find(text); !slices.Equal(subs, f) {
		t.Fatalf("FindIndex ranges %q != Find %q", subs, f)
	}
	if subs[2] != "https://z.io/한글?q=1" {
		t.Fatalf("third URL %q", subs[2])
	}
}

// A soft-wrapped URL: each line is searched on its own, and the ranges
// index that line, which is what linking after wrapping needs.
func TestFindIndexPerWrappedLine(t *testing.T) {
	lines := []string{"log: https://x.io/a/very/long/pa", "th/rest done"}
	if got := FindIndex(lines[0]); !slices.Equal(got, [][2]int{{5, 32}}) {
		t.Fatalf("line 0 = %v", got)
	}
	if got := FindIndex(lines[1]); got != nil {
		t.Fatalf("line 1 = %v, want none", got)
	}
}

func TestSensitive(t *testing.T) {
	for u, want := range map[string]bool{
		"https://github.com/keyolk/tuikit/pull/9":                            false,
		"https://x.io/search?q=token+bucket":                                 false, // the value, not the key
		"https://x.io/a?monkey=1":                                            false, // "key" is matched whole
		"https://x.io/a?page=2&sort=asc":                                     false,
		"https://user:pw@x.io/":                                              true,
		"https://user@x.io/":                                                 true,
		"https://x.io/cb?code=abc&state=1":                                   true,
		"https://b.s3.amazonaws.com/o?X-Amz-Signature=ff&X-Amz-Credential=a": true,
		"https://x.io/a?access_token=1":                                      true,
		"https://x.io/a#access_token=1&type=bearer":                          true,
		"https://x.io/a?client_secret=1":                                     true,
		"https://x.io/a?apiKey=1":                                            true,
		"https://x.io/a?api-key=1":                                           true,
		"https://x.io/a?SIG=1":                                               true,
		"https://x.io/a?bad%zz&session=1":                                    true, // a malformed pair hides nothing
		"https://x.io/a?%73ecret=1":                                          true, // percent-encoded key
		"https://x.io/a?jwt=1":                                               true,
		"https://x.io/a?password=1":                                          true,
		"::not a url":                                                        false,
	} {
		if got := Sensitive(u); got != want {
			t.Errorf("Sensitive(%q) = %v, want %v", u, got, want)
		}
	}
	// Valid stays about form, not content.
	if !Valid("https://x.io/cb?code=abc") {
		t.Fatalf("Valid rejected a sensitive URL; callers decide")
	}
}

func TestEncode(t *testing.T) {
	if got := Encode("https://x.io/한"); got != "https://x.io/%ED%95%9C" {
		t.Fatalf("Encode = %q", got)
	}
	if got := Encode("https://x.io/a"); got != "https://x.io/a" {
		t.Fatalf("Encode ASCII = %q", got)
	}
}
