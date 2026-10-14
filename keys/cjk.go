package keys

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// With the OS input source set to Korean, pressing the `q` key emits `ㅂ`, so
// every single-letter shortcut silently stops working until the user switches
// back to English. NormalizeCJK maps each jamo back to the Latin key at the same
// physical position on a US QWERTY keyboard — the same idea as Vim's `langmap`.
//
// Merged from okx (internal/tui/keymap.go), ccproxy (internal/keymap) and ccx
// (internal/tui/keymap_langmap.go); the three were identical in behaviour.

// hangulToLatin maps jamo produced by the 2-set (두벌식) Korean layout to the
// Latin key at the same physical position on a US QWERTY keyboard.
//
// Shifted jamo (double consonants, ㅒ/ㅖ) map to the uppercase Latin letter,
// which is what the same physical chord would have produced in English. That is
// the only way shift survives the IME: tea.Key has no Shift field, and on every
// other key Shift produces the same jamo as the bare key.
var hangulToLatin = map[rune]rune{
	// unshifted row
	'ㅂ': 'q', 'ㅈ': 'w', 'ㄷ': 'e', 'ㄱ': 'r', 'ㅅ': 't',
	'ㅛ': 'y', 'ㅕ': 'u', 'ㅑ': 'i', 'ㅐ': 'o', 'ㅔ': 'p',
	'ㅁ': 'a', 'ㄴ': 's', 'ㅇ': 'd', 'ㄹ': 'f', 'ㅎ': 'g',
	'ㅗ': 'h', 'ㅓ': 'j', 'ㅏ': 'k', 'ㅣ': 'l',
	'ㅋ': 'z', 'ㅌ': 'x', 'ㅊ': 'c', 'ㅍ': 'v', 'ㅠ': 'b',
	'ㅜ': 'n', 'ㅡ': 'm',
	// shifted row
	'ㅃ': 'Q', 'ㅉ': 'W', 'ㄸ': 'E', 'ㄲ': 'R', 'ㅆ': 'T',
	'ㅒ': 'O', 'ㅖ': 'P',
}

// Latin returns the Latin key at the same physical position as the jamo r, and
// whether r is a jamo this package maps at all.
func Latin(r rune) (rune, bool) {
	latin, ok := hangulToLatin[r]
	return latin, ok
}

// NormalizeCJK rewrites a single-jamo key message to the Latin key at the same
// physical position, so shortcuts fire under a Korean input source.
//
// The message is returned unchanged when it is not a single rune, is a paste,
// or carries alt — those already arrive as Latin, and rewriting a chord would
// break bindings like alt+f. Ctrl chords never reach here as KeyRunes.
//
// Callers must not apply this while a text field owns the keyboard, so Korean
// can still be typed into a filter verbatim (Keymap.ResolveRaw exists for that).
func NormalizeCJK(msg tea.KeyMsg) tea.KeyMsg {
	if msg.Type != tea.KeyRunes || msg.Paste || msg.Alt || len(msg.Runes) != 1 {
		return msg
	}
	latin, ok := hangulToLatin[msg.Runes[0]]
	if !ok {
		return msg
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{latin}}
}

// ReachableUnderCJK reports whether key can be typed while a 2-set Korean input
// source is active.
//
// Only bare uppercase letters are at risk. Shift yields a distinct jamo on
// q w e r t o p alone; on every other key the shifted and bare press send the
// same jamo, which normalizes to lowercase. A binding on `F` is therefore dead
// for that user — and when a lowercase action shares the letter it silently
// receives both presses (ccx CPLAT-12413: "f" fold-all swallowed "F" expand).
// Named keys and ctrl/alt chords survive the IME.
func ReachableUnderCJK(key string) bool {
	if len(key) != 1 || key[0] < 'A' || key[0] > 'Z' {
		return true
	}
	return strings.ContainsRune("QWERTOP", rune(key[0]))
}

// TypedText is the text k types into a field: the runes of a key press, a
// batch of runes delivered at once, or a bracketed paste, and " " for the
// space bar. ok is false for everything else — named keys, ctrl chords,
// alt+letter — which the field should treat as commands.
//
// Jamo come back as typed, never normalized: a text field wants Korean. The
// text may hold control characters from a paste; input.Field strips them.
func TypedText(k tea.KeyMsg) (text string, ok bool) {
	switch {
	case k.Alt:
		return "", false
	case k.Type == tea.KeySpace:
		return " ", true
	case k.Type == tea.KeyRunes && len(k.Runes) > 0:
		return string(k.Runes), true
	}
	return "", false
}
