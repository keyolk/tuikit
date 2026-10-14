package tmux

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// SelectPane makes pane the active pane of its window.
func (c Client) SelectPane(ctx context.Context, pane string) error {
	if err := checkID(pane, "%"); err != nil {
		return err
	}
	_, err := c.run(ctx, "select-pane", "-t", pane)
	return err
}

// SelectWindow makes window (@window, or the window of a %pane) current in
// its session.
func (c Client) SelectWindow(ctx context.Context, window string) error {
	if err := checkID(window, "@%"); err != nil {
		return err
	}
	_, err := c.run(ctx, "select-window", "-t", window)
	return err
}

// SwitchClient moves the current client to the session holding target.
func (c Client) SwitchClient(ctx context.Context, target string) error {
	if err := checkID(target, "$@%"); err != nil {
		return err
	}
	_, err := c.run(ctx, "switch-client", "-t", target)
	return err
}

// Jump brings pane in front of the user: switch the client when the pane is
// in another session, then select its window, then the pane. select-pane
// alone leaves the user looking at the right pane in a window they cannot
// see (brd); switch-client for the session already shown would overwrite
// the user's "last session" for no reason (ccx), so it is skipped then.
func (c Client) Jump(ctx context.Context, pane string) error {
	if err := checkID(pane, "%"); err != nil {
		return err
	}
	cur, err := c.Current(ctx)
	if err != nil {
		return err
	}
	sess, err := c.DisplayMessage(ctx, pane, "#{session_id}")
	if err != nil {
		return err
	}
	if sess != cur.SessionID {
		if err := c.SwitchClient(ctx, pane); err != nil {
			return err
		}
	}
	if err := c.SelectWindow(ctx, pane); err != nil {
		return err
	}
	return c.SelectPane(ctx, pane)
}

// SendKeys sends key names ("Enter", "C-c", "M-x") to pane. Text sent this
// way is parsed for key names; use SendLiteral for text.
func (c Client) SendKeys(ctx context.Context, pane string, keys ...string) error {
	if err := checkID(pane, "%"); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	_, err := c.run(ctx, append([]string{"send-keys", "-t", pane}, keys...)...)
	return err
}

// SendLiteral types text into pane exactly as given (-l), with "--" so text
// that starts with "-" is not read as a flag. Nothing is sent for "".
func (c Client) SendLiteral(ctx context.Context, pane, text string) error {
	if err := checkID(pane, "%"); err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	_, err := c.run(ctx, "send-keys", "-l", "-t", pane, "--", text)
	return err
}

// SendLine types text literally and then presses Enter, as two calls: Enter
// inside the -l call would type the word "Enter", and the text without -l
// would have its words read as key names.
func (c Client) SendLine(ctx context.Context, pane, text string) error {
	if err := c.SendLiteral(ctx, pane, text); err != nil {
		return err
	}
	return c.SendKeys(ctx, pane, "Enter")
}

// SendKeyMsg forwards one Bubble Tea key press to pane, mapped by
// TeaKeyToTmux. A key with no tmux equivalent is ErrUnmappedKey, not a
// silent no-op.
func (c Client) SendKeyMsg(ctx context.Context, pane string, msg tea.KeyMsg) error {
	k, ok := TeaKeyToTmux(msg)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnmappedKey, msg.String())
	}
	if k.Literal {
		return c.SendLiteral(ctx, pane, k.Text)
	}
	return c.SendKeys(ctx, pane, k.Text)
}

// CaptureOpts shapes CapturePane.
type CaptureOpts struct {
	// Start and End are capture-pane -S/-E: a line number (0 is the first
	// visible line, negative reaches into history) or "-" for the start of
	// history / end of the visible pane. "" leaves tmux's default (the
	// visible pane).
	Start, End string
	Escapes    bool // -e: keep SGR escapes
	Join       bool // -J: join wrapped lines
}

// CapturePane returns pane's contents with trailing newlines trimmed.
func (c Client) CapturePane(ctx context.Context, pane string, o CaptureOpts) (string, error) {
	if err := checkID(pane, "%"); err != nil {
		return "", err
	}
	args := []string{"capture-pane", "-p", "-t", pane}
	if o.Escapes {
		args = append(args, "-e")
	}
	if o.Join {
		args = append(args, "-J")
	}
	if o.Start != "" {
		args = append(args, "-S", o.Start)
	}
	if o.End != "" {
		args = append(args, "-E", o.End)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// Command is what a new pane, window or popup runs. The zero Command is the
// default shell.
type Command struct {
	// Argv runs exactly these words. tmux runs a single word through
	// "sh -c", so a one-word Argv is shell-quoted first.
	Argv []string
	// Shell is a shell command line, run by "sh -c" as is. Ignored when
	// Argv is set.
	Shell string
}

func (c Command) args() []string {
	switch {
	case len(c.Argv) == 1:
		return []string{ShellQuote(c.Argv[0])}
	case len(c.Argv) > 1:
		return c.Argv
	case c.Shell != "":
		return []string{c.Shell}
	}
	return nil
}

// SplitOpts shapes SplitWindow.
type SplitOpts struct {
	Horizontal bool   // -h: side by side; otherwise stacked (-v)
	Before     bool   // -b: new pane left of / above target
	Size       string // -l: "30%" or a line/cell count
	Cwd        string // -c
	Env        []string
	Command    Command
}

func cwdEnv(cwd string, env []string) []string {
	var a []string
	if cwd != "" {
		a = append(a, "-c", cwd)
	}
	for _, e := range env {
		a = append(a, "-e", e)
	}
	return a
}

// SplitWindow splits target (a %pane) without moving focus and returns the
// new pane's ID.
func (c Client) SplitWindow(ctx context.Context, target string, o SplitOpts) (string, error) {
	if err := checkID(target, "%"); err != nil {
		return "", err
	}
	args := []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-t", target}
	if o.Horizontal {
		args = append(args, "-h")
	} else {
		args = append(args, "-v")
	}
	if o.Before {
		args = append(args, "-b")
	}
	if o.Size != "" {
		args = append(args, "-l", o.Size)
	}
	args = append(args, cwdEnv(o.Cwd, o.Env)...)
	return c.created(ctx, append(args, o.Command.args()...))
}

// WindowOpts shapes NewWindow.
type WindowOpts struct {
	Name    string // -n
	Cwd     string
	Env     []string
	Command Command
}

// NewWindow opens a window without switching to it and returns its pane's
// ID. target "" is the current session; a $session puts it there; a @window
// or %pane inserts it right after that window (-a), since new-window -t
// @window alone means "at that index" and fails because it is taken.
func (c Client) NewWindow(ctx context.Context, target string, o WindowOpts) (string, error) {
	args := []string{"new-window", "-d", "-P", "-F", "#{pane_id}"}
	switch {
	case target == "":
	case checkID(target, "$") == nil:
		args = append(args, "-t", target)
	default:
		if err := checkID(target, "@%"); err != nil {
			return "", err
		}
		args = append(args, "-a", "-t", target)
	}
	if o.Name != "" {
		args = append(args, "-n", o.Name)
	}
	args = append(args, cwdEnv(o.Cwd, o.Env)...)
	return c.created(ctx, append(args, o.Command.args()...))
}

func (c Client) created(ctx context.Context, args []string) (string, error) {
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(out))
	if checkID(id, "%") != nil {
		return "", fmt.Errorf("tmux %s: unexpected output %q", args[0], id)
	}
	return id, nil
}

// PopupOpts shapes DisplayPopup.
type PopupOpts struct {
	Width, Height string // -w/-h: "80%" or cells
	Title         string // -T
	Cwd           string // -d
	KeepOpen      bool   // drop -E: the popup stays after the command exits
	Command       Command
}

// DisplayPopup opens a popup over target's client (target "" is the current
// one) and returns when it closes. It waits on a person, so the runner's
// timeout does not apply; ctx alone bounds it.
func (c Client) DisplayPopup(ctx context.Context, target string, o PopupOpts) error {
	args := []string{"display-popup"}
	if !o.KeepOpen {
		args = append(args, "-E")
	}
	if target != "" {
		if err := checkID(target, "%"); err != nil {
			return err
		}
		args = append(args, "-t", target)
	}
	for _, f := range [][2]string{{"-w", o.Width}, {"-h", o.Height}, {"-T", o.Title}, {"-d", o.Cwd}} {
		if f[1] != "" {
			args = append(args, f[0], f[1])
		}
	}
	_, err := c.run(Interactive(ctx), append(args, o.Command.args()...)...)
	return err
}

// KillPane kills pane.
func (c Client) KillPane(ctx context.Context, pane string) error {
	if err := checkID(pane, "%"); err != nil {
		return err
	}
	_, err := c.run(ctx, "kill-pane", "-t", pane)
	return err
}

// KillWindow kills window (@window, or the window holding a %pane).
func (c Client) KillWindow(ctx context.Context, window string) error {
	if err := checkID(window, "@%"); err != nil {
		return err
	}
	_, err := c.run(ctx, "kill-window", "-t", window)
	return err
}

// SendToPanes sends text as a line to each pane and joins the errors, so a
// partial failure names every pane that missed it rather than the last one.
func (c Client) SendToPanes(ctx context.Context, panes []string, text string) error {
	var errs []error
	for _, p := range panes {
		if err := c.SendLine(ctx, p, text); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
		}
	}
	return errors.Join(errs...)
}
