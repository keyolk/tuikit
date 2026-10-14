package kitty

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// fakeTmux answers by the joined argument list; anything else fails the way
// tmux does for an unset variable.
type fakeTmux struct {
	out   map[string]string
	fail  map[string]error
	calls []string
}

func (f *fakeTmux) Run(args ...string) (string, error) {
	k := strings.Join(args, " ")
	f.calls = append(f.calls, k)
	if err := f.fail[k]; err != nil {
		return "", err
	}
	if v, ok := f.out[k]; ok {
		return v, nil
	}
	return "", errors.New("exit status 1")
}

func envOf(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

const (
	qPass     = "show-options -gv allow-passthrough"
	qProg     = "show-environment -g TERM_PROGRAM"
	qKittyWin = "show-environment -g KITTY_WINDOW_ID"
	qClient   = "display-message -p #{client_termname}"
)

func TestDetectMatrix(t *testing.T) {
	tmuxEnv := []string{"TMUX", "/tmp/tmux-1/default,1,0", "TMUX_PANE", "%3", "TERM_PROGRAM", "tmux", "TERM", "tmux-256color"}
	cases := []struct {
		name string
		env  []string
		tmux map[string]string
		want Caps
	}{
		{name: "nothing", want: Caps{}},
		{name: "kitty program", env: []string{"TERM_PROGRAM", "kitty"}, want: Caps{Supported: true, Terminal: "kitty"}},
		{name: "ghostty program", env: []string{"TERM_PROGRAM", "ghostty"}, want: Caps{Supported: true, Terminal: "ghostty"}},
		{name: "wezterm program", env: []string{"TERM_PROGRAM", "WezTerm"}, want: Caps{Supported: true, Terminal: "wezterm"}},
		{name: "xterm-kitty TERM", env: []string{"TERM", "xterm-kitty"}, want: Caps{Supported: true, Terminal: "kitty"}},
		{name: "xterm-ghostty TERM", env: []string{"TERM", "xterm-ghostty"}, want: Caps{Supported: true, Terminal: "ghostty"}},
		{name: "kitty window id", env: []string{"KITTY_WINDOW_ID", "4"}, want: Caps{Supported: true, Terminal: "kitty"}},
		{name: "iterm is not kitty", env: []string{"TERM_PROGRAM", "iTerm.app", "TERM", "xterm-256color"}, want: Caps{}},
		{name: "override off beats kitty", env: []string{OverrideEnv, "0", "TERM_PROGRAM", "kitty"}, want: Caps{}},
		{name: "override on", env: []string{OverrideEnv, "1"}, want: Caps{Supported: true}},
		{
			name: "tmux under ghostty", env: tmuxEnv,
			tmux: map[string]string{qProg: "TERM_PROGRAM=ghostty", qPass: "on"},
			want: Caps{Supported: true, Terminal: "ghostty", InTmux: true, NeedsPassthrough: true, PassthroughMode: "on", Pane: "%3"},
		},
		{
			name: "tmux kitty window id", env: tmuxEnv,
			tmux: map[string]string{qProg: "-TERM_PROGRAM", qKittyWin: "KITTY_WINDOW_ID=1", qPass: "all"},
			want: Caps{Supported: true, Terminal: "kitty", InTmux: true, NeedsPassthrough: true, PassthroughMode: "all", Pane: "%3"},
		},
		{
			name: "tmux client termname", env: tmuxEnv,
			tmux: map[string]string{qProg: "TERM_PROGRAM=Apple_Terminal", qClient: "xterm-kitty", qPass: "off"},
			want: Caps{Supported: true, Terminal: "kitty", InTmux: true, NeedsPassthrough: true, PassthroughMode: "off", Pane: "%3"},
		},
		{
			name: "tmux under plain terminal", env: tmuxEnv,
			tmux: map[string]string{qClient: "xterm-256color", qPass: "on"},
			want: Caps{InTmux: true, Pane: "%3"},
		},
		{
			name: "tmux passthrough query fails", env: tmuxEnv,
			tmux: map[string]string{qProg: "TERM_PROGRAM=WezTerm"},
			want: Caps{Supported: true, Terminal: "wezterm", InTmux: true, NeedsPassthrough: true, Pane: "%3"},
		},
		{
			name: "tmux forced on", env: append([]string{OverrideEnv, "1"}, tmuxEnv...),
			tmux: map[string]string{qPass: "on"},
			want: Caps{Supported: true, InTmux: true, NeedsPassthrough: true, PassthroughMode: "on", Pane: "%3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ft := &fakeTmux{out: tc.tmux}
			if got := Detect(envOf(tc.env...), ft); got != tc.want {
				t.Fatalf("Detect = %+v\nwant     %+v", got, tc.want)
			}
		})
	}
}

func TestDetectRunsNoTmuxOutsideTmux(t *testing.T) {
	ft := &fakeTmux{out: map[string]string{qProg: "TERM_PROGRAM=kitty"}}
	if c := Detect(envOf("TERM", "xterm-256color"), ft); c.Supported {
		t.Fatalf("outside tmux, the tmux server's env must not count: %+v", c)
	}
	if len(ft.calls) != 0 {
		t.Fatalf("tmux ran outside tmux: %v", ft.calls)
	}
}

func TestDetectSkipsPassthroughQueryWhenUnsupported(t *testing.T) {
	ft := &fakeTmux{out: map[string]string{qPass: "on"}}
	Detect(envOf("TMUX", "x"), ft)
	if slices.Contains(ft.calls, qPass) {
		t.Fatalf("queried passthrough with no graphics: %v", ft.calls)
	}
}

func TestDetectNilEnvAndTmux(t *testing.T) {
	if c := Detect(nil, nil); c != (Caps{}) {
		t.Fatalf("Detect(nil, nil) = %+v", c)
	}
	if c := Detect(envOf("TMUX", "x", "TERM_PROGRAM", "tmux"), nil); c.Supported || !c.InTmux {
		t.Fatalf("tmux without a querier = %+v", c)
	}
}

func TestEnsurePassthroughCmd(t *testing.T) {
	const set = "set-option -g allow-passthrough all"
	need := Caps{Supported: true, InTmux: true, NeedsPassthrough: true, PassthroughMode: "on"}

	for _, c := range []Caps{{}, {Supported: true}, {Supported: true, InTmux: true, NeedsPassthrough: true, PassthroughMode: "all"}} {
		if EnsurePassthroughCmd(c, &fakeTmux{}) != nil {
			t.Fatalf("nothing to do for %+v, got a command", c)
		}
	}
	if EnsurePassthroughCmd(need, nil) != nil {
		t.Fatal("no querier, no command")
	}

	ft := &fakeTmux{out: map[string]string{set: ""}}
	cmd := EnsurePassthroughCmd(need, ft)
	if len(ft.calls) != 0 {
		t.Fatal("tmux ran before the command did")
	}
	msg := cmd().(PassthroughMsg)
	if msg.Err != nil || msg.Mode != "all" || !slices.Equal(ft.calls, []string{set}) {
		t.Fatalf("msg = %+v, calls %v", msg, ft.calls)
	}
	if got := need.WithPassthrough(msg); got.PassthroughMode != "all" || need.PassthroughMode != "on" {
		t.Fatalf("WithPassthrough = %+v (original %+v)", got, need)
	}

	boom := errors.New("no server")
	msg = EnsurePassthroughCmd(need, &fakeTmux{fail: map[string]error{set: boom}})().(PassthroughMsg)
	if !errors.Is(msg.Err, boom) || msg.Mode != "on" {
		t.Fatalf("failure must be reported with the old mode: %+v", msg)
	}
}
