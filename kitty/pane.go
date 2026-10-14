package kitty

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Pane is where our tmux pane sits and whether its window is on screen.
type Pane struct {
	Visible   bool
	Top, Left int
}

// PaneMsg reports a pane query or a WatchPane transition.
type PaneMsg struct {
	Pane Pane
	Err  error
}

// QueryPane asks tmux about pane (Caps.Pane). It blocks: call it from a
// command. Outside tmux it reports a visible pane at 0,0.
//
// It targets the pane by id. Without -t, tmux answers for the client's
// current pane, which right after a window switch is in the new window and
// reports window_active=1 although we are hidden (ccx).
//
// On error the pane counts as visible, so a transient failure during a
// window switch does not delete images; the error is still returned.
func QueryPane(tmux TmuxQuerier, pane string) (Pane, error) {
	if tmux == nil || pane == "" {
		return Pane{Visible: true}, nil
	}
	out, err := tmux.Run("display-message", "-p", "-t", pane, "#{window_active} #{pane_top} #{pane_left}")
	if err != nil {
		return Pane{Visible: true}, err
	}
	var active string
	var p Pane
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%s %d %d", &active, &p.Top, &p.Left); err != nil {
		return Pane{Visible: true}, fmt.Errorf("kitty: parse pane %q: %w", out, err)
	}
	p.Visible = active == "1"
	return p, nil
}

// PaneCmd runs QueryPane off the UI loop.
func PaneCmd(tmux TmuxQuerier, pane string) tea.Cmd {
	return func() tea.Msg {
		p, err := QueryPane(tmux, pane)
		return PaneMsg{Pane: p, Err: err}
	}
}

// WatchPane polls probe every interval and returns only when the answer
// differs from last: the pane moved, was hidden or shown, or the query
// started or stopped failing. tmux sends no focus-out on a window switch, so
// polling is the only signal; doing it here, not on a tick through Update,
// means an idle app gets no messages at all (ccx#175). The app re-arms it
// with the message it got.
//
//	case kitty.PaneMsg:
//		if !msg.Pane.Visible && m.pane.Pane.Visible {
//			cmd = m.out.WriteCmd(m.caps.DeleteAll()) // a cursor-placed image would linger
//		}
//		m.pane = msg
//		return m, tea.Batch(cmd, kitty.WatchPane(msg, probe, 250*time.Millisecond))
func WatchPane(last PaneMsg, probe func() (Pane, error), every time.Duration) tea.Cmd {
	return func() tea.Msg {
		for {
			time.Sleep(every)
			p, err := probe()
			if p != last.Pane || (err == nil) != (last.Err == nil) {
				return PaneMsg{Pane: p, Err: err}
			}
		}
	}
}
