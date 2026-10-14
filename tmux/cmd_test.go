package tmux

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Every Cmd returns its typed Msg with the gen it was issued under, and the
// error the call returned.
func TestCmdsCarryGenAndError(t *testing.T) {
	boom := errors.New("boom")
	f := NewFake()
	for _, sub := range []string{"list-panes", "show-options", "set-option", "show-environment", "display-message",
		"capture-pane", "split-window", "new-window", "select-pane", "select-window", "switch-client",
		"send-keys", "display-popup", "kill-pane", "kill-window"} {
		f.On(sub, "", boom)
	}
	c := Client{Runner: f}
	const gen = 42
	cmds := map[string]tea.Cmd{
		"Panes":           c.PanesCmd(gen, PaneFilter{}),
		"PaneOptions":     c.PaneOptionsCmd(gen, "@x"),
		"Option":          c.OptionCmd(gen, ScopePane, "%1", "@x"),
		"SetOption":       c.SetOptionCmd(gen, ScopePane, "%1", "@x", "v"),
		"UnsetOption":     c.UnsetOptionCmd(gen, ScopePane, "%1", "@x"),
		"ShowEnvironment": c.ShowEnvironmentCmd(gen, "$1"),
		"DisplayMessage":  c.DisplayMessageCmd(gen, "%1", "#{pane_id}"),
		"Current":         c.CurrentCmd(gen),
		"CapturePane":     c.CapturePaneCmd(gen, "%1", CaptureOpts{}),
		"SplitWindow":     c.SplitWindowCmd(gen, "%1", SplitOpts{}),
		"NewWindow":       c.NewWindowCmd(gen, "", WindowOpts{}),
		"SelectPane":      c.SelectPaneCmd(gen, "%1"),
		"SelectWindow":    c.SelectWindowCmd(gen, "@1"),
		"SwitchClient":    c.SwitchClientCmd(gen, "$1"),
		"Jump":            c.JumpCmd(gen, "%1"),
		"SendKeys":        c.SendKeysCmd(gen, "%1", "Enter"),
		"SendLiteral":     c.SendLiteralCmd(gen, "%1", "x"),
		"SendLine":        c.SendLineCmd(gen, "%1", "x"),
		"SendKeyMsg":      c.SendKeyMsgCmd(gen, "%1", tea.KeyMsg{Type: tea.KeyEnter}),
		"DisplayPopup":    c.DisplayPopupCmd(gen, "", PopupOpts{}),
		"KillPane":        c.KillPaneCmd(gen, "%1"),
		"KillWindow":      c.KillWindowCmd(gen, "@1"),
	}
	if n := len(f.Calls()); n != 0 {
		t.Fatalf("building Cmds ran %d tmux calls; they must run only when the Cmd does", n)
	}
	for name, cmd := range cmds {
		g, err := genErr(cmd())
		if g != gen || !errors.Is(err, boom) {
			t.Errorf("%s: gen %d err %v; want %d and boom", name, g, err, gen)
		}
	}
}

func genErr(msg tea.Msg) (uint64, error) {
	switch m := msg.(type) {
	case PanesMsg:
		return m.Gen, m.Err
	case PaneOptionsMsg:
		return m.Gen, m.Err
	case OptionMsg:
		return m.Gen, m.Err
	case EnvMsg:
		return m.Gen, m.Err
	case DisplayMsg:
		return m.Gen, m.Err
	case LocationMsg:
		return m.Gen, m.Err
	case CaptureMsg:
		return m.Gen, m.Err
	case CreatedMsg:
		return m.Gen, m.Err
	case DoneMsg:
		return m.Gen, m.Err
	}
	return 0, nil
}

func TestCmdSuccessPayloads(t *testing.T) {
	f := NewFake().
		On("list-panes", row("%1", "@1", "$0", "0", "0", "10", "1", "1", "80", "24", "s", "w", "/", "zsh", "t"), nil).
		On("split-window", "%9\n", nil).
		On("kill-pane", "", nil)
	c := Client{Runner: f}

	pm := c.PanesCmd(7, PaneFilter{})().(PanesMsg)
	if pm.Gen != 7 || pm.Err != nil || len(pm.Panes) != 1 || pm.Panes[0].ID != "%1" {
		t.Errorf("PanesMsg %+v", pm)
	}
	cm := c.SplitWindowCmd(8, "%1", SplitOpts{})().(CreatedMsg)
	if cm != (CreatedMsg{Gen: 8, Op: OpSplitWindow, Target: "%1", PaneID: "%9"}) {
		t.Errorf("CreatedMsg %+v", cm)
	}
	dm := c.KillPaneCmd(9, "%9")().(DoneMsg)
	if dm != (DoneMsg{Gen: 9, Op: OpKillPane, Target: "%9"}) {
		t.Errorf("DoneMsg %+v", dm)
	}
}

func TestCmdsOutsideTmuxReportClearly(t *testing.T) {
	c := New(env(nil))
	for _, cmd := range []tea.Cmd{
		c.PanesCmd(3, PaneFilter{}),
		c.SendLineCmd(3, "%1", "hi"),
		c.JumpCmd(3, "%1"),
		c.DisplayPopupCmd(3, "", PopupOpts{}),
	} {
		g, err := genErr(cmd())
		if g != 3 || !errors.Is(err, ErrNotInTmux) {
			t.Errorf("gen %d err %v; want 3 and ErrNotInTmux", g, err)
		}
	}
	m := c.PaneOptionsCmd(3, "@x")().(PaneOptionsMsg)
	if m.Values != nil || !errors.Is(m.Err, ErrNotInTmux) {
		t.Errorf("PaneOptionsMsg outside tmux: %+v; want nil Values", m)
	}
}

func TestCmdBadTargetIsAnErrorMsg(t *testing.T) {
	f := NewFake()
	m := Client{Runner: f}.SendKeysCmd(1, "main:0", "Enter")().(DoneMsg)
	if !errors.Is(m.Err, ErrBadTarget) || len(f.Calls()) != 0 {
		t.Errorf("%+v, calls %q", m, f.CallLines())
	}
}
