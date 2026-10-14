package kitty

import (
	"reflect"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Queries the app writes (outside tmux, or wrapped with Caps.Wrap when tmux
// forwards the reply) and parses the reply of.
const (
	// CellSizeQuery asks for the cell size; the reply is CSI 6 ; h ; w t.
	CellSizeQuery = "\x1b[16t"
	// KeyboardQuery asks for the progressive keyboard flags (CSI u
	// protocol); a terminal that supports it replies CSI ? flags u, one that
	// does not ignores it.
	KeyboardQuery = "\x1b[?u"
)

// CSIReply returns the bytes of a CSI sequence Bubble Tea did not recognise.
// Bubble Tea v1 delivers those to Update as an unexported []byte type, so
// replies to CellSizeQuery and KeyboardQuery arrive that way; this reads it
// by kind instead of by name.
func CSIReply(msg tea.Msg) ([]byte, bool) {
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Uint8 {
		return nil, false
	}
	if v.Type().Name() != "unknownCSISequenceMsg" {
		return nil, false
	}
	b := v.Bytes()
	if len(b) < 3 || b[0] != 0x1b || b[1] != '[' {
		return nil, false
	}
	return b, true
}

// csiParams splits a reply "ESC [ <prefix> p1;p2… <final>" into its numbers.
func csiParams(reply []byte, prefix string, final byte) ([]int, bool) {
	s := string(reply)
	s, ok := strings.CutPrefix(s, "\x1b["+prefix)
	if !ok || len(s) < 2 || s[len(s)-1] != final {
		return nil, false
	}
	parts := strings.Split(s[:len(s)-1], ";")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

// ParseCellSize reads a reply to CellSizeQuery.
func ParseCellSize(reply []byte) (CellSize, bool) {
	p, ok := csiParams(reply, "", 't')
	if !ok || len(p) != 3 || p[0] != 6 || p[1] == 0 || p[2] == 0 {
		return CellSize{}, false
	}
	return CellSize{W: p[2], H: p[1]}, true
}

// ParseKeyboardFlags reads a reply to KeyboardQuery: the flags the terminal
// has enabled. ok means the terminal speaks the CSI u protocol.
func ParseKeyboardFlags(reply []byte) (flags int, ok bool) {
	p, ok := csiParams(reply, "?", 'u')
	if !ok || len(p) != 1 {
		return 0, false
	}
	return p[0], true
}
