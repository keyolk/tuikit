package tuitest

import "strings"

// Link is one OSC 8 hyperlink in a frame: its URL and the text it covers,
// escapes stripped. Row is the frame line it starts on.
type Link struct {
	Row       int
	URL, Text string
}

// Links returns the OSC 8 hyperlinks in frame, in reading order. It accepts
// both terminators (ESC \ and BEL): lipgloss closes and reopens a link with
// BEL when it wraps one across lines. A link left open at the end of a line
// is closed there, as terminals do.
func Links(frame string) []Link {
	var out []Link
	var cur *Link
	var text strings.Builder
	row := 0
	end := func() {
		if cur != nil {
			cur.Text = text.String()
			out = append(out, *cur)
			cur = nil
			text.Reset()
		}
	}
	for i := 0; i < len(frame); {
		c := frame[i]
		switch {
		case c == 0x1b && i+1 < len(frame) && frame[i+1] == ']':
			j := i + 2
			for j < len(frame) && frame[j] != 0x07 && !(frame[j] == 0x1b && j+1 < len(frame) && frame[j+1] == '\\') {
				j++
			}
			body := frame[i+2 : j]
			if j < len(frame) && frame[j] == 0x1b {
				j++
			}
			i = j + 1
			if !strings.HasPrefix(body, "8;") {
				continue
			}
			// 8;params;URI — an empty URI closes the link.
			uri := body[2:]
			if k := strings.IndexByte(uri, ';'); k >= 0 {
				uri = uri[k+1:]
			}
			end()
			if uri != "" {
				cur = &Link{Row: row, URL: uri}
			}
		case c == 0x1b && i+1 < len(frame) && frame[i+1] == '[':
			j := i + 2
			for j < len(frame) && (frame[j] < 0x40 || frame[j] > 0x7e) {
				j++
			}
			i = j + 1
		case c == '\n':
			end()
			row++
			i++
		default:
			if cur != nil {
				text.WriteByte(c)
			}
			i++
		}
	}
	end()
	return out
}
