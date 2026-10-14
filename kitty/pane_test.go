package kitty

import (
	"errors"
	"slices"
	"testing"
	"time"
)

const qPane = "display-message -p -t %3 #{window_active} #{pane_top} #{pane_left}"

func TestQueryPane(t *testing.T) {
	ft := &fakeTmux{out: map[string]string{qPane: "0 5 10"}}
	p, err := QueryPane(ft, "%3")
	if err != nil || p != (Pane{Visible: false, Top: 5, Left: 10}) {
		t.Fatalf("got %+v %v", p, err)
	}
	// Targeted by id, never the client's current pane (ccx).
	if !slices.Equal(ft.calls, []string{qPane}) {
		t.Fatalf("calls %v", ft.calls)
	}

	p, err = QueryPane(&fakeTmux{out: map[string]string{qPane: "1 0 0"}}, "%3")
	if err != nil || !p.Visible {
		t.Fatalf("got %+v %v", p, err)
	}

	boom := errors.New("server exited")
	p, err = QueryPane(&fakeTmux{fail: map[string]error{qPane: boom}}, "%3")
	if !errors.Is(err, boom) || !p.Visible {
		t.Fatalf("failure: visible with the error, got %+v %v", p, err)
	}
	if p, err = QueryPane(&fakeTmux{out: map[string]string{qPane: "junk"}}, "%3"); err == nil || !p.Visible {
		t.Fatalf("unparsable reply: %+v %v", p, err)
	}
	if p, err = QueryPane(nil, "%3"); err != nil || !p.Visible {
		t.Fatalf("outside tmux: %+v %v", p, err)
	}
	if msg := PaneCmd(&fakeTmux{out: map[string]string{qPane: "0 1 2"}}, "%3")().(PaneMsg); msg.Pane != (Pane{Top: 1, Left: 2}) {
		t.Fatalf("PaneCmd = %+v", msg)
	}
}

// Ported from ccx#175: silent while nothing changes, one message on a change.
func TestWatchPaneEmitsOnlyOnTransition(t *testing.T) {
	seq := []Pane{{Visible: true}, {Visible: true}, {Visible: true}, {Visible: false}}
	calls := 0
	probe := func() (Pane, error) {
		p := seq[min(calls, len(seq)-1)]
		calls++
		return p, nil
	}
	msg := WatchPane(PaneMsg{Pane: Pane{Visible: true}}, probe, time.Millisecond)().(PaneMsg)
	if msg.Pane.Visible || msg.Err != nil {
		t.Fatalf("want the hidden transition, got %+v", msg)
	}
	if calls != 4 {
		t.Fatalf("probe ran %d times, want 4: unchanged polls must not return", calls)
	}
}

func TestWatchPaneReportsMovesAndErrorEdges(t *testing.T) {
	last := PaneMsg{Pane: Pane{Visible: true}}
	moved := WatchPane(last, func() (Pane, error) { return Pane{Visible: true, Top: 3}, nil }, time.Millisecond)().(PaneMsg)
	if moved.Pane.Top != 3 {
		t.Fatalf("a moved pane is a change: %+v", moved)
	}

	boom := errors.New("tmux gone")
	failing := func() (Pane, error) { return Pane{Visible: true}, boom }
	msg := WatchPane(last, failing, time.Millisecond)().(PaneMsg)
	if !errors.Is(msg.Err, boom) {
		t.Fatalf("the first failure must be reported: %+v", msg)
	}
	// Still failing: no second report (no flood), until it recovers.
	n := 0
	probe := func() (Pane, error) {
		n++
		if n < 5 {
			return Pane{Visible: true}, boom
		}
		return Pane{Visible: true}, nil
	}
	msg = WatchPane(msg, probe, time.Millisecond)().(PaneMsg)
	if msg.Err != nil || n != 5 {
		t.Fatalf("recovery after %d polls: %+v", n, msg)
	}
}
