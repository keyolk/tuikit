package tmux

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSelectSwitchKillArgv(t *testing.T) {
	f := NewFake().On("select-pane", "", nil).On("select-window", "", nil).
		On("switch-client", "", nil).On("kill-pane", "", nil).On("kill-window", "", nil)
	c := Client{Runner: f}
	for _, err := range []error{
		c.SelectPane(bg, "%3"),
		c.SelectWindow(bg, "@2"),
		c.SwitchClient(bg, "$1"),
		c.KillPane(bg, "%3"),
		c.KillWindow(bg, "%3"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"select-pane -t %3", "select-window -t @2", "switch-client -t $1", "kill-pane -t %3", "kill-window -t %3"}
	if got := f.CallLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestTargetsMustBeIDs(t *testing.T) {
	f := NewFake()
	c := Client{Runner: f}
	checks := map[string]error{
		"SelectPane window":  c.SelectPane(bg, "@1"),
		"SelectPane index":   c.SelectPane(bg, "main:0.1"),
		"SelectWindow index": c.SelectWindow(bg, "main:1"),
		"KillPane empty":     c.KillPane(bg, ""),
		"KillWindow session": c.KillWindow(bg, "$1"),
		"SendKeys empty":     c.SendKeys(bg, "", "Enter"),
		"SendLiteral index":  c.SendLiteral(bg, "1", "x"),
		"Jump window":        c.Jump(bg, "@1"),
		"Popup window":       c.DisplayPopup(bg, "@1", PopupOpts{}),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrBadTarget) {
			t.Errorf("%s: %v, want ErrBadTarget", name, err)
		}
	}
	_, err := c.CapturePane(bg, "", CaptureOpts{})
	if !errors.Is(err, ErrBadTarget) {
		t.Errorf("CapturePane empty: %v", err)
	}
	if _, err := c.SplitWindow(bg, "@1", SplitOpts{}); !errors.Is(err, ErrBadTarget) {
		t.Errorf("SplitWindow window: %v", err)
	}
	if _, err := c.NewWindow(bg, "main", WindowOpts{}); !errors.Is(err, ErrBadTarget) {
		t.Errorf("NewWindow name: %v", err)
	}
	if n := len(f.Calls()); n != 0 {
		t.Errorf("%d calls ran with bad targets: %q", n, f.CallLines())
	}
}

func TestJumpSameSessionSkipsSwitchClient(t *testing.T) {
	f := NewFake().
		On("display-message", "$1\t@1\t%1\n", nil). // Current
		On("display-message", "$1\n", nil).         // target's session
		On("select-window", "", nil).On("select-pane", "", nil)
	if err := (Client{Runner: f}).Jump(bg, "%7"); err != nil {
		t.Fatal(err)
	}
	got := f.CallLines()
	if got[2] != "select-window -t %7" || got[3] != "select-pane -t %7" || len(got) != 4 {
		t.Errorf("calls %q", got)
	}
}

func TestJumpOtherSessionSwitchesFirst(t *testing.T) {
	f := NewFake().
		On("display-message", "$1\t@1\t%1\n", nil).
		On("display-message", "$2\n", nil).
		On("switch-client", "", nil).On("select-window", "", nil).On("select-pane", "", nil)
	if err := (Client{Runner: f}).Jump(bg, "%7"); err != nil {
		t.Fatal(err)
	}
	want := []string{"switch-client -t %7", "select-window -t %7", "select-pane -t %7"}
	if got := f.CallLines()[2:]; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestJumpStopsAtFirstError(t *testing.T) {
	gone := &Error{Args: []string{"select-window"}, Stderr: "can't find window: %7", Err: errors.New("exit status 1")}
	f := NewFake().
		On("display-message", "$1\t@1\t%1\n", nil).
		On("display-message", "$1\n", nil).
		On("select-window", "", gone)
	if err := (Client{Runner: f}).Jump(bg, "%7"); !errors.Is(err, gone) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.Calls()); n != 3 {
		t.Errorf("select-pane ran after select-window failed: %q", f.CallLines())
	}
}

func TestSendKeysLiteralAndNames(t *testing.T) {
	f := NewFake().On("send-keys", "", nil)
	c := Client{Runner: f}
	if err := c.SendLine(bg, "%5", "-n 'it''s' $HOME; Enter"); err != nil {
		t.Fatal(err)
	}
	if err := c.SendKeys(bg, "%5", "C-c", "C-c"); err != nil {
		t.Fatal(err)
	}
	if err := c.SendLiteral(bg, "%5", ""); err != nil { // nothing to send
		t.Fatal(err)
	}
	if err := c.SendKeys(bg, "%5"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"send-keys", "-l", "-t", "%5", "--", "-n 'it''s' $HOME; Enter"},
		{"send-keys", "-t", "%5", "Enter"},
		{"send-keys", "-t", "%5", "C-c", "C-c"},
	}
	if got := f.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestSendKeyMsg(t *testing.T) {
	f := NewFake().On("send-keys", "", nil)
	c := Client{Runner: f}
	_ = c.SendKeyMsg(bg, "%5", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("한")})
	_ = c.SendKeyMsg(bg, "%5", tea.KeyMsg{Type: tea.KeyEnter})
	want := [][]string{{"send-keys", "-l", "-t", "%5", "--", "한"}, {"send-keys", "-t", "%5", "Enter"}}
	if got := f.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	err := c.SendKeyMsg(bg, "%5", tea.KeyMsg{Type: tea.KeyCtrlBackslash})
	if !errors.Is(err, ErrUnmappedKey) {
		t.Errorf("unmapped key: %v", err)
	}
}

func TestSendToPanesJoinsErrors(t *testing.T) {
	fail := errors.New("pane gone")
	f := NewFake().On("send-keys", "", nil)
	f.On("send-keys", "", nil).On("send-keys", "", fail)
	err := Client{Runner: f}.SendToPanes(bg, []string{"%1", "%2"}, "hi")
	if !errors.Is(err, fail) || !strings.Contains(err.Error(), "%2") {
		t.Errorf("err = %v, want the failure attributed to %%2", err)
	}
}

func TestCapturePaneArgv(t *testing.T) {
	f := NewFake().On("capture-pane", "line 1\nline 2\n\n\n", nil)
	got, err := Client{Runner: f}.CapturePane(bg, "%8", CaptureOpts{Start: "-200", End: "-", Escapes: true, Join: true})
	if err != nil || got != "line 1\nline 2" {
		t.Fatalf("%q %v", got, err)
	}
	want := []string{"capture-pane", "-p", "-t", "%8", "-e", "-J", "-S", "-200", "-E", "-"}
	if c := f.Calls()[0]; !reflect.DeepEqual(c, want) {
		t.Errorf("argv %q", c)
	}
}

func TestSplitWindowArgv(t *testing.T) {
	f := NewFake().On("split-window", "%21\n", nil)
	id, err := Client{Runner: f}.SplitWindow(bg, "%3", SplitOpts{
		Horizontal: true, Size: "30%", Cwd: "/home/u/my project", Env: []string{"A=b c"},
		Command: Command{Argv: []string{"narwhal", "monitor", "--task", "a b"}},
	})
	if err != nil || id != "%21" {
		t.Fatalf("%q %v", id, err)
	}
	want := []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-t", "%3", "-h", "-l", "30%",
		"-c", "/home/u/my project", "-e", "A=b c", "narwhal", "monitor", "--task", "a b"}
	if c := f.Calls()[0]; !reflect.DeepEqual(c, want) {
		t.Errorf("argv\n got %q\nwant %q", c, want)
	}
}

func TestNewWindowArgvAndPlacement(t *testing.T) {
	cases := []struct {
		target string
		o      WindowOpts
		want   []string
	}{
		{"", WindowOpts{Name: "nw:run", Command: Command{Argv: []string{"/opt/my tools/run"}}},
			[]string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-n", "nw:run", "'/opt/my tools/run'"}},
		{"$2", WindowOpts{Command: Command{Shell: "cd /x && make"}},
			[]string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-t", "$2", "cd /x && make"}},
		{"@4", WindowOpts{Cwd: "/x"},
			[]string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-a", "-t", "@4", "-c", "/x"}},
	}
	for _, c := range cases {
		f := NewFake().On("new-window", "%30\n", nil)
		id, err := Client{Runner: f}.NewWindow(bg, c.target, c.o)
		if err != nil || id != "%30" {
			t.Fatalf("%q %v", id, err)
		}
		if got := f.Calls()[0]; !reflect.DeepEqual(got, c.want) {
			t.Errorf("target %q:\n got %q\nwant %q", c.target, got, c.want)
		}
	}
}

func TestCreatedRejectsUnexpectedOutput(t *testing.T) {
	f := NewFake().On("split-window", "main:1.2\n", nil)
	if _, err := (Client{Runner: f}).SplitWindow(bg, "%1", SplitOpts{}); err == nil {
		t.Error("non-ID output accepted as a pane ID")
	}
}

func TestDisplayPopupArgvAndInteractive(t *testing.T) {
	f := NewFake()
	var interactive bool
	f.Func = func(ctx context.Context, args []string) ([]byte, error) {
		interactive = IsInteractive(ctx)
		return nil, nil
	}
	err := Client{Runner: f}.DisplayPopup(bg, "%2", PopupOpts{Width: "80%", Height: "70%", Title: "edit", Cwd: "/w d",
		Command: Command{Argv: []string{"vim", "/tmp/a b.md"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"display-popup", "-E", "-t", "%2", "-w", "80%", "-h", "70%", "-T", "edit", "-d", "/w d", "vim", "/tmp/a b.md"}
	if got := f.Calls()[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("argv %q", got)
	}
	if !interactive {
		t.Error("popup call not marked Interactive; the runner timeout would kill the editor")
	}
}
