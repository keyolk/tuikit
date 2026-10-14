package keys

import (
	"maps"
	"strings"
)

// Names turns key strings into what the footer, help and palette display.
// tea.KeyMsg.String() is right for matching but wrong for reading: the space
// bar is " ", which shows as nothing in "  open".
//
// The zero value spells keys as words, naming only the space bar ("space").
// Glyphs swaps named keys for symbols (enter → ↵, up → ↑); turn it on only
// when the terminal draws Unicode — theme.Theme.KeyNames does. Caret writes
// ctrl chords the terminal way (ctrl+x → ^x). With adds per-key overrides,
// which win over both.
//
// Names is a value: With returns a copy with its own map, so a Names stored
// in a style and copied with the model never changes underneath a copy.
type Names struct {
	Glyphs bool
	Caret  bool
	over   map[string]string
}

var (
	wordNames  = map[string]string{" ": "space"}
	glyphNames = map[string]string{
		" ": "space", "enter": "↵", "tab": "⇥", "shift+tab": "⇤", "backspace": "⌫",
		"up": "↑", "down": "↓", "left": "←", "right": "→",
	}
)

// With returns a copy of n that displays key as name.
func (n Names) With(key, name string) Names {
	over := maps.Clone(n.over)
	if over == nil {
		over = map[string]string{}
	}
	over[key] = name
	n.over = over
	return n
}

// Name is the display form of key.
func (n Names) Name(key string) string {
	if v, ok := n.over[key]; ok {
		return v
	}
	table := wordNames
	if n.Glyphs {
		table = glyphNames
	}
	if v, ok := table[key]; ok {
		return v
	}
	if rest, ok := strings.CutPrefix(key, "ctrl+"); ok && n.Caret && rest != "" {
		return "^" + rest
	}
	return key
}

// Join displays several keys as one: concatenated when every name is a single
// cell ("hjkl", "↑↓"), slash-separated otherwise ("enter/o", "pgup/pgdown"),
// so a group never reads as one long key name.
func (n Names) Join(keys []string) string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = n.Name(k)
	}
	return joinNames(names, "")
}
