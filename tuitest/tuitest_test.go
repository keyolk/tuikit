package tuitest

import (
	"slices"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keyolk/tuikit/toast"
)

type msg string

func send(s string) tea.Cmd { return func() tea.Msg { return msg(s) } }

func TestDrainFlattensAndSkipsTimers(t *testing.T) {
	_, toastTick := toast.Push(toast.Queue{}, toast.Toast{Text: "saved"}) // a 4 s tick
	cmd := tea.Batch(
		send("a"),
		tea.Tick(time.Hour, func(time.Time) tea.Msg { return msg("timer") }),
		toastTick,
		nil,
		tea.Batch(send("b"), send("c")),
		tea.Sequence(send("d"), func() tea.Msg { return nil }, send("e")),
	)
	start := time.Now()
	got := Drain(cmd, 0)
	if el := time.Since(start); el > time.Second {
		t.Fatalf("Drain waited %v on timers", el)
	}
	want := []tea.Msg{msg("a"), msg("b"), msg("c"), msg("d"), msg("e")}
	if !slices.Equal(got, want) {
		t.Fatalf("Drain = %v, want %v", got, want)
	}
}

func TestDrainSingleAndNil(t *testing.T) {
	if got := Drain(send("x"), time.Second); !slices.Equal(got, []tea.Msg{msg("x")}) {
		t.Fatalf("single = %v", got)
	}
	if Drain(nil, 0) != nil {
		t.Fatal("nil cmd drains nothing")
	}
	// A short timer within the wait is kept.
	quick := tea.Tick(time.Millisecond, func(time.Time) tea.Msg { return msg("quick") })
	if got := Drain(quick, time.Second); !slices.Equal(got, []tea.Msg{msg("quick")}) {
		t.Fatalf("quick timer = %v", got)
	}
}

func TestReversed(t *testing.T) {
	frame := "ab\x1b[7mcd\x1b[0mef\x1b[38;5;7mgh\x1b[1;7mi\x1b]8;;https://x.io\x1b\\j\x1b]8;;\x1b\\\nk\x1b[27ml\x1b[7m\x1b[m"
	got := Reversed(frame)
	want := []string{"cd", "ij", "k"}
	if !slices.Equal(got, want) {
		t.Fatalf("Reversed = %q, want %q", got, want)
	}
}
