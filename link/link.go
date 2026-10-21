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

// Wrap returns text as an OSC 8 hyperlink to u. An empty text shows u. A u
// that is not Valid returns text unchanged, so a bad URL costs the link, not
// the label. The escape has no width (render.Width, lipgloss.Width and
// ansi.Truncate all skip it), so a wrapped label fits a column like the
// plain one.
func Wrap(u, text string) string {
	if text == "" {
		text = u
	}
	if !Valid(u) {
		return text
	}
	return "\x1b]8;;" + u + "\x1b\\" + text + "\x1b]8;;\x1b\\"
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

// OpenedMsg reports the result of Open.
type OpenedMsg struct {
	URL string
	Err error
}

// Note is the toast text for the result: "opened <url>" or why not.
func (m OpenedMsg) Note() string {
	if m.Err != nil {
		return "open failed: " + m.Err.Error()
	}
	return "opened " + m.URL
}

var (
	// ErrInvalid: the URL is not an http or https URL (see Valid).
	ErrInvalid = errors.New("not an http(s) URL")
	// ErrRemote: the app runs over SSH, where the browser would open on the
	// remote host, not in front of the user. Show the URL instead.
	ErrRemote = errors.New("no local browser over SSH")
	// ErrNoOpener: the platform has no known URL opener.
	ErrNoOpener = errors.New("no URL opener on this platform")
)

// Launcher opens a URL in the browser; Open calls it. Tests replace it so
// that running Open's command is safe and the URL can be asserted from the
// OpenedMsg.
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
	return exec.Command(name, u).Run()
}

// Action is the binding name for opening the selected item's link.
const Action = "link.open"

// Binding is the fleet's open-link key, `o`, in ctx. The app resolves it like
// any binding and passes the selected item's URL to Open; when the item has
// none, it shows that instead of doing nothing.
func Binding(ctx keys.Ctx) keys.Binding {
	return keys.Binding{Action: Action, Keys: []string{"o"}, Ctx: ctx, Help: "open link"}
}
