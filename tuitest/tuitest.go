// Package tuitest runs Bubble Tea commands in tests without waiting on the
// timers inside them.
//
// A command an Update returns is often a batch: a fetch plus a toast's
// expiry tick plus a debounced save. Calling it runs the batch's members
// only when the test unpacks tea.BatchMsg by hand, and calling a tick blocks
// for its full duration (4 s for a toast). Drain does both: it unpacks
// batches and sequences, runs every command, keeps the messages of those
// that return quickly and gives up on the rest — the timers.
package tuitest

import (
	"reflect"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultWait is how long Drain lets one command run when wait is zero.
const DefaultWait = 50 * time.Millisecond

var cmdType = reflect.TypeOf(tea.Cmd(nil))

// Drain runs cmd and returns the messages it produced, batches and
// sequences flattened, in command order. A command still running after wait
// (DefaultWait when zero) is treated as a timer and skipped; it keeps
// running in the background and its message is dropped. nil commands and
// nil messages are skipped.
//
// Members of a batch run concurrently, so a batch of ten ticks costs one
// wait, not ten; members of a sequence run in order, as Bubble Tea runs
// them.
//
// Drain cannot tell a slow command from a timer: keep real I/O out of
// commands under test, or pass a wait long enough for it.
func Drain(cmd tea.Cmd, wait time.Duration) []tea.Msg {
	if wait <= 0 {
		wait = DefaultWait
	}
	return drain(cmd, wait)
}

func drain(cmd tea.Cmd, wait time.Duration) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(wait):
		return nil
	}
	if msg == nil {
		return nil
	}
	if b, ok := msg.(tea.BatchMsg); ok {
		return drainAll(b, wait)
	}
	if cmds, ok := sequence(msg); ok {
		var out []tea.Msg
		for _, c := range cmds {
			out = append(out, drain(c, wait)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// drainAll runs cmds concurrently and keeps their messages in order.
func drainAll(cmds []tea.Cmd, wait time.Duration) []tea.Msg {
	res := make([][]tea.Msg, len(cmds))
	var wg sync.WaitGroup
	for i, c := range cmds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i] = drain(c, wait)
		}()
	}
	wg.Wait()
	var out []tea.Msg
	for _, r := range res {
		out = append(out, r...)
	}
	return out
}

// sequence unpacks tea.Sequence's message. Its type is unexported, so it is
// recognized by shape: a slice of tea.Cmd that is not a BatchMsg.
func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != cmdType {
		return nil, false
	}
	out := make([]tea.Cmd, v.Len())
	for i := range out {
		out[i] = v.Index(i).Interface().(tea.Cmd) // Elem is tea.Cmd, checked above
	}
	return out, true
}
