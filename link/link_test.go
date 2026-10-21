package link

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/tuitest"
)

func TestValid(t *testing.T) {
	for u, want := range map[string]bool{
		"https://github.com/keyolk/tuikit/pull/9": true,
		"http://localhost:8080/x?a=1&b=2":         true,
		"https://x.io/path;with;semis":            true,
		"":                                        false,
		"github.com/keyolk":                       false, // no scheme
		"https://":                                false, // no host
		"file:///etc/passwd":                      false,
		"javascript:alert(1)":                     false,
		"ssh://host/x":                            false,
		"https://x.io/\x1b]8;;evil\x1b\\":         false, // an escape inside
		"https://x.io/a b":                        false,
		"https://x.io/" + strings.Repeat("a", maxURL): false,
	} {
		if got := Valid(u); got != want {
			t.Errorf("Valid(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestWrapIsAnOSC8LinkThatTakesNoCells(t *testing.T) {
	u := "https://github.com/keyolk/tuikit/pull/9"
	got := Wrap(u, "#9 link package")
	if want := "\x1b]8;;" + u + "\x1b\\#9 link package\x1b]8;;\x1b\\"; got != want {
		t.Fatalf("Wrap = %q, want %q", got, want)
	}
	if w := render.Width(got); w != render.Width("#9 link package") {
		t.Fatalf("width %d: the escape must not take cells", w)
	}
	// Cut to a column, the link still opens and still closes.
	cut := render.Truncate(got, 6)
	if !strings.HasPrefix(cut, "\x1b]8;;"+u) || !strings.HasSuffix(cut, "\x1b]8;;\x1b\\") {
		t.Fatalf("truncated link lost an end: %q", cut)
	}
	// A style around it keeps it.
	styled := lipgloss.NewRenderer(io.Discard).NewStyle().Bold(true).Render(got)
	if !strings.Contains(styled, "\x1b]8;;"+u+"\x1b\\") {
		t.Fatalf("styling dropped the link: %q", styled)
	}
}

func TestWrapLeavesBadURLsAsPlainText(t *testing.T) {
	if got := Wrap("javascript:alert(1)", "click"); got != "click" {
		t.Fatalf("got %q", got)
	}
	if got := Wrap("https://x.io/\x1b\\x", "t"); got != "t" {
		t.Fatalf("an escape in the URL must not reach the terminal: %q", got)
	}
	if got := Wrap("https://x.io/a", ""); got != "" {
		t.Fatalf("a label cut to nothing must stay empty, not become the URL: %q", got)
	}
}

func TestFind(t *testing.T) {
	text := "see https://x.io/a. and (https://en.wikipedia.org/wiki/Go_(game)) or " +
		"\x1b[1mhttps://y.io/b?q=1\x1b[0m, not ftp://z.io or file:///x; [https://z.io/c]"
	got := Find(text)
	want := []string{"https://x.io/a", "https://en.wikipedia.org/wiki/Go_(game)", "https://y.io/b?q=1", "https://z.io/c"}
	if !slices.Equal(got, want) {
		t.Fatalf("Find = %q\nwant  %q", got, want)
	}
}

func TestOpenRunsTheLauncherOffTheLoopAndReportsIt(t *testing.T) {
	var got []string
	old := Launcher
	Launcher = func(u string) error { got = append(got, u); return nil }
	t.Cleanup(func() { Launcher = old })

	cmd := Open("https://x.io/a")
	if len(got) != 0 {
		t.Fatal("Open launched before its command ran")
	}
	msgs := tuitest.Drain(cmd, time.Second)
	if len(msgs) != 1 || msgs[0] != (OpenedMsg{URL: "https://x.io/a"}) {
		t.Fatalf("got %#v", msgs)
	}
	if !slices.Equal(got, []string{"https://x.io/a"}) {
		t.Fatalf("launcher got %q", got)
	}
	if msgs[0].(OpenedMsg).Note() != "opened https://x.io/a" {
		t.Fatalf("note %q", msgs[0].(OpenedMsg).Note())
	}
}

func TestOpenRefusesAnythingButHTTP(t *testing.T) {
	old := Launcher
	Launcher = func(string) error { t.Fatal("launcher ran for a bad URL"); return nil }
	t.Cleanup(func() { Launcher = old })
	for _, u := range []string{"file:///etc/passwd", "-a Calculator", "x-man-page://ls", ""} {
		m := Open(u)().(OpenedMsg)
		if !errors.Is(m.Err, ErrInvalid) {
			t.Fatalf("Open(%q) err = %v", u, m.Err)
		}
	}
}

func TestLaunchOverSSHDoesNotOpenARemoteBrowser(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22")
	if err := launch("https://x.io/a"); !errors.Is(err, ErrRemote) {
		t.Fatalf("err = %v, want ErrRemote", err)
	}
}

func TestBindingResolvesO(t *testing.T) {
	km := keys.New(Binding("list"))
	b, ok := km.Resolve([]keys.Ctx{"list"}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}, keys.Opts{})
	if !ok || b.Action != Action {
		t.Fatalf("o resolved to %+v, %v", b, ok)
	}
	// Under a Korean layout the same key types ㅐ.
	b, ok = km.Resolve([]keys.Ctx{"list"}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ㅐ")}, keys.Opts{})
	if !ok || b.Action != Action {
		t.Fatalf("ㅐ resolved to %+v, %v", b, ok)
	}
}

func TestANonASCIIURLDoesNotWidenTheLabel(t *testing.T) {
	u := "https://x.io/한글-서비스?q=값"
	got := Wrap(u, "ab")
	for name, w := range map[string]int{
		"render":   render.Width(got),
		"lipgloss": lipgloss.Width(got),
		"ansi":     ansi.StringWidth(got),
		"pad":      render.Width(render.PadRight(got, 6)),
	} {
		want := 2
		if name == "pad" {
			want = 6
		}
		if w != want {
			t.Errorf("%s width = %d, want %d: %q", name, w, want, got)
		}
	}
	links := tuitest.Links(got)
	if len(links) != 1 || links[0].Text != "ab" {
		t.Fatalf("links %+v", links)
	}
	// Percent-encoded, it is the same URL to a browser.
	if want := "https://x.io/%ED%95%9C%EA%B8%80-%EC%84%9C%EB%B9%84%EC%8A%A4?q=%EA%B0%92"; links[0].URL != want {
		t.Fatalf("URL %q, want %q", links[0].URL, want)
	}
}

func TestSpanLinksOnlyTheText(t *testing.T) {
	got := Span("https://x.io/a", "● ", "title", "  2h")
	links := tuitest.Links(got)
	if len(links) != 1 || links[0].Text != "title" || ansi.Strip(got) != "● title  2h" {
		t.Fatalf("got %q, links %+v", got, links)
	}
}

func TestOpenFirstSaysHowManyAndSkipsBadURLs(t *testing.T) {
	var got []string
	old := Launcher
	Launcher = func(u string) error { got = append(got, u); return nil }
	t.Cleanup(func() { Launcher = old })

	m := OpenFirst([]string{"javascript:x", "https://x.io/a", "https://x.io/b"})().(OpenedMsg)
	if m.URL != "https://x.io/a" || m.Of != 2 || m.Err != nil {
		t.Fatalf("got %+v", m)
	}
	if m.Note() != "opened https://x.io/a (the first of 2 links)" {
		t.Fatalf("note %q", m.Note())
	}
	if !slices.Equal(got, []string{"https://x.io/a"}) {
		t.Fatalf("launched %q", got)
	}
	one := OpenFirst([]string{"https://x.io/c"})().(OpenedMsg)
	if one.Note() != "opened https://x.io/c" {
		t.Fatalf("one link note %q", one.Note())
	}
	none := OpenFirst([]string{"file:///etc/passwd"})().(OpenedMsg)
	if !errors.Is(none.Err, ErrNoLink) || none.Note() != "no link here" || none.Refused() {
		t.Fatalf("no valid link: %+v %q", none, none.Note())
	}
	if len(got) != 2 {
		t.Fatalf("launched %q", got)
	}
}

func TestRefusedTellsAFilteredURLFromAFailedOne(t *testing.T) {
	if !(OpenedMsg{Err: ErrInvalid}).Refused() || (OpenedMsg{Err: ErrRemote}).Refused() {
		t.Fatal("Refused must be true only for ErrInvalid")
	}
}

func TestNoteWithAnotherLanguage(t *testing.T) {
	ko := Notes{Opened: "열기: %s", OpenedFirst: "열기: %s (링크 %d개 중 첫 번째)", Failed: "열기 실패: %s", None: "링크 없음"}
	for m, want := range map[OpenedMsg]string{
		{URL: "https://x.io/a"}:        "열기: https://x.io/a",
		{URL: "https://x.io/a", Of: 3}: "열기: https://x.io/a (링크 3개 중 첫 번째)",
		{Err: ErrRemote}:               "열기 실패: no local browser over SSH",
		{Err: ErrNoLink}:               "링크 없음",
	} {
		if got := m.NoteWith(ko); got != want {
			t.Errorf("%+v: %q, want %q", m, got, want)
		}
	}
}
