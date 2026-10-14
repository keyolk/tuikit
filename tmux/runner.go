package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner runs one tmux command line and returns its stdout. args never
// include the binary or the socket flags; those belong to the Runner.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// DefaultTimeout bounds a single tmux call when ExecRunner.Timeout is zero.
// tmux answers in milliseconds; a call that takes seconds is a wedged server
// (or a socket that vanished under it), and a Cmd that never returns leaves
// its caller's generation in flight forever.
const DefaultTimeout = 5 * time.Second

// ExecRunner runs the tmux binary. The zero value runs "tmux" against the
// default server (the one $TMUX names, when set) with DefaultTimeout.
type ExecRunner struct {
	Bin        string        // binary; "" is "tmux"
	Socket     string        // -L socket name
	SocketPath string        // -S socket path; wins over Socket
	Timeout    time.Duration // per call; 0 is DefaultTimeout, < 0 is none
}

// Run implements Runner. A failed call returns an *Error carrying tmux's
// stderr, because the exit status alone ("exit status 1") never says which
// pane was missing or which option was invalid.
func (r ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if t := r.timeout(); t > 0 && !IsInteractive(ctx) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t)
		defer cancel()
	}
	bin := r.Bin
	if bin == "" {
		bin = "tmux"
	}
	var argv []string
	switch {
	case r.SocketPath != "":
		argv = append(argv, "-S", r.SocketPath)
	case r.Socket != "":
		argv = append(argv, "-L", r.Socket)
	}
	argv = append(argv, args...)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// A child that inherited the pipes could keep Wait blocked after the
	// kill; WaitDelay closes them so the timeout really bounds the call.
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr // so errors.Is(err, context.DeadlineExceeded) holds
		}
		return stdout.Bytes(), &Error{Args: args, Stderr: strings.TrimSpace(stderr.String()), Err: err}
	}
	return stdout.Bytes(), nil
}

func (r ExecRunner) timeout() time.Duration {
	if r.Timeout == 0 {
		return DefaultTimeout
	}
	return r.Timeout
}

// Error is a failed tmux call.
type Error struct {
	Args   []string // tmux arguments, subcommand first
	Stderr string   // what tmux printed, trimmed
	Err    error    // the exec error or the context's error
}

func (e *Error) Error() string {
	sub := "tmux"
	if len(e.Args) > 0 {
		sub = "tmux " + e.Args[0]
	}
	if e.Stderr != "" {
		return fmt.Sprintf("%s: %v: %s", sub, e.Err, e.Stderr)
	}
	return fmt.Sprintf("%s: %v", sub, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

var (
	// ErrNotInTmux is every call's result on a Client made by New outside
	// tmux. Nothing runs.
	ErrNotInTmux = errors.New("tmux: not running inside tmux")
	// ErrNoRunner is every call's result on the zero Client. It never falls
	// back to the real tmux: a test that forgot its Fake must fail, not send
	// keys into the developer's panes.
	ErrNoRunner = errors.New("tmux: Client has no Runner")
	// ErrBadTarget rejects a target that is not a tmux ID (%pane, @window,
	// $session). Indices shift when windows move; an empty target means
	// "the current pane", which for send-keys is the app's own pane.
	ErrBadTarget = errors.New("tmux: target must be a tmux ID")
	// ErrUnmappedKey is a key TeaKeyToTmux has no tmux name for.
	ErrUnmappedKey = errors.New("tmux: key has no tmux equivalent")
)

type interactiveKey struct{}

// Interactive marks ctx as waiting on a person (a popup running an editor):
// ExecRunner then skips its timeout and only ctx itself can end the call.
func Interactive(ctx context.Context) context.Context {
	return context.WithValue(ctx, interactiveKey{}, true)
}

// IsInteractive reports whether ctx was marked by Interactive.
func IsInteractive(ctx context.Context) bool {
	v, _ := ctx.Value(interactiveKey{}).(bool)
	return v
}

// outside is the Runner New installs outside tmux.
type outside struct{}

func (outside) Run(context.Context, ...string) ([]byte, error) { return nil, ErrNotInTmux }
