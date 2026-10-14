package tmux

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// The Cmd forms run each call off the UI loop. Every result Msg carries the
// gen it was issued with and the call's error, never dropped: compare Gen
// with an async.Gen before applying it, and show Err.
//
//	case tmux.PanesMsg:
//		if !m.panesGen.Is(msg.Gen) {
//			return m, nil // a newer refresh is in flight
//		}
//		if msg.Err != nil {
//			m.toasts = toast.Add(m.toasts, toast.Toast{Source: "tmux", Text: msg.Err.Error(), Level: toast.Error})
//			return m, nil
//		}
//		m.panes = msg.Panes

// Op names the call a DoneMsg or CreatedMsg reports.
type Op string

const (
	OpSelectPane   Op = "select-pane"
	OpSelectWindow Op = "select-window"
	OpSwitchClient Op = "switch-client"
	OpJump         Op = "jump"
	OpSendKeys     Op = "send-keys"
	OpSetOption    Op = "set-option"
	OpUnsetOption  Op = "unset-option"
	OpDisplayPopup Op = "display-popup"
	OpKillPane     Op = "kill-pane"
	OpKillWindow   Op = "kill-window"
	OpSplitWindow  Op = "split-window"
	OpNewWindow    Op = "new-window"
)

// PanesMsg is PanesCmd's result.
type PanesMsg struct {
	Gen   uint64
	Panes []Pane
	Err   error
}

// PaneOptionsMsg is PaneOptionsCmd's result. Values is nil exactly when Err
// is set.
type PaneOptionsMsg struct {
	Gen    uint64
	Name   string
	Values map[string]string
	Err    error
}

// OptionMsg is OptionCmd's result. Set is false when the option is unset.
type OptionMsg struct {
	Gen          uint64
	Target, Name string
	Value        string
	Set          bool
	Err          error
}

// EnvMsg is ShowEnvironmentCmd's result.
type EnvMsg struct {
	Gen    uint64
	Target string
	Env    map[string]string
	Err    error
}

// DisplayMsg is DisplayMessageCmd's result.
type DisplayMsg struct {
	Gen            uint64
	Target, Format string
	Text           string
	Err            error
}

// LocationMsg is CurrentCmd's result.
type LocationMsg struct {
	Gen uint64
	Location
	Err error
}

// CaptureMsg is CapturePaneCmd's result.
type CaptureMsg struct {
	Gen  uint64
	Pane string
	Text string
	Err  error
}

// CreatedMsg is SplitWindowCmd's and NewWindowCmd's result.
type CreatedMsg struct {
	Gen    uint64
	Op     Op
	Target string // what was split or placed after
	PaneID string // the new pane
	Err    error
}

// DoneMsg reports a call that returns nothing but its error.
type DoneMsg struct {
	Gen    uint64
	Op     Op
	Target string
	Err    error
}

// PanesCmd is Panes as a tea.Cmd.
func (c Client) PanesCmd(gen uint64, f PaneFilter) tea.Cmd {
	return func() tea.Msg {
		p, err := c.Panes(context.Background(), f)
		return PanesMsg{Gen: gen, Panes: p, Err: err}
	}
}

// PaneOptionsCmd is PaneOptions as a tea.Cmd.
func (c Client) PaneOptionsCmd(gen uint64, name string) tea.Cmd {
	return func() tea.Msg {
		v, err := c.PaneOptions(context.Background(), name)
		return PaneOptionsMsg{Gen: gen, Name: name, Values: v, Err: err}
	}
}

// OptionCmd is Option as a tea.Cmd.
func (c Client) OptionCmd(gen uint64, scope Scope, target, name string) tea.Cmd {
	return func() tea.Msg {
		v, ok, err := c.Option(context.Background(), scope, target, name)
		return OptionMsg{Gen: gen, Target: target, Name: name, Value: v, Set: ok, Err: err}
	}
}

// SetOptionCmd is SetOption as a tea.Cmd.
func (c Client) SetOptionCmd(gen uint64, scope Scope, target, name, value string) tea.Cmd {
	return c.done(gen, OpSetOption, target, func(ctx context.Context) error {
		return c.SetOption(ctx, scope, target, name, value)
	})
}

// UnsetOptionCmd is UnsetOption as a tea.Cmd.
func (c Client) UnsetOptionCmd(gen uint64, scope Scope, target, name string) tea.Cmd {
	return c.done(gen, OpUnsetOption, target, func(ctx context.Context) error {
		return c.UnsetOption(ctx, scope, target, name)
	})
}

// ShowEnvironmentCmd is ShowEnvironment as a tea.Cmd.
func (c Client) ShowEnvironmentCmd(gen uint64, target string) tea.Cmd {
	return func() tea.Msg {
		env, err := c.ShowEnvironment(context.Background(), target)
		return EnvMsg{Gen: gen, Target: target, Env: env, Err: err}
	}
}

// DisplayMessageCmd is DisplayMessage as a tea.Cmd.
func (c Client) DisplayMessageCmd(gen uint64, target, format string) tea.Cmd {
	return func() tea.Msg {
		s, err := c.DisplayMessage(context.Background(), target, format)
		return DisplayMsg{Gen: gen, Target: target, Format: format, Text: s, Err: err}
	}
}

// CurrentCmd is Current as a tea.Cmd.
func (c Client) CurrentCmd(gen uint64) tea.Cmd {
	return func() tea.Msg {
		l, err := c.Current(context.Background())
		return LocationMsg{Gen: gen, Location: l, Err: err}
	}
}

// CapturePaneCmd is CapturePane as a tea.Cmd.
func (c Client) CapturePaneCmd(gen uint64, pane string, o CaptureOpts) tea.Cmd {
	return func() tea.Msg {
		s, err := c.CapturePane(context.Background(), pane, o)
		return CaptureMsg{Gen: gen, Pane: pane, Text: s, Err: err}
	}
}

// SplitWindowCmd is SplitWindow as a tea.Cmd.
func (c Client) SplitWindowCmd(gen uint64, target string, o SplitOpts) tea.Cmd {
	return func() tea.Msg {
		id, err := c.SplitWindow(context.Background(), target, o)
		return CreatedMsg{Gen: gen, Op: OpSplitWindow, Target: target, PaneID: id, Err: err}
	}
}

// NewWindowCmd is NewWindow as a tea.Cmd.
func (c Client) NewWindowCmd(gen uint64, target string, o WindowOpts) tea.Cmd {
	return func() tea.Msg {
		id, err := c.NewWindow(context.Background(), target, o)
		return CreatedMsg{Gen: gen, Op: OpNewWindow, Target: target, PaneID: id, Err: err}
	}
}

// SelectPaneCmd is SelectPane as a tea.Cmd.
func (c Client) SelectPaneCmd(gen uint64, pane string) tea.Cmd {
	return c.done(gen, OpSelectPane, pane, func(ctx context.Context) error { return c.SelectPane(ctx, pane) })
}

// SelectWindowCmd is SelectWindow as a tea.Cmd.
func (c Client) SelectWindowCmd(gen uint64, window string) tea.Cmd {
	return c.done(gen, OpSelectWindow, window, func(ctx context.Context) error { return c.SelectWindow(ctx, window) })
}

// SwitchClientCmd is SwitchClient as a tea.Cmd.
func (c Client) SwitchClientCmd(gen uint64, target string) tea.Cmd {
	return c.done(gen, OpSwitchClient, target, func(ctx context.Context) error { return c.SwitchClient(ctx, target) })
}

// JumpCmd is Jump as a tea.Cmd.
func (c Client) JumpCmd(gen uint64, pane string) tea.Cmd {
	return c.done(gen, OpJump, pane, func(ctx context.Context) error { return c.Jump(ctx, pane) })
}

// SendKeysCmd is SendKeys as a tea.Cmd.
func (c Client) SendKeysCmd(gen uint64, pane string, keys ...string) tea.Cmd {
	return c.done(gen, OpSendKeys, pane, func(ctx context.Context) error { return c.SendKeys(ctx, pane, keys...) })
}

// SendLiteralCmd is SendLiteral as a tea.Cmd.
func (c Client) SendLiteralCmd(gen uint64, pane, text string) tea.Cmd {
	return c.done(gen, OpSendKeys, pane, func(ctx context.Context) error { return c.SendLiteral(ctx, pane, text) })
}

// SendLineCmd is SendLine as a tea.Cmd.
func (c Client) SendLineCmd(gen uint64, pane, text string) tea.Cmd {
	return c.done(gen, OpSendKeys, pane, func(ctx context.Context) error { return c.SendLine(ctx, pane, text) })
}

// SendKeyMsgCmd is SendKeyMsg as a tea.Cmd.
func (c Client) SendKeyMsgCmd(gen uint64, pane string, msg tea.KeyMsg) tea.Cmd {
	return c.done(gen, OpSendKeys, pane, func(ctx context.Context) error { return c.SendKeyMsg(ctx, pane, msg) })
}

// DisplayPopupCmd is DisplayPopup as a tea.Cmd. Its DoneMsg arrives when the
// popup closes.
func (c Client) DisplayPopupCmd(gen uint64, target string, o PopupOpts) tea.Cmd {
	return c.done(gen, OpDisplayPopup, target, func(ctx context.Context) error { return c.DisplayPopup(ctx, target, o) })
}

// KillPaneCmd is KillPane as a tea.Cmd.
func (c Client) KillPaneCmd(gen uint64, pane string) tea.Cmd {
	return c.done(gen, OpKillPane, pane, func(ctx context.Context) error { return c.KillPane(ctx, pane) })
}

// KillWindowCmd is KillWindow as a tea.Cmd.
func (c Client) KillWindowCmd(gen uint64, window string) tea.Cmd {
	return c.done(gen, OpKillWindow, window, func(ctx context.Context) error { return c.KillWindow(ctx, window) })
}

func (c Client) done(gen uint64, op Op, target string, f func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		return DoneMsg{Gen: gen, Op: op, Target: target, Err: f(context.Background())}
	}
}
