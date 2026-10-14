package tmux

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Fake is a Runner for tests: it records every call and answers from a
// script keyed by subcommand. A call nobody scripted fails, so a test can
// never fall through to the real tmux.
//
// Fake is used by pointer and is safe for concurrent use; Cmds run on other
// goroutines.
type Fake struct {
	mu     sync.Mutex
	calls  [][]string
	script map[string][]Reply
	// Func, when set, answers every call instead of the script. It still
	// gets recorded.
	Func func(ctx context.Context, args []string) ([]byte, error)
}

// Reply is one scripted answer.
type Reply struct {
	Out string
	Err error
	// Block waits for ctx to end and returns its error, for timeout tests.
	Block bool
}

// NewFake returns an empty Fake.
func NewFake() *Fake { return &Fake{script: map[string][]Reply{}} }

// On queues a reply for subcommand sub ("list-panes"). Replies are used in
// order; the last one repeats.
func (f *Fake) On(sub, out string, err error) *Fake {
	return f.OnReply(sub, Reply{Out: out, Err: err})
}

// OnReply queues r for subcommand sub.
func (f *Fake) OnReply(sub string, r Reply) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.script == nil {
		f.script = map[string][]Reply{}
	}
	f.script[sub] = append(f.script[sub], r)
	return f
}

// Run implements Runner.
func (f *Fake) Run(ctx context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, slices.Clone(args))
	fn := f.Func
	var r Reply
	var ok bool
	if fn == nil && len(args) > 0 {
		if q := f.script[args[0]]; len(q) > 0 {
			r, ok = q[0], true
			if len(q) > 1 {
				f.script[args[0]] = q[1:]
			}
		}
	}
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, args)
	}
	if !ok {
		return nil, fmt.Errorf("tmux fake: no reply scripted for %q", strings.Join(args, " "))
	}
	if r.Block {
		<-ctx.Done()
		return nil, &Error{Args: args, Err: ctx.Err()}
	}
	return []byte(r.Out), r.Err
}

// Calls returns a copy of every call so far, in order.
func (f *Fake) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = slices.Clone(c)
	}
	return out
}

// CallLines is Calls with each call joined by single spaces, for compact
// assertions. Use Calls when an argument itself holds spaces.
func (f *Fake) CallLines() []string {
	var out []string
	for _, c := range f.Calls() {
		out = append(out, strings.Join(c, " "))
	}
	return out
}
