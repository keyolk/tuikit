package tmux

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTeaKeyToTmux(t *testing.T) {
	cases := []struct {
		msg  tea.KeyMsg
		want Key
		ok   bool
	}{
		{tea.KeyMsg{Type: tea.KeyEnter}, Key{Text: "Enter"}, true},
		{tea.KeyMsg{Type: tea.KeyBackspace}, Key{Text: "BSpace"}, true},
		{tea.KeyMsg{Type: tea.KeyTab}, Key{Text: "Tab"}, true},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, Key{Text: "BTab"}, true},
		{tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, Key{Text: "Space"}, true},
		{tea.KeyMsg{Type: tea.KeyEsc}, Key{Text: "Escape"}, true},
		{tea.KeyMsg{Type: tea.KeyPgDown}, Key{Text: "PageDown"}, true},
		{tea.KeyMsg{Type: tea.KeyDelete}, Key{Text: "DC"}, true},
		{tea.KeyMsg{Type: tea.KeyF5}, Key{Text: "F5"}, true},
		{tea.KeyMsg{Type: tea.KeyCtrlC}, Key{Text: "C-c"}, true},
		{tea.KeyMsg{Type: tea.KeyCtrlR}, Key{Text: "C-r"}, true},
		{tea.KeyMsg{Type: tea.KeyCtrlX}, Key{Text: "C-x"}, true}, // ccx's table stopped at a handful of letters
		{tea.KeyMsg{Type: tea.KeyUp, Alt: true}, Key{Text: "M-Up"}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, Key{Text: "M-x"}, true},
		// Typed text is literal: one rune, Hangul (ccx dropped it: len("한") is 3 bytes), batches, pastes.
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, Key{Text: "a", Literal: true}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("한")}, Key{Text: "한", Literal: true}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Enter")}, Key{Text: "Enter", Literal: true}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb"), Paste: true}, Key{Text: "a\nb", Literal: true}, true},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab"), Alt: true}, Key{}, false},
		{tea.KeyMsg{Type: tea.KeyRunes}, Key{}, false},
		{tea.KeyMsg{Type: tea.KeyCtrlBackslash}, Key{}, false},
	}
	for _, c := range cases {
		got, ok := TeaKeyToTmux(c.msg)
		if got != c.want || ok != c.ok {
			t.Errorf("%q (type %d): got %+v %v, want %+v %v", c.msg.String(), c.msg.Type, got, ok, c.want, c.ok)
		}
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"":              "''",
		"vim":           "vim",
		"/usr/bin/nvim": "/usr/bin/nvim",
		"a b":           "'a b'",
		"it's":          `'it'\''s'`,
		"$HOME":         "'$HOME'",
		"a;rm -rf /":    "'a;rm -rf /'",
		"한글 경로":         "'한글 경로'",
		"x\ny":          "'x\ny'",
	} {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommandArgs(t *testing.T) {
	cases := []struct {
		c    Command
		want []string
	}{
		{Command{}, nil},
		{Command{Shell: "make && ./run"}, []string{"make && ./run"}},
		{Command{Argv: []string{"my prog"}}, []string{"'my prog'"}},
		{Command{Argv: []string{"git", "log", "--format=%h %s"}, Shell: "ignored"}, []string{"git", "log", "--format=%h %s"}},
	}
	for _, c := range cases {
		got := c.c.args()
		if len(got) != len(c.want) {
			t.Errorf("%+v: %q", c.c, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%+v: %q, want %q", c.c, got, c.want)
			}
		}
	}
}
