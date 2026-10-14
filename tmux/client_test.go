package tmux

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var bg = context.Background()

// row builds one list-panes row the way PaneFormat asks tmux to print it:
// ten tab-separated fixed fields, then each free-text field as "\t<bytes>:<value>".
func row(fields ...string) string {
	var b strings.Builder
	b.WriteString(strings.Join(fields[:10], "\t"))
	for _, f := range fields[10:] {
		fmt.Fprintf(&b, "\t%d:%s", len(f), f)
	}
	return b.String() + "\n"
}

func TestPanesArgvPerFilter(t *testing.T) {
	cases := []struct {
		f    PaneFilter
		want []string
	}{
		{PaneFilter{}, []string{"list-panes", "-a", "-F", PaneFormat(nil)}},
		{PaneFilter{Target: "$2"}, []string{"list-panes", "-s", "-t", "$2", "-F", PaneFormat(nil)}},
		{PaneFilter{Target: "@5"}, []string{"list-panes", "-t", "@5", "-F", PaneFormat(nil)}},
		{PaneFilter{Target: "%12", Where: "#{pane_active}", Options: []string{"@cc_state"}},
			[]string{"list-panes", "-t", "%12", "-f", "#{pane_active}", "-F", PaneFormat([]string{"@cc_state"})}},
	}
	for _, c := range cases {
		f := NewFake().On("list-panes", "", nil)
		if _, err := (Client{Runner: f}).Panes(bg, c.f); err != nil {
			t.Fatal(err)
		}
		if got := f.Calls()[0]; !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v:\n got %q\nwant %q", c.f, got, c.want)
		}
	}
}

func TestPanesRejectsIndicesAndBadOptions(t *testing.T) {
	f := NewFake()
	c := Client{Runner: f}
	for _, target := range []string{"main:1", "1", "main:1.2", "%", "%1x"} {
		if _, err := c.Panes(bg, PaneFilter{Target: target}); !errors.Is(err, ErrBadTarget) {
			t.Errorf("Target %q: %v, want ErrBadTarget", target, err)
		}
	}
	for _, o := range []string{"cc_state", "@x}#{pane_id", ""} {
		if _, err := c.Panes(bg, PaneFilter{Options: []string{o}}); err == nil {
			t.Errorf("option %q accepted", o)
		}
	}
	if n := len(f.Calls()); n != 0 {
		t.Errorf("%d calls ran for rejected input", n)
	}
}

func TestParsePanesFreeTextFields(t *testing.T) {
	out := row("%12", "@3", "$1", "2", "0", "4242", "1", "0", "120", "40", "work", "editor", "/home/u/my project", "nvim", "제목\twith tab") +
		row("%13", "@3", "$1", "2", "1", "4343", "0", "0", "80", "40", "work", "editor", "/tmp/line\nbreak", "zsh", "한글 타이틀")
	got := ParsePanes(out, nil)
	want := []Pane{
		{ID: "%12", WindowID: "@3", SessionID: "$1", WindowIndex: 2, PaneIndex: 0, PID: 4242, Active: true,
			Width: 120, Height: 40, SessionName: "work", WindowName: "editor", Cwd: "/home/u/my project", Command: "nvim", Title: "제목\twith tab"},
		{ID: "%13", WindowID: "@3", SessionID: "$1", WindowIndex: 2, PaneIndex: 1, PID: 4343,
			Width: 80, Height: 40, SessionName: "work", WindowName: "editor", Cwd: "/tmp/line\nbreak", Command: "zsh", Title: "한글 타이틀"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// Tabs and newlines pass through tmux raw and occur in real data (a cwd with
// a tab, an option value with a newline); the length prefix keeps every
// field and every row in place.
func TestParsePanesTabsAndNewlinesInFields(t *testing.T) {
	out := row("%1", "@1", "$0", "0", "0", "1", "1", "1", "10", "5", "se\tss", "w\tin", "multi\nline\topt", "/tmp/a\ttab", "c\tmd", "ti\ttle") +
		row("%2", "@1", "$0", "0", "1", "2", "0", "1", "10", "5", "s", "w", "", "/", "sh", "")
	got := ParsePanes(out, []string{"@o"})
	if len(got) != 2 {
		t.Fatalf("got %d panes: %+v", len(got), got)
	}
	p := got[0]
	if p.SessionName != "se\tss" || p.WindowName != "w\tin" || p.Options["@o"] != "multi\nline\topt" ||
		p.Cwd != "/tmp/a\ttab" || p.Command != "c\tmd" || p.Title != "ti\ttle" {
		t.Errorf("pane 1 = %+v", p)
	}
	if got[1].ID != "%2" || got[1].Title != "" || got[1].Cwd != "/" {
		t.Errorf("pane 2 = %+v", got[1])
	}
}

// tmux prints control characters other than tab and newline as octal text,
// so the printed value is longer than #{n:} said. The field falls back to
// its terminator and the row still parses.
func TestParsePanesEscapedValueFallsBack(t *testing.T) {
	out := "%1\t@1\t$0\t0\t0\t1\t1\t1\t10\t5\t1:s\t1:w\t3:a\\037b\t1:/\t2:sh\t5:title\n" +
		row("%2", "@1", "$0", "0", "1", "2", "0", "1", "10", "5", "s", "w", "x", "/", "sh", "t")
	got := ParsePanes(out, []string{"@o"})
	if len(got) != 2 || got[0].Options["@o"] != "a\\037b" || got[0].Title != "title" || got[1].Options["@o"] != "x" {
		t.Fatalf("got %+v", got)
	}
}

// tmux 3.5a printed an option set to "multi\nline\tval\x1fctl" (18 bytes)
// as 21 bytes, with a raw tab inside, so cutting at the next tab would
// misplace every later field.
func TestParsePanesEscapedValueWithTab(t *testing.T) {
	raw, printed := "multi\nline\tval\x1fctl", "multi\nline\tval\\037ctl"
	out := fmt.Sprintf("%%0\t@0\t$0\t0\t0\t1\t1\t1\t50\t30\t1:p\t3:zsh\t%d:%s\t10:/tmp/a\tb c\t5:sleep\t5:title\n", len(raw), printed) +
		row("%1", "@0", "$0", "0", "1", "2", "0", "1", "49", "30", "p", "zsh", "", "/private/tmp", "bash", "t")
	got := ParsePanes(out, []string{"@cc_state"})
	if len(got) != 2 {
		t.Fatalf("got %d panes: %+v", len(got), got)
	}
	if p := got[0]; p.Options["@cc_state"] != printed || p.Cwd != "/tmp/a\tb c" || p.Command != "sleep" || p.Title != "title" {
		t.Errorf("pane 0 = %+v", p)
	}
}

func TestPaneFormat(t *testing.T) {
	want := "#{pane_id}\t#{window_id}\t#{session_id}\t#{window_index}\t#{pane_index}\t#{pane_pid}\t" +
		"#{pane_active}\t#{window_active}\t#{pane_width}\t#{pane_height}" +
		"\t#{n:session_name}:#{session_name}\t#{n:window_name}:#{window_name}" +
		"\t#{n:@cc_state}:#{@cc_state}" +
		"\t#{n:pane_current_path}:#{pane_current_path}\t#{n:pane_current_command}:#{pane_current_command}" +
		"\t#{n:pane_title}:#{pane_title}"
	if got := PaneFormat([]string{"@cc_state"}); got != want {
		t.Errorf("PaneFormat:\n got %q\nwant %q", got, want)
	}
}

func TestParsePanesSkipsBadRows(t *testing.T) {
	out := "garbage\n" +
		row("%1", "@1", "$0", "x", "0", "1", "1", "1", "10", "5", "s", "w", "/", "sh", "t") + // bad index
		row("%2", "@1", "$0", "0", "1", "2", "0", "1", "10", "5", "s", "w", "/", "sh", "t")
	got := ParsePanes(out, nil)
	if len(got) != 1 || got[0].ID != "%2" {
		t.Fatalf("got %+v", got)
	}
	if got := ParsePanes("", nil); got == nil || len(got) != 0 {
		t.Errorf("empty output: %#v, want empty non-nil", got)
	}
}

func TestPanesWithOptions(t *testing.T) {
	opts := []string{"@cc_pane_session", "@cc_pane_state"}
	f := NewFake().On("list-panes",
		row("%1", "@1", "$0", "0", "0", "10", "1", "1", "80", "24", "s", "w", "abc", "working", "/src", "claude", "t")+
			row("%2", "@1", "$0", "0", "1", "11", "0", "1", "80", "24", "s", "w", "", "", "/", "zsh", "t"), nil)
	panes, err := Client{Runner: f}.Panes(bg, PaneFilter{Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if panes[0].Options["@cc_pane_session"] != "abc" || panes[0].Options["@cc_pane_state"] != "working" || panes[0].Cwd != "/src" {
		t.Errorf("pane 0 = %+v", panes[0])
	}
	if v, ok := panes[1].Options["@cc_pane_session"]; !ok || v != "" {
		t.Errorf("pane 1 options = %#v", panes[1].Options)
	}
}

func TestPaneOptionsNilVersusEmpty(t *testing.T) {
	f := NewFake().On("list-panes", row("%1", "@1", "$0", "0", "0", "10", "1", "1", "80", "24", "s", "w", "", "/", "zsh", "t"), nil)
	m, err := Client{Runner: f}.PaneOptions(bg, "@cc_pane_session")
	if err != nil || m == nil || len(m) != 0 {
		t.Fatalf("answered, none set: %#v, %v; want empty non-nil map", m, err)
	}

	f = NewFake().On("list-panes", row("%1", "@1", "$0", "0", "0", "10", "1", "1", "80", "24", "s", "w", "sess-1", "/", "zsh", "t"), nil)
	m, _ = Client{Runner: f}.PaneOptions(bg, "@cc_pane_session")
	if !reflect.DeepEqual(m, map[string]string{"%1": "sess-1"}) {
		t.Errorf("got %#v", m)
	}

	m, err = New(env(nil)).PaneOptions(bg, "@cc_pane_session")
	if m != nil || !errors.Is(err, ErrNotInTmux) {
		t.Errorf("outside tmux: %#v, %v; want nil map and ErrNotInTmux", m, err)
	}
}

func TestOptionArgvPerScope(t *testing.T) {
	cases := []struct {
		scope  Scope
		target string
		want   string
	}{
		{ScopePane, "%3", "show-options -v -p -t %3 @cc_state"},
		{ScopeWindow, "@2", "show-options -v -w -t @2 @cc_state"},
		{ScopeWindow, "%3", "show-options -v -w -t %3 @cc_state"},
		{ScopeSession, "$1", "show-options -v -t $1 @cc_state"},
		{ScopeGlobal, "", "show-options -v -g @cc_state"},
	}
	for _, c := range cases {
		f := NewFake().On("show-options", "waiting\n", nil)
		v, ok, err := Client{Runner: f}.Option(bg, c.scope, c.target, "@cc_state")
		if err != nil || !ok || v != "waiting" {
			t.Errorf("%v: %q %v %v", c.scope, v, ok, err)
		}
		if got := f.CallLines()[0]; got != c.want {
			t.Errorf("argv %q, want %q", got, c.want)
		}
	}
}

func TestOptionUnsetVersusEmpty(t *testing.T) {
	f := NewFake().On("show-options", "", &Error{Args: []string{"show-options"}, Stderr: "invalid option: @x", Err: errors.New("exit status 1")})
	v, ok, err := Client{Runner: f}.Option(bg, ScopePane, "%1", "@x")
	if err != nil || ok || v != "" {
		t.Errorf("unset: %q %v %v; want ok=false, no error", v, ok, err)
	}
	f = NewFake().On("show-options", "\n", nil)
	v, ok, err = Client{Runner: f}.Option(bg, ScopePane, "%1", "@x")
	if err != nil || !ok || v != "" {
		t.Errorf("set empty: %q %v %v; want ok=true", v, ok, err)
	}
	boom := &Error{Args: []string{"show-options"}, Stderr: "no server running on /tmp/x", Err: errors.New("exit status 1")}
	f = NewFake().On("show-options", "", boom)
	if _, _, err = (Client{Runner: f}).Option(bg, ScopePane, "%1", "@x"); !errors.Is(err, boom) {
		t.Errorf("server error swallowed: %v", err)
	}
}

func TestOptionScopeTargetsChecked(t *testing.T) {
	c := Client{Runner: NewFake()}
	for _, tc := range []struct {
		s Scope
		t string
	}{{ScopePane, "@1"}, {ScopePane, ""}, {ScopeWindow, "$1"}, {ScopeSession, "work"}} {
		if _, _, err := c.Option(bg, tc.s, tc.t, "@x"); !errors.Is(err, ErrBadTarget) {
			t.Errorf("scope %v target %q: %v", tc.s, tc.t, err)
		}
	}
	if err := c.SetOption(bg, ScopePane, "%1", "@x y", "v"); err == nil {
		t.Error("option name with a space accepted")
	}
}

func TestSetAndUnsetOptionArgv(t *testing.T) {
	f := NewFake().On("set-option", "", nil)
	c := Client{Runner: f}
	if err := c.SetOption(bg, ScopePane, "%4", "@cc_state", "-has dash and spaces"); err != nil {
		t.Fatal(err)
	}
	if err := c.UnsetOption(bg, ScopeGlobal, "", "@cc_state"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"set-option", "-p", "-t", "%4", "@cc_state", "-has dash and spaces"},
		{"set-option", "-u", "-g", "@cc_state"},
	}
	if got := f.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestShowEnvironment(t *testing.T) {
	f := NewFake().On("show-environment", "-DISPLAY\nPATH=/bin:/usr/bin\nEMPTY=\nEQ=a=b\n", nil)
	got, err := Client{Runner: f}.ShowEnvironment(bg, "$1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"PATH": "/bin:/usr/bin", "EMPTY": "", "EQ": "a=b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
	if got := f.CallLines()[0]; got != "show-environment -t $1" {
		t.Errorf("argv %q", got)
	}
	f = NewFake().On("show-environment", "", nil)
	_, _ = Client{Runner: f}.ShowEnvironment(bg, "")
	if got := f.CallLines()[0]; got != "show-environment -g" {
		t.Errorf("global argv %q", got)
	}
}

func TestDisplayMessageAndCurrent(t *testing.T) {
	f := NewFake().On("display-message", "-starts with dash\n", nil)
	s, err := Client{Runner: f}.DisplayMessage(bg, "%2", "-#{pane_title}")
	if err != nil || s != "-starts with dash" {
		t.Fatalf("%q %v", s, err)
	}
	if got := f.Calls()[0]; !reflect.DeepEqual(got, []string{"display-message", "-p", "-t", "%2", "--", "-#{pane_title}"}) {
		t.Errorf("argv %q", got)
	}

	f = NewFake().On("display-message", "$1\t@4\t%9\n", nil)
	loc, err := Client{Runner: f}.Current(bg)
	if err != nil || loc != (Location{SessionID: "$1", WindowID: "@4", PaneID: "%9"}) {
		t.Fatalf("%+v %v", loc, err)
	}
	if got := f.Calls()[0]; len(got) != 4 || got[2] != "--" {
		t.Errorf("Current must not pass -t: %q", got)
	}

	f = NewFake().On("display-message", "garbage\n", nil)
	if _, err := (Client{Runner: f}).Current(bg); err == nil {
		t.Error("malformed Current output accepted")
	}
}
