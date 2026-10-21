package link

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/tuitest"
)

func TestCellLinksOnlyTheTextAndIsExactlyW(t *testing.T) {
	for _, tc := range []struct {
		text, want string
		w          int
	}{
		{"short", "short", 5},
		{"pull request title", "pull req~", 9},
		{"한글 제목입니다", "한글 ~", 6},
	} {
		c := Cell("https://x.io/한", tc.text, tc.w, "~")
		if w := render.Width(c); w != tc.w {
			t.Errorf("Cell(%q, %d) is %d cells", tc.text, tc.w, w)
		}
		ls := tuitest.Links(c)
		if len(ls) != 1 || ls[0].Text != tc.want || ls[0].URL != "https://x.io/%ED%95%9C" {
			t.Errorf("Cell(%q, %d) links %+v, want text %q", tc.text, tc.w, ls, tc.want)
		}
	}
	if c := Cell("https://x.io", "abc", 6, ""); tuitest.Links(c)[0].Text != "abc" || render.Width(c) != 6 {
		t.Errorf("the padding is inside the link or the width is off: %q", c)
	}
	if c := Cell("file:///etc/passwd", "abc", 4, "~"); c != "abc " {
		t.Errorf("an invalid URL: %q, want plain padded text", c)
	}
	if Cell("https://x.io", "abc", 0, "~") != "" {
		t.Errorf("width 0 drew something")
	}
}

func TestFindIndexAndFindAgree(t *testing.T) {
	text := "a https://x.io/b, c"
	r := FindIndex(text)
	if len(r) != 1 || text[r[0][0]:r[0][1]] != "https://x.io/b" {
		t.Fatalf("FindIndex = %v", r)
	}
	// Find still strips styling first, as before the move to urls.
	if got := Find("\x1b[1mhttps://x.io/b\x1b[0m."); len(got) != 1 || got[0] != "https://x.io/b" {
		t.Fatalf("Find on styled text = %q", got)
	}
}

func TestOpenWithUsesItsLauncherNotTheGlobal(t *testing.T) {
	old := Launcher
	Launcher = func(string) error { t.Errorf("the global Launcher ran"); return nil }
	t.Cleanup(func() { Launcher = old })
	var got []string
	mine := func(u string) error { got = append(got, u); return nil }
	msgs := tuitest.Drain(OpenWith(mine, "https://x.io/a"), 0)
	if len(msgs) != 1 || msgs[0].(OpenedMsg).Err != nil || len(got) != 1 {
		t.Fatalf("OpenWith: %v, launched %v", msgs, got)
	}
	if m := OpenWith(mine, "file:///x")().(OpenedMsg); !m.Refused() || len(got) != 1 {
		t.Fatalf("OpenWith opened an invalid URL: %+v %v", m, got)
	}
}

func TestOpenWithNilUsesLauncher(t *testing.T) {
	old := Launcher
	ran := false
	Launcher = func(string) error { ran = true; return nil }
	t.Cleanup(func() { Launcher = old })
	OpenWith(nil, "https://x.io/a")()
	if !ran {
		t.Fatal("nil launch did not fall back to Launcher")
	}
}

func TestIsError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{ErrNoLink, false},
		{ErrRemote, true},
		{ErrInvalid, true},
		{errors.New("exit status 1"), true},
	} {
		if got := (OpenedMsg{Err: tc.err}).IsError(); got != tc.want {
			t.Errorf("IsError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestBindingWith(t *testing.T) {
	b := BindingWith("list", "Open", 3)
	if b.Action != Action || b.Key() != "o" || b.Group != "Open" || b.Footer != 3 || b.Ctx != "list" {
		t.Fatalf("BindingWith = %+v", b)
	}
	km := keys.New(b)
	got, ok := km.Resolve([]keys.Ctx{"list"}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}, keys.Opts{})
	if !ok || got.Action != Action {
		t.Fatalf("o resolved to %v %v", got.Action, ok)
	}
}

func TestSensitiveWrapper(t *testing.T) {
	if !Sensitive("https://x.io/cb?code=1") || Sensitive("https://x.io/a") {
		t.Fatal("Sensitive does not follow urls.Sensitive")
	}
	if !Valid("https://x.io/cb?code=1") {
		t.Fatal("Valid rejects sensitive URLs; it must not")
	}
}
