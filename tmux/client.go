// Package tmux drives the tmux server a TUI runs under: list panes, read and
// write options, send keys, capture, split, pop up, jump and kill — through a
// Runner, so tests script a Fake and never touch the developer's real panes.
//
// Every target is a tmux ID (%pane, @window, $session). Indices move when a
// window is renumbered or a pane is swapped, and an empty target means "the
// current pane", which for send-keys is the app's own pane; both are rejected
// with ErrBadTarget.
//
// Each call has a Cmd form (PanesCmd, SendKeysCmd, …) that runs off the UI
// loop and returns a typed Msg carrying the caller's generation and the
// error, so an app never calls tmux inside Update or View, and a late result
// is dropped with an async.Gen check (engine principle E5).
//
// Client is a value; copies share only their Runner.
package tmux

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Client issues typed tmux calls through Runner. The zero Client fails every
// call with ErrNoRunner.
type Client struct {
	Runner Runner
}

// New returns a Client for the tmux server this process runs under, as env
// describes it (nil is os.Getenv). Outside tmux every call fails with
// ErrNotInTmux and nothing is executed.
func New(env func(string) string) Client {
	env = orGetenv(env)
	if !InTmux(env) {
		return Client{Runner: outside{}}
	}
	return Client{Runner: ExecRunner{SocketPath: SocketFromEnv(env)}}
}

// InTmux reports whether env (nil is os.Getenv) says this process runs inside
// tmux.
func InTmux(env func(string) string) bool { return orGetenv(env)("TMUX") != "" }

// SocketFromEnv is the server socket path from $TMUX ("path,pid,index"), or
// "" outside tmux. The path is cut at the last two commas, so a path that
// itself holds a comma survives.
func SocketFromEnv(env func(string) string) string {
	v := orGetenv(env)("TMUX")
	for range 2 {
		i := strings.LastIndexByte(v, ',')
		if i < 0 {
			return v
		}
		v = v[:i]
	}
	return v
}

// PaneFromEnv is this process's own pane ID ($TMUX_PANE), or "". Prefer it to
// Current when the app means "my pane": Current answers what the user is
// looking at, which differs once they switch windows.
func PaneFromEnv(env func(string) string) string { return orGetenv(env)("TMUX_PANE") }

func orGetenv(env func(string) string) func(string) string {
	if env == nil {
		return os.Getenv
	}
	return env
}

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	if c.Runner == nil {
		return nil, ErrNoRunner
	}
	return c.Runner.Run(ctx, args...)
}

var idPattern = regexp.MustCompile(`^[%@$][0-9]+$`)

// checkID accepts id when it is a tmux ID whose sigil is one of kinds.
func checkID(id, kinds string) error {
	if !idPattern.MatchString(id) || !strings.ContainsRune(kinds, rune(id[0])) {
		return fmt.Errorf("%w: %q (want one of %s)", ErrBadTarget, id, kinds)
	}
	return nil
}

// Pane is one row of list-panes.
type Pane struct {
	ID           string // %12
	WindowID     string // @3
	SessionID    string // $1
	SessionName  string
	WindowIndex  int
	WindowName   string
	PaneIndex    int
	PID          int // the pane's first process, usually its shell
	Cwd          string
	Command      string // pane_current_command
	Active       bool   // active pane of its window
	WindowActive bool   // active window of its session
	Title        string
	Width        int
	Height       int
	// Options holds the user options PaneFilter.Options asked for. tmux
	// formats resolve them pane → window → session → global, and report an
	// unset option as "", the same as one set empty.
	Options map[string]string
}

// PaneFilter narrows Panes.
type PaneFilter struct {
	// Target is "" for every pane on the server, a $session for that
	// session, a @window (or a %pane, meaning its window) for one window.
	Target string
	// Where is a tmux -f filter, e.g. "#{==:#{@cc_state},waiting}".
	Where string
	// Options are user options (@name) to read along with each pane.
	Options []string
}

var userOption = regexp.MustCompile(`^@[A-Za-z0-9_.-]+$`)

// Panes lists panes. A failed call returns a nil slice and the error; a
// server that answered with no rows returns an empty, non-nil slice.
func (c Client) Panes(ctx context.Context, f PaneFilter) ([]Pane, error) {
	for _, o := range f.Options {
		if !userOption.MatchString(o) {
			return nil, fmt.Errorf("tmux: %q is not a user option name", o)
		}
	}
	args := []string{"list-panes"}
	switch {
	case f.Target == "":
		args = append(args, "-a")
	case checkID(f.Target, "$") == nil:
		args = append(args, "-s", "-t", f.Target)
	default:
		if err := checkID(f.Target, "@%"); err != nil {
			return nil, err
		}
		args = append(args, "-t", f.Target)
	}
	if f.Where != "" {
		args = append(args, "-f", f.Where)
	}
	args = append(args, "-F", PaneFormat(f.Options))
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return ParsePanes(string(out), f.Options), nil
}

// PaneOptions maps each pane ID to the value of user option name, for panes
// where it is non-empty (the brd liveness read: which pane holds which
// session). The two empties mean different things and stay different: a nil
// map comes with an error (tmux told us nothing — outside tmux, server gone),
// while an empty map means tmux answered and no pane has the option.
func (c Client) PaneOptions(ctx context.Context, name string) (map[string]string, error) {
	panes, err := c.Panes(ctx, PaneFilter{Options: []string{name}})
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, p := range panes {
		if v := p.Options[name]; v != "" {
			m[p.ID] = v
		}
	}
	return m, nil
}

// Scope says which level an option lives at.
type Scope int

const (
	ScopePane    Scope = iota // -p, target %pane
	ScopeWindow               // -w, target @window or %pane
	ScopeSession              // target $session (or a pane/window in it)
	ScopeGlobal               // -g, no target
)

func (s Scope) args(target string) ([]string, error) {
	switch s {
	case ScopePane:
		return []string{"-p", "-t", target}, checkID(target, "%")
	case ScopeWindow:
		return []string{"-w", "-t", target}, checkID(target, "@%")
	case ScopeSession:
		return []string{"-t", target}, checkID(target, "$@%")
	case ScopeGlobal:
		return []string{"-g"}, nil
	}
	return nil, fmt.Errorf("tmux: unknown scope %d", s)
}

var optionName = regexp.MustCompile(`^@?[A-Za-z0-9_.-]+$`)

// Option reads one option at exactly scope (no inheritance). ok is false
// when the option is unset there, which is not the same as set to "".
func (c Client) Option(ctx context.Context, scope Scope, target, name string) (value string, ok bool, err error) {
	if !optionName.MatchString(name) {
		return "", false, fmt.Errorf("tmux: bad option name %q", name)
	}
	sargs, err := scope.args(target)
	if err != nil {
		return "", false, err
	}
	args := append(append([]string{"show-options", "-v"}, sargs...), name)
	out, err := c.run(ctx, args...)
	if err != nil {
		if isUnsetOption(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSuffix(string(out), "\n"), true, nil
}

// tmux 3.x says "invalid option: @x" for an unset user option; 2.x said
// "unknown option".
func isUnsetOption(err error) bool {
	te, ok := err.(*Error)
	return ok && (strings.Contains(te.Stderr, "invalid option") || strings.Contains(te.Stderr, "unknown option"))
}

// SetOption sets name to value at scope.
func (c Client) SetOption(ctx context.Context, scope Scope, target, name, value string) error {
	if !optionName.MatchString(name) {
		return fmt.Errorf("tmux: bad option name %q", name)
	}
	sargs, err := scope.args(target)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, append(append([]string{"set-option"}, sargs...), name, value)...)
	return err
}

// UnsetOption removes name at scope.
func (c Client) UnsetOption(ctx context.Context, scope Scope, target, name string) error {
	if !optionName.MatchString(name) {
		return fmt.Errorf("tmux: bad option name %q", name)
	}
	sargs, err := scope.args(target)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, append(append([]string{"set-option", "-u"}, sargs...), name)...)
	return err
}

// ShowEnvironment returns a session's environment (target $session, or a
// pane/window in it), or the global one when target is "". Variables tmux
// marks as removed ("-NAME") are left out.
func (c Client) ShowEnvironment(ctx context.Context, target string) (map[string]string, error) {
	args := []string{"show-environment", "-g"}
	if target != "" {
		if err := checkID(target, "$@%"); err != nil {
			return nil, err
		}
		args = []string{"show-environment", "-t", target}
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(k, "-") {
			env[k] = v
		}
	}
	return env, nil
}

// DisplayMessage expands format against target ("" is the current pane) and
// returns it without its trailing newline.
func (c Client) DisplayMessage(ctx context.Context, target, format string) (string, error) {
	args := []string{"display-message", "-p"}
	if target != "" {
		if err := checkID(target, "$@%"); err != nil {
			return "", err
		}
		args = append(args, "-t", target)
	}
	out, err := c.run(ctx, append(args, "--", format)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// Location names a session, window and pane by ID.
type Location struct{ SessionID, WindowID, PaneID string }

// Current is what the user is looking at: the current window and its active
// pane in the session this process runs in. It is not this process's own
// pane once the user switches windows; use PaneFromEnv for that.
func (c Client) Current(ctx context.Context) (Location, error) {
	s, err := c.DisplayMessage(ctx, "", "#{session_id}\t#{window_id}\t#{pane_id}")
	if err != nil {
		return Location{}, err
	}
	f := strings.Split(s, "\t")
	if len(f) != 3 {
		return Location{}, fmt.Errorf("tmux: unexpected display-message output %q", s)
	}
	return Location{SessionID: f[0], WindowID: f[1], PaneID: f[2]}, nil
}

// Run runs one raw tmux command and returns its stdout trimmed of
// surrounding whitespace. It is the escape hatch for commands without a typed
// method, and it makes Client satisfy kitty.TmuxQuerier, so one value serves
// both packages. Run bypasses the ID checks; prefer the typed calls.
func (c Client) Run(args ...string) (string, error) {
	out, err := c.run(context.Background(), args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
