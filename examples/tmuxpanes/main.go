// Command tmuxpanes lists the panes of the tmux server it runs under and
// jumps to the one under the cursor. It shows the tmux package's contract:
// every tmux call is a Cmd, every result carries a generation, and errors
// reach the screen. Outside tmux it says so and exits.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/keyolk/tuikit/async"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/tmux"
)

type model struct {
	tm     tmux.Client
	gen    async.Gen // the newest pane listing; older ones are dropped
	panes  []tmux.Pane
	cursor int
	status string
	w, h   int
}

func newModel(tm tmux.Client) model { return model{tm: tm} }

// refresh starts a listing tagged with a new generation.
func (m model) refresh() (model, tea.Cmd) {
	var id uint64
	m.gen, id = m.gen.Next()
	return m, m.tm.PanesCmd(id, tmux.PaneFilter{Options: []string{"@cc_state"}})
}

func (m model) Init() tea.Cmd {
	_, cmd := m.refresh()
	return cmd
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tmux.PanesMsg:
		if !m.gen.Is(msg.Gen) {
			return m, nil // superseded by a later refresh
		}
		if msg.Err != nil {
			m.status = msg.Err.Error()
			return m, nil
		}
		m.panes = msg.Panes
		m.cursor = min(m.cursor, max(0, len(m.panes)-1))
	case tmux.DoneMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("%s %s: %v", msg.Op, msg.Target, msg.Err)
		} else {
			m.status = fmt.Sprintf("%s %s", msg.Op, msg.Target)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.cursor = min(m.cursor+1, max(0, len(m.panes)-1))
		case "k", "up":
			m.cursor = max(m.cursor-1, 0)
		case "r":
			return m.refresh()
		case "enter":
			if m.cursor < len(m.panes) {
				// Jump by pane ID; the result comes back as a DoneMsg.
				return m, m.tm.JumpCmd(0, m.panes[m.cursor].ID)
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	for i, p := range m.panes {
		mark := "  "
		if i == m.cursor {
			mark = "> "
		}
		line := fmt.Sprintf("%s%-5s %-12s %-10s %-8s %s", mark, p.ID, p.SessionName+":"+p.WindowName,
			p.Command, p.Options["@cc_state"], p.Cwd)
		b.WriteString(render.Truncate(line, max(m.w, 20)) + "\n")
	}
	b.WriteString("\nj/k move  enter jump  r refresh  q quit")
	if m.status != "" {
		b.WriteString("\n" + m.status)
	}
	return b.String()
}

func main() {
	if !tmux.InTmux(nil) {
		fmt.Fprintln(os.Stderr, "tmuxpanes: run me inside tmux")
		os.Exit(1)
	}
	if _, err := tea.NewProgram(newModel(tmux.New(nil))).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
