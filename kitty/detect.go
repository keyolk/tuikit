package kitty

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// OverrideEnv forces detection: "1" on, "0" off. An app with its own
// variable maps it in the env func it passes to Detect.
const OverrideEnv = "TUIKIT_KITTY"

// Caps is what the terminal can do, as a value the model keeps.
type Caps struct {
	// Supported: the outer terminal speaks the Kitty graphics protocol.
	Supported bool
	// Terminal is "kitty", "ghostty" or "wezterm"; "" when forced by
	// OverrideEnv or unknown.
	Terminal string
	// InTmux: $TMUX is set.
	InTmux bool
	// NeedsPassthrough: every sequence must be wrapped in tmux's DCS
	// passthrough to reach the outer terminal (InTmux && Supported).
	NeedsPassthrough bool
	// PassthroughMode is tmux's allow-passthrough: "on", "all" or "off".
	// "" outside tmux, when unsupported, or when the query failed.
	PassthroughMode string
	// Pane is $TMUX_PANE, the target for pane queries.
	Pane string
}

// TmuxQuerier runs one tmux command and returns its trimmed stdout. The
// real one is ExecTmux; tests pass a map.
type TmuxQuerier interface {
	Run(args ...string) (string, error)
}

// ExecTmux runs the tmux binary. It blocks: call it from main or a tea.Cmd.
type ExecTmux struct{}

// Run implements TmuxQuerier.
func (ExecTmux) Run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Detect reports the terminal's graphics support. env is usually os.Getenv;
// tmux may be nil outside tmux. It runs tmux commands when $TMUX is set, so
// call it once from main (before the program starts) and keep the value.
//
// Order: OverrideEnv, then TERM_PROGRAM / TERM / KITTY_WINDOW_ID in our own
// environment, then — inside tmux, where TERM_PROGRAM reads "tmux" — the
// outer terminal as tmux recorded it: the server's global TERM_PROGRAM and
// KITTY_WINDOW_ID (tmux keeps the launching shell's environment), then the
// attached client's termname.
func Detect(env func(string) string, tmux TmuxQuerier) Caps {
	if env == nil {
		env = func(string) string { return "" }
	}
	c := Caps{InTmux: env("TMUX") != "", Pane: env("TMUX_PANE")}
	if !c.InTmux {
		tmux = nil
	}
	switch env(OverrideEnv) {
	case "0":
		return c
	case "1":
		c.Supported = true
	default:
		c.Terminal = terminalFromEnv(env)
		if c.Terminal == "" && tmux != nil {
			c.Terminal = terminalFromTmux(tmux)
		}
		c.Supported = c.Terminal != ""
	}
	c.NeedsPassthrough = c.InTmux && c.Supported
	if c.NeedsPassthrough && tmux != nil {
		if mode, err := tmux.Run("show-options", "-gv", "allow-passthrough"); err == nil {
			c.PassthroughMode = mode
		}
	}
	return c
}

func terminalFromEnv(env func(string) string) string {
	if t := terminalFromProgram(env("TERM_PROGRAM")); t != "" {
		return t
	}
	if t := terminalFromTerm(env("TERM")); t != "" {
		return t
	}
	if env("KITTY_WINDOW_ID") != "" || env("KITTY_PID") != "" {
		return "kitty"
	}
	return ""
}

func terminalFromProgram(p string) string {
	switch p {
	case "kitty":
		return "kitty"
	case "ghostty":
		return "ghostty"
	case "WezTerm":
		return "wezterm"
	}
	return ""
}

func terminalFromTerm(term string) string {
	switch {
	case strings.HasPrefix(term, "xterm-kitty"):
		return "kitty"
	case strings.HasPrefix(term, "xterm-ghostty"):
		return "ghostty"
	}
	return ""
}

// terminalFromTmux reads the outer terminal from tmux. Errors mean "not
// known", not failure: an unset variable makes show-environment exit 1.
func terminalFromTmux(tmux TmuxQuerier) string {
	if v, err := tmux.Run("show-environment", "-g", "TERM_PROGRAM"); err == nil {
		// "TERM_PROGRAM=kitty", or "-TERM_PROGRAM" when unset.
		if p, ok := strings.CutPrefix(v, "TERM_PROGRAM="); ok {
			if t := terminalFromProgram(p); t != "" {
				return t
			}
		}
	}
	if v, err := tmux.Run("show-environment", "-g", "KITTY_WINDOW_ID"); err == nil && strings.HasPrefix(v, "KITTY_WINDOW_ID=") {
		return "kitty"
	}
	if v, err := tmux.Run("display-message", "-p", "#{client_termname}"); err == nil {
		return terminalFromTerm(v)
	}
	return ""
}

// PassthroughMsg reports EnsurePassthroughCmd's result. Mode is the mode now
// in effect; on error it is the old one.
type PassthroughMsg struct {
	Mode string
	Err  error
}

// EnsurePassthroughCmd sets tmux's allow-passthrough to "all", off the UI
// loop, and returns nil when nothing needs doing.
//
// Why "all" and not "on" (ccx): "on" passes DCS passthrough only from the
// visible pane. A cursor-placed image drawn while the pane was visible then
// stays on the terminal after the user switches tmux windows, because the
// delete sent from the now-hidden pane is dropped. "all" lets the delete
// through.
func EnsurePassthroughCmd(c Caps, tmux TmuxQuerier) tea.Cmd {
	if !c.NeedsPassthrough || c.PassthroughMode == "all" || tmux == nil {
		return nil
	}
	old := c.PassthroughMode
	return func() tea.Msg {
		if _, err := tmux.Run("set-option", "-g", "allow-passthrough", "all"); err != nil {
			return PassthroughMsg{Mode: old, Err: err}
		}
		return PassthroughMsg{Mode: "all"}
	}
}

// WithPassthrough returns c with the mode from msg applied.
func (c Caps) WithPassthrough(msg PassthroughMsg) Caps {
	c.PassthroughMode = msg.Mode
	return c
}
