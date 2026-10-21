// Package link makes URLs in a TUI reachable two ways, as the fleet
// convention says (E11): the text is an OSC 8 hyperlink, so the terminal
// opens it on cmd/ctrl+click even while the app captures the mouse, and the
// `o` key opens the selected item's URL.
//
// OSC 8 matters most where the URL is not on screen: a PR title, an issue
// key, a "logs" label. A URL printed in full is already clickable in most
// terminals, which detect URLs in plain text. Behind tmux, links need
//
//	set -as terminal-features '*:hyperlinks'
//
// (tmux 3.4+); a terminal that does not know OSC 8 ignores it and shows the
// text.
//
// Only absolute http and https URLs are linked or opened. The URL comes from
// data (an API, a log line), and a file:, javascript: or custom-scheme URL
// is not something a click or a key in a TUI should launch.
package link

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/keys"
)

// maxURL is the longest URL that is linked. Terminals cap OSC 8 URIs
// (VTE at 2083 bytes) and drop longer ones, sometimes with the text.
const maxURL = 2048

// Valid reports whether u is an absolute http or https URL with a host and
// nothing a terminal escape could be smuggled through (control characters,
// spaces).
func Valid(u string) bool {
	if u == "" || len(u) > maxURL {
		return false
	}
	for i := 0; i < len(u); i++ {
		if c := u[i]; c <= ' ' || c == 0x7f {
			return false
		}
	}
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return false
	}
	return p.Scheme == "http" || p.Scheme == "https"
}

// Wrap returns text as an OSC 8 hyperlink to u. A u that is not Valid
// returns text unchanged, so a bad URL costs the link, not the label. An
// empty text stays empty: a label cut to nothing by its column must not turn
// into the whole URL and widen the row (pass u as the text to show it).
//
// The escape has no width (render.Width, lipgloss.Width and ansi.Truncate
// skip it), so a wrapped label fits a column like the plain one. That holds
// only for an ASCII URL: the width counters measure non-ASCII bytes inside
// the escape as text, so "https://x.io/한글" made a 2-cell label 11 cells
// wide. Wrap percent-encodes non-ASCII (the same URL to a browser).
func Wrap(u, text string) string {
	if text == "" || !Valid(u) {
		return text
	}
	return "\x1b]8;;" + asciiURL(u) + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// asciiURL percent-encodes every non-ASCII byte of u, as browsers do.
func asciiURL(u string) string {
	for i := 0; i < len(u); i++ {
		if u[i] >= 0x80 {
			var b strings.Builder
			for j := 0; j < len(u); j++ {
				if c := u[j]; c >= 0x80 {
					fmt.Fprintf(&b, "%%%02X", c)
				} else {
					b.WriteByte(c)
				}
			}
			return b.String()
		}
	}
	return u
}

// Span links the part of a row between prefix and suffix: markers, padding
// and badges stay outside the link, and the row's width is unchanged.
func Span(u, prefix, text, suffix string) string {
	return prefix + Wrap(u, text) + suffix
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// Find returns the http and https URLs in text, in order, escapes ignored.
// Trailing sentence punctuation is not part of a URL ("see https://x.io/a."),
// and neither is a closing bracket that has no opening one in the URL
// ("(https://x.io/a)").
func Find(text string) []string {
	var out []string
	for _, u := range urlPattern.FindAllString(ansi.Strip(text), -1) {
		u = trimURL(u)
		if Valid(u) {
			out = append(out, u)
		}
	}
	return out
}

func trimURL(u string) string {
	for u != "" {
		last := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?", last) >= 0:
			u = u[:len(u)-1]
		case last == ')' && strings.Count(u, "(") < strings.Count(u, ")"),
			last == ']' && strings.Count(u, "[") < strings.Count(u, "]"):
			u = u[:len(u)-1]
		default:
			return u
		}
	}
	return u
}

// OpenedMsg reports the result of Open or OpenFirst.
type OpenedMsg struct {
	URL string
	Err error
	// Of is how many URLs OpenFirst chose from; 0 or 1 means there was no
	// choice. The note says "the first of N" so the user knows to look.
	Of int
}

// Refused reports that the URL was never handed to the opener because it is
// not an http(s) URL. An app that filters with Valid before Open never sees
// it; a test can assert on it to prove the app's own filter.
func (m OpenedMsg) Refused() bool { return errors.Is(m.Err, ErrInvalid) }

// Notes is the text of the result toasts. Replace the fields for a UI in
// another language; %s is the URL or the error, %d the count.
type Notes struct {
	Opened, OpenedFirst, Failed, None string
}

// EnglishNotes is what Note uses.
var EnglishNotes = Notes{
	Opened:      "opened %s",
	OpenedFirst: "opened %s (the first of %d links)",
	Failed:      "open failed: %s",
	None:        "no link here",
}

// Note is the toast text for the result: "opened <url>", "opened <url> (the
// first of 3 links)" or why not.
func (m OpenedMsg) Note() string { return m.NoteWith(EnglishNotes) }

// NoteWith is Note in other words.
func (m OpenedMsg) NoteWith(n Notes) string {
	switch {
	case errors.Is(m.Err, ErrNoLink):
		return n.None
	case m.Err != nil:
		return fmt.Sprintf(n.Failed, m.Err.Error())
	case m.Of > 1:
		return fmt.Sprintf(n.OpenedFirst, m.URL, m.Of)
	}
	return fmt.Sprintf(n.Opened, m.URL)
}

var (
	// ErrInvalid: the URL is not an http or https URL (see Valid).
	ErrInvalid = errors.New("not an http(s) URL")
	// ErrRemote: the app runs over SSH, where the browser would open on the
	// remote host, not in front of the user. Show the URL instead.
	ErrRemote = errors.New("no local browser over SSH")
	// ErrNoOpener: the platform has no known URL opener.
	ErrNoOpener = errors.New("no URL opener on this platform")
	// ErrNoLink: OpenFirst got no URL. Show "no link here" (Note) rather
	// than doing nothing, so a key press always answers.
	ErrNoLink = errors.New("no link")
)

// Launcher opens a URL in the browser; Open calls it. Tests replace it so
// that running Open's command is safe and the URL can be asserted from the
// OpenedMsg. It runs inside the command, on another goroutine: a stub must
// record and report with t.Errorf, not t.Fatal (FailNow off the test
// goroutine is lost, and the drained command reports nothing).
var Launcher = launch

// Open opens u in the browser off the UI thread and reports OpenedMsg. It
// runs the platform opener with u as one argument, never through a shell,
// and with no terminal attached, so the opener cannot draw over the TUI.
func Open(u string) tea.Cmd {
	return func() tea.Msg {
		if !Valid(u) {
			return OpenedMsg{URL: u, Err: ErrInvalid}
		}
		return OpenedMsg{URL: u, Err: Launcher(u)}
	}
}

// OpenFirst opens the first of urls and says how many there were (a message
// or a card often carries several). With no URL it opens nothing and reports
// ErrNoLink, so the key still answers. Invalid URLs are skipped, not opened.
func OpenFirst(urls []string) tea.Cmd {
	var valid []string
	for _, u := range urls {
		if Valid(u) {
			valid = append(valid, u)
		}
	}
	if len(valid) == 0 {
		return func() tea.Msg { return OpenedMsg{Err: ErrNoLink} }
	}
	open := Open(valid[0])
	return func() tea.Msg {
		m := open().(OpenedMsg)
		m.Of = len(valid)
		return m
	}
}

var openers = map[string]string{"darwin": "open", "linux": "xdg-open"}

func launch(u string) error {
	if os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != "" {
		return ErrRemote
	}
	name, ok := openers[runtime.GOOS]
	if !ok {
		return ErrNoOpener
	}
	// Valid already holds: u starts with http, so the opener cannot read it
	// as a flag. Stdin, stdout and stderr stay nil (/dev/null).
	return exec.Command(name, asciiURL(u)).Run()
}

// Action is the binding name for opening the selected item's link.
const Action = "link.open"

// Binding is the fleet's open-link key, `o`, in ctx. The app resolves it like
// any binding and passes the selected item's URL to Open; when the item has
// none, it shows that instead of doing nothing.
func Binding(ctx keys.Ctx) keys.Binding {
	return keys.Binding{Action: Action, Keys: []string{"o"}, Ctx: ctx, Help: "open link"}
}
