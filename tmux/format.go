package tmux

import (
	"strconv"
	"strings"
)

// How list-panes output is made unambiguous, measured on tmux 3.5a:
//
//   - tmux prints every control character except tab and newline as octal
//     text ("\037"), including one written into the -F format itself, so
//     \x1f or \x1e cannot serve as separators.
//   - tab and newline pass through raw, and both occur in data: a cwd may
//     hold a tab, an option value a newline. Splitting rows on "\n" and
//     fields on "\t" (as tmc and ccx do) misreads those panes.
//   - #{n:var} is the value's length in bytes.
//
// So the fixed fields (IDs, numbers, flags — never a tab) are tab-separated,
// and every free-text field is written as "<bytes>:<value>" and read by
// length. A value holding other control bytes prints longer than its length
// (each byte became \ooo); the parser then counts each \ooo as one byte, and
// failing that reads the field to its terminator. Such values come back in
// tmux's printed form, as Option returns them too.

var paneFixed = []string{
	"pane_id", "window_id", "session_id", "window_index", "pane_index", "pane_pid",
	"pane_active", "window_active", "pane_width", "pane_height",
}

// Free-text fields; user options are inserted between the names and the cwd.
var (
	paneNames = []string{"session_name", "window_name"}
	paneTail  = []string{"pane_current_path", "pane_current_command", "pane_title"}
)

// PaneFormat is the -F format Panes uses for the given user options.
func PaneFormat(options []string) string {
	var b strings.Builder
	for i, n := range paneFixed {
		if i > 0 {
			b.WriteByte('\t')
		}
		b.WriteString("#{" + n + "}")
	}
	for _, n := range freeFields(options) {
		b.WriteString("\t#{n:" + n + "}:#{" + n + "}")
	}
	return b.String()
}

func freeFields(options []string) []string {
	f := append([]string(nil), paneNames...)
	f = append(f, options...)
	return append(f, paneTail...)
}

// ParsePanes parses list-panes output made with PaneFormat(options). A row
// that does not parse is skipped: one odd pane must not hide the rest. The
// result is never nil.
func ParsePanes(out string, options []string) []Pane {
	panes := []Pane{}
	for out != "" {
		var p Pane
		var ok bool
		p, out, ok = parsePaneRow(out, options)
		if ok {
			panes = append(panes, p)
		}
	}
	return panes
}

// parsePaneRow reads one row from s and returns the rest after it.
func parsePaneRow(s string, options []string) (Pane, string, bool) {
	var fixed [10]string
	for i := range fixed {
		j := strings.IndexAny(s, "\t\n")
		if j < 0 || s[j] != '\t' {
			return Pane{}, skipLine(s), false
		}
		fixed[i], s = s[:j], s[j+1:]
	}
	names := freeFields(options)
	free := make([]string, len(names))
	for i := range free {
		term := byte('\t')
		if i == len(free)-1 {
			term = '\n'
		}
		v, rest, ok := lenField(s, term)
		if !ok {
			return Pane{}, skipLine(s), false
		}
		free[i], s = v, rest
	}

	p := Pane{
		ID: fixed[0], WindowID: fixed[1], SessionID: fixed[2],
		Active: fixed[6] == "1", WindowActive: fixed[7] == "1",
		SessionName: free[0], WindowName: free[1],
	}
	for _, n := range []struct {
		dst *int
		s   string
	}{{&p.WindowIndex, fixed[3]}, {&p.PaneIndex, fixed[4]}, {&p.PID, fixed[5]}, {&p.Width, fixed[8]}, {&p.Height, fixed[9]}} {
		v, err := strconv.Atoi(n.s)
		if err != nil {
			return Pane{}, s, false
		}
		*n.dst = v
	}
	if checkID(p.ID, "%") != nil {
		return Pane{}, s, false
	}
	k := len(paneNames)
	if len(options) > 0 {
		p.Options = make(map[string]string, len(options))
		for i, o := range options {
			p.Options[o] = free[k+i]
		}
	}
	k += len(options)
	p.Cwd, p.Command, p.Title = free[k], free[k+1], free[k+2]
	return p, s, true
}

// lenField reads "<n>:<n bytes>" followed by term (or the end of input) and
// returns the value and the input after term.
func lenField(s string, term byte) (val, rest string, ok bool) {
	c := strings.IndexByte(s, ':')
	if c <= 0 {
		return "", s, false
	}
	n, err := strconv.Atoi(s[:c])
	if err != nil || n < 0 {
		return "", s, false
	}
	body := s[c+1:]
	if n == len(body) {
		return body, "", true
	}
	if n < len(body) && body[n] == term {
		return body[:n], body[n+1:], true
	}
	// tmux printed control bytes as \ooo, so the printed value is longer
	// than n. Count each such escape as the one byte it stands for.
	if m, ok := escapedLen(body, n); ok && (m == len(body) || body[m] == term) {
		if m == len(body) {
			return body, "", true
		}
		return body[:m], body[m+1:], true
	}
	// Last resort: the field runs to the next terminator.
	j := strings.IndexByte(body, term)
	if j < 0 {
		if term == '\n' {
			return body, "", true
		}
		return "", s, false
	}
	return body[:j], body[j+1:], true
}

// escapedLen is how many printed bytes of s stand for n raw bytes when every
// "\ooo" (backslash, three octal digits) is one escaped byte. tmux leaves a
// literal backslash alone, so a value that really holds "\037" is ambiguous;
// the caller checks that the guess lands on the field terminator.
func escapedLen(s string, n int) (int, bool) {
	i := 0
	for ; n > 0 && i < len(s); n-- {
		if s[i] == '\\' && i+3 < len(s) && isOctal(s[i+1]) && isOctal(s[i+2]) && isOctal(s[i+3]) {
			i += 4
			continue
		}
		i++
	}
	return i, n == 0
}

func isOctal(b byte) bool { return b >= '0' && b <= '7' }

func skipLine(s string) string {
	if j := strings.IndexByte(s, '\n'); j >= 0 {
		return s[j+1:]
	}
	return ""
}
