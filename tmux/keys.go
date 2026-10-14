package tmux

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Key is one argument for send-keys: a key name, or text to type with -l.
type Key struct {
	Text    string
	Literal bool
}

// teaNames maps Bubble Tea key names to tmux key names.
var teaNames = map[string]string{
	"enter": "Enter", "backspace": "BSpace", "tab": "Tab", "shift+tab": "BTab",
	"space": "Space", " ": "Space", "esc": "Escape",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right",
	"home": "Home", "end": "End", "pgup": "PageUp", "pgdown": "PageDown",
	"delete": "DC", "insert": "IC",
	"f1": "F1", "f2": "F2", "f3": "F3", "f4": "F4", "f5": "F5", "f6": "F6",
	"f7": "F7", "f8": "F8", "f9": "F9", "f10": "F10", "f11": "F11", "f12": "F12",
}

// TeaKeyToTmux maps a Bubble Tea key press to what send-keys needs.
//
// Typed text (one rune or a batch, Korean included, and pastes) is literal,
// so tmux never reads "Enter" typed by the user as the Enter key. Named keys
// map to tmux names, ctrl+<letter> to C-<letter>, and Alt adds M-. ok is
// false for keys tmux cannot express.
func TeaKeyToTmux(msg tea.KeyMsg) (Key, bool) {
	if msg.Type == tea.KeyRunes {
		if len(msg.Runes) == 0 {
			return Key{}, false
		}
		if msg.Alt && !msg.Paste {
			if len(msg.Runes) != 1 {
				return Key{}, false
			}
			return Key{Text: "M-" + string(msg.Runes)}, true
		}
		return Key{Text: string(msg.Runes), Literal: true}, true
	}
	name := msg.String()
	alt := strings.HasPrefix(name, "alt+")
	name = strings.TrimPrefix(name, "alt+")
	k, ok := teaNames[name]
	if !ok {
		rest, isCtrl := strings.CutPrefix(name, "ctrl+")
		if !isCtrl || len(rest) != 1 || rest[0] < 'a' || rest[0] > 'z' {
			return Key{}, false
		}
		k = "C-" + rest
	}
	if alt {
		k = "M-" + k
	}
	return Key{Text: k}, true
}

// ShellQuote quotes s as one sh word. tmux runs a pane's single-word command
// through "sh -c", so anything spliced into one must be quoted.
func ShellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`;&|<>()*?[]#~{}!=%^,") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
