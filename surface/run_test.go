package surface

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/keyolk/tuikit/tuitest"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func TestRunWritesEventsAsJSONL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kbd, kw := io.Pipe()
	defer kw.Close()
	var out, screen lockedBuf
	in := strings.NewReader(create + "\n" + "{oops\n")
	done := make(chan error, 1)
	go func() { done <- Run(ctx, in, &out, RunOptions{Keyboard: kbd, Screen: &screen}) }()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "VALIDATION_FAILED") {
		if time.Now().After(deadline) {
			t.Fatalf("no event written; out=%q", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := out.String(); !strings.HasPrefix(got, `{"version":"v0.9","error":`) || !strings.HasSuffix(got, "\n") {
		t.Errorf("event line: %q", got)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop on cancel")
	}
}

func TestDragSelectsInsideOnePane(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.demo()
	y, l := h.row("Author")
	_, right, _ := strings.Cut(l, "│")
	x := len([]rune(l)) - len([]rune(right)) // the cell after the divider
	h.send(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	h.send(tea.MouseMsg{X: x + 6, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	h.send(tea.MouseMsg{X: x + 6, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	var hit bool
	for _, sp := range tuitest.ReversedSpans(h.view()) {
		if sp.Row != y {
			continue // the focused list row is reverse too, on its own row
		}
		hit = strings.Contains(sp.Text, "Author")
		if sp.X0 < x {
			t.Errorf("selection reaches left of the right pane: %+v (pane starts at %d)", sp, x)
		}
	}
	if !hit {
		t.Fatalf("no highlight on %q: %+v", "Author", tuitest.ReversedSpans(h.view()))
	}
	h.cmds = nil // the copy command would reach the real clipboard
}
