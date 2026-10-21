package surface

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/tuitest"
)

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// harness drives a Model the way Bubble Tea would and records every event.
type harness struct {
	t      *testing.T
	m      Model
	events []Event
	logs   []string
	opened []string
	cmds   []tea.Cmd
}

func newHarness(t *testing.T, w, h int) *harness {
	t.Helper()
	hs := &harness{t: t}
	th := theme.New(theme.NewRenderer(os.Stdout))
	hs.m = New(func(e Event) error { hs.events = append(hs.events, e); return nil }, Options{
		Theme:  th,
		Logf:   func(f string, a ...any) { hs.logs = append(hs.logs, f) },
		Now:    func() time.Time { return t0 },
		Launch: func(u string) error { hs.opened = append(hs.opened, u); return nil },
	})
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	return hs
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	nm, cmd := h.m.Update(msg)
	h.m = nm.(Model)
	if cmd != nil {
		h.cmds = append(h.cmds, cmd)
	}
}

func (h *harness) lines(s ...string) {
	h.t.Helper()
	for _, l := range s {
		h.send(LineMsg(l))
	}
}

func (h *harness) file(path string) {
	h.t.Helper()
	f, err := os.Open(path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<22)
	for sc.Scan() {
		h.send(LineMsg(sc.Text()))
	}
	if err := sc.Err(); err != nil {
		h.t.Fatal(err)
	}
}

// demo loads the example up to (not including) its incremental updates.
func (h *harness) demo() {
	h.t.Helper()
	f, err := os.ReadFile("../examples/surface/pr-dashboard.jsonl")
	if err != nil {
		h.t.Fatal(err)
	}
	ls := strings.Split(strings.TrimSpace(string(f)), "\n")
	h.lines(ls[:3]...)
}

func (h *harness) key(s string) {
	h.t.Helper()
	var k tea.KeyMsg
	switch s {
	case "enter":
		k = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		k = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		k = tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		k = tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		k = tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	h.send(k)
}

// run drains the commands collected so far and feeds their messages back.
func (h *harness) run() []tea.Msg {
	h.t.Helper()
	cmds := h.cmds
	h.cmds = nil
	var all []tea.Msg
	for _, c := range cmds {
		for _, msg := range tuitest.Drain(c, 0) {
			all = append(all, msg)
			h.send(msg)
		}
	}
	return all
}

func (h *harness) view() string { return h.m.View() }

func (h *harness) plain() string { return ansi.Strip(h.view()) }

// row returns the plain line of the frame containing s, or fails.
func (h *harness) row(s string) (int, string) {
	h.t.Helper()
	for i, l := range strings.Split(h.plain(), "\n") {
		if strings.Contains(l, s) {
			return i, l
		}
	}
	h.t.Fatalf("no row contains %q in\n%s", s, h.plain())
	return -1, ""
}

func (h *harness) has(s string) bool { return strings.Contains(h.plain(), s) }

// checkWidth fails when any line of the frame is wider than w.
func checkWidth(t *testing.T, frame string, w int) {
	t.Helper()
	for i, l := range strings.Split(frame, "\n") {
		if got := render.Width(l); got > w {
			t.Errorf("line %d is %d cells wide, terminal is %d: %q", i, got, w, ansi.Strip(l))
		}
	}
}
