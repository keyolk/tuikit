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
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/link/urls"
	"github.com/keyolk/tuikit/render"
)

// maxURL is the longest URL that is linked (urls.MaxLen).
const maxURL = urls.MaxLen

// Valid reports whether u is an absolute http or https URL with a host and
// nothing a terminal escape could be smuggled through (control characters,
// spaces). It is urls.Valid; import link/urls where Bubble Tea is not
// wanted. Valid accepts URLs that carry credentials; see Sensitive.
func Valid(u string) bool { return urls.Valid(u) }

// Sensitive reports whether u carries userinfo or a credential-like query
// or fragment key (urls.Sensitive). Valid does not reject such URLs: the
// caller decides, and a tool showing captured traffic keeps them plain text
// (neither Wrap nor Open them).
func Sensitive(u string) bool { return urls.Sensitive(u) }

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
	return "\x1b]8;;" + urls.Encode(u) + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// Span links the part of a row between prefix and suffix: markers, padding
// and badges stay outside the link, and the row's width is unchanged.
func Span(u, prefix, text, suffix string) string {
	return prefix + Wrap(u, text) + suffix
}

// Cell is text as a table cell exactly w cells wide that links to u: text is
// cut to w with ellipsis (render.Ellipsis() is the usual one, "" cuts hard),
// only the text is linked, and the padding stays outside the link so a
// cmd+click on the blank part of the cell opens nothing. okx wrote this by
// hand as linkCell.
func Cell(u, text string, w int, ellipsis string) string {
	if w <= 0 {
		return ""
	}
	t := render.TruncateMarker(text, w, ellipsis)
	return render.PadRight(Wrap(u, t), w)
}

// Find returns the http and https URLs in text, in order, escapes ignored.
// Trailing sentence punctuation is not part of a URL ("see https://x.io/a."),
// and neither is a closing bracket that has no opening one in the URL
// ("(https://x.io/a)").
func Find(text string) []string { return urls.Find(ansi.Strip(text)) }

// FindIndex returns the byte ranges of the URLs in plain text, by the rules
// of Find (urls.FindIndex). Use it to link URLs in text that is wrapped
// first: wrap the plain text, find the ranges in each line, and Wrap them
// after. Wrapping a linked string splits the escape across lines.
func FindIndex(text string) [][2]int { return urls.FindIndex(text) }

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

// IsError reports whether the result deserves an error toast: an opener
// failure, ErrRemote, a refused URL. It is false on success and for
// ErrNoLink, which is an answer ("no link here") rather than a failure, so
// apps pick the toast level from it instead of testing Err themselves.
func (m OpenedMsg) IsError() bool {
	return m.Err != nil && !errors.Is(m.Err, ErrNoLink)
}

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
func Open(u string) tea.Cmd { return OpenWith(nil, u) }

// OpenWith is Open through launch instead of Launcher: an app whose config
// names its own opener (ccx's browser command) passes it here rather than
// assigning the package-wide Launcher, which every other caller and test
// shares. A nil launch means Launcher. launch runs off the UI goroutine and
// gets only a Valid URL.
func OpenWith(launch func(string) error, u string) tea.Cmd {
	return func() tea.Msg {
		if !Valid(u) {
			return OpenedMsg{URL: u, Err: ErrInvalid}
		}
		if launch == nil {
			launch = Launcher
		}
		return OpenedMsg{URL: u, Err: launch(u)}
	}
}

// OpenFirst opens the first of urls and says how many there were (a message
// or a card often carries several). With no URL it opens nothing and reports
// ErrNoLink, so the key still answers. Invalid URLs are skipped, not opened,
// and not counted: Of is the number of Valid URLs, so "the first of 3" never
// counts a file: link the user could not have opened.
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
	return exec.Command(name, urls.Encode(u)).Run()
}

// Action is the binding name for opening the selected item's link.
const Action = "link.open"

// Binding is the fleet's open-link key, `o`, in ctx. The app resolves it like
// any binding and passes the selected item's URL to Open; when the item has
// none, it shows that instead of doing nothing.
func Binding(ctx keys.Ctx) keys.Binding {
	return keys.Binding{Action: Action, Keys: []string{"o"}, Ctx: ctx, Help: "open link"}
}

// BindingWith is Binding placed in a help group and given a footer priority
// (0 keeps it out of the footer), the two fields every adopter set by hand.
func BindingWith(ctx keys.Ctx, group string, footer int) keys.Binding {
	b := Binding(ctx)
	b.Group, b.Footer = group, footer
	return b
}
