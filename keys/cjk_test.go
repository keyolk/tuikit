package keys

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Ported from ccproxy internal/keymap, okx internal/tui/keymap_test.go and ccx
// internal/tui/keymap_langmap_test.go.

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestLatinMapsJamoByPhysicalPosition(t *testing.T) {
	cases := map[rune]rune{'ㅂ': 'q', 'ㅁ': 'a', 'ㅋ': 'z', 'ㅓ': 'j', 'ㅃ': 'Q', 'ㄲ': 'R'}
	for jamo, want := range cases {
		got, ok := Latin(jamo)
		if !ok || got != want {
			t.Errorf("Latin(%q) = %q, %v; want %q, true", jamo, got, ok, want)
		}
	}
}

func TestLatinLeavesNonJamoAlone(t *testing.T) {
	// A composed syllable only reaches the TUI as committed text, never as a
	// shortcut, so it is not remapped.
	for _, r := range []rune{'q', 'Z', '가', '1'} {
		if _, ok := Latin(r); ok {
			t.Errorf("Latin(%q) reported a mapping; want none", r)
		}
	}
}

func TestNormalizeCJK(t *testing.T) {
	cases := []struct{ in, want string }{
		// unshifted jamo → lowercase Latin at the same QWERTY position
		{"ㅂ", "q"}, {"ㅌ", "x"}, {"ㅓ", "j"}, {"ㅏ", "k"}, {"ㅣ", "l"}, {"ㅁ", "a"}, {"ㅇ", "d"},
		// shifted jamo restore the shift: uppercase Latin
		{"ㄲ", "R"}, {"ㅃ", "Q"}, {"ㄸ", "E"}, {"ㅉ", "W"}, {"ㅆ", "T"}, {"ㅒ", "O"}, {"ㅖ", "P"},
		// already-Latin and unmapped runes pass through
		{"q", "q"}, {"R", "R"}, {"0", "0"}, {"가", "가"}, {"😀", "😀"},
	}
	for _, c := range cases {
		if got := NormalizeCJK(runes(c.in)).String(); got != c.want {
			t.Errorf("NormalizeCJK(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeCJKLeavesNonRuneKeysUntouched(t *testing.T) {
	for _, in := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyCtrlC}, {Type: tea.KeyEsc}, {Type: tea.KeyCtrlS}} {
		if got := NormalizeCJK(in); got.String() != in.String() {
			t.Errorf("NormalizeCJK(%q) = %q; want unchanged", in.String(), got.String())
		}
	}
}

func TestNormalizeCJKLeavesPasteAndAltUntouched(t *testing.T) {
	// A pasted Korean string must reach a text field verbatim, not be remapped
	// character by character.
	paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("안녕"), Paste: true}
	if got := NormalizeCJK(paste); got.String() != paste.String() || !got.Paste {
		t.Errorf("paste mangled: %q paste=%v", got.String(), got.Paste)
	}
	pasteOne := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'ㅂ'}, Paste: true}
	if got := NormalizeCJK(pasteOne); got.String() != pasteOne.String() {
		t.Errorf("pasted jamo was rewritten to %q", got.String())
	}
	alt := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'ㅂ'}, Alt: true}
	if got := NormalizeCJK(alt); got.String() != alt.String() {
		t.Errorf("alt chord was rewritten to %q", got.String())
	}
}

func TestNormalizeCJKLeavesMultiRuneMessagesUntouched(t *testing.T) {
	in := runes("ㅂㅈ")
	if got := NormalizeCJK(in); got.String() != in.String() {
		t.Errorf("multi-rune message was rewritten to %q", got.String())
	}
}

// From ccx TestDefaultKeymapIsReachableUnderCJK (CPLAT-12413).
func TestReachableUnderCJK(t *testing.T) {
	for _, k := range []string{"q", "f", "Q", "W", "E", "R", "T", "O", "P", "ctrl+f", "enter", "?", "G1"} {
		if !ReachableUnderCJK(k) {
			t.Errorf("%q reported unreachable", k)
		}
	}
	for _, k := range []string{"F", "A", "J", "K", "L", "M", "X", "Z"} {
		if ReachableUnderCJK(k) {
			t.Errorf("%q reported reachable; Shift on that key sends the bare jamo", k)
		}
	}
	// The reachable set is exactly the uppercase targets of the jamo table.
	for _, l := range hangulToLatin {
		if l >= 'A' && l <= 'Z' && !ReachableUnderCJK(string(l)) {
			t.Errorf("%q is produced by a shifted jamo but reported unreachable", l)
		}
	}
}
