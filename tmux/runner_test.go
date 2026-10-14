package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain makes the real tmux unreachable. The developer runs these tests
// inside tmux, where a stray send-keys would land in their own panes: TMUX
// and TMUX_PANE are cleared, and PATH holds only a decoy tmux that fails.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tuikit-tmux-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	decoy := "#!/bin/sh\necho 'tuikit tmux tests: real tmux must not be called' >&2\nexit 99\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(decoy), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Unsetenv("TMUX")
	os.Unsetenv("TMUX_PANE")
	os.Setenv("PATH", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// script writes an executable sh script standing in for the tmux binary.
func script(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "faketmux")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExecRunnerPassesSocketFlagsAndArgs(t *testing.T) {
	bin := script(t, `for a in "$@"; do printf '[%s]' "$a"; done`)
	cases := []struct {
		r    ExecRunner
		want string
	}{
		{ExecRunner{Bin: bin}, "[list-panes][-a]"},
		{ExecRunner{Bin: bin, Socket: "work"}, "[-L][work][list-panes][-a]"},
		{ExecRunner{Bin: bin, Socket: "work", SocketPath: "/tmp/s p"}, "[-S][/tmp/s p][list-panes][-a]"},
	}
	for _, c := range cases {
		out, err := c.r.Run(context.Background(), "list-panes", "-a")
		if err != nil || string(out) != c.want {
			t.Errorf("%+v: got %q, %v; want %q", c.r, out, err, c.want)
		}
	}
}

func TestExecRunnerErrorCarriesStderr(t *testing.T) {
	bin := script(t, `echo "can't find pane: %9" >&2; exit 1`)
	_, err := ExecRunner{Bin: bin}.Run(context.Background(), "send-keys", "-t", "%9", "x")
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("err = %T %v, want *Error", err, err)
	}
	if te.Stderr != "can't find pane: %9" || te.Args[0] != "send-keys" {
		t.Errorf("Error = %+v", te)
	}
	if msg := err.Error(); !strings.Contains(msg, "tmux send-keys") || !strings.Contains(msg, "can't find pane: %9") {
		t.Errorf("message %q lacks subcommand or stderr", msg)
	}
}

func TestExecRunnerTimeout(t *testing.T) {
	bin := script(t, `exec /bin/sleep 5`)
	start := time.Now()
	_, err := ExecRunner{Bin: bin, Timeout: 100 * time.Millisecond}.Run(context.Background(), "list-panes")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("timeout took %v", d)
	}
}

func TestExecRunnerInteractiveSkipsTimeout(t *testing.T) {
	bin := script(t, `/bin/sleep 0.3; echo done`)
	out, err := ExecRunner{Bin: bin, Timeout: 50 * time.Millisecond}.Run(Interactive(context.Background()), "display-popup")
	if err != nil || strings.TrimSpace(string(out)) != "done" {
		t.Fatalf("got %q, %v; an interactive call must outlive the runner timeout", out, err)
	}
}

func TestExecRunnerHonorsCallerContext(t *testing.T) {
	bin := script(t, `exec /bin/sleep 5`)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ExecRunner{Bin: bin, Timeout: -1}.Run(Interactive(ctx), "display-popup")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the caller's deadline", err)
	}
}

func TestZeroClientNeverRunsTmux(t *testing.T) {
	var c Client
	if _, err := c.Panes(context.Background(), PaneFilter{}); !errors.Is(err, ErrNoRunner) {
		t.Errorf("Panes on zero Client: %v, want ErrNoRunner", err)
	}
	if err := c.SendKeys(context.Background(), "%1", "Enter"); !errors.Is(err, ErrNoRunner) {
		t.Errorf("SendKeys on zero Client: %v, want ErrNoRunner", err)
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestNewOutsideTmux(t *testing.T) {
	c := New(env(nil))
	if _, ok := c.Runner.(outside); !ok {
		t.Fatalf("Runner = %T, want the outside-tmux runner", c.Runner)
	}
	if err := c.SendLine(context.Background(), "%1", "hi"); !errors.Is(err, ErrNotInTmux) {
		t.Errorf("err = %v, want ErrNotInTmux", err)
	}
	if InTmux(env(nil)) {
		t.Error("InTmux with empty env")
	}
}

func TestNewInsideTmuxPinsSocket(t *testing.T) {
	c := New(env(map[string]string{"TMUX": "/tmp/tmux-501/default,4242,0"}))
	r, ok := c.Runner.(ExecRunner)
	if !ok || r.SocketPath != "/tmp/tmux-501/default" {
		t.Fatalf("Runner = %#v", c.Runner)
	}
}

func TestSocketFromEnv(t *testing.T) {
	for in, want := range map[string]string{
		"":                          "",
		"/tmp/tmux-501/default,1,0": "/tmp/tmux-501/default",
		"/tmp/a,b/sock,77,3":        "/tmp/a,b/sock",
		"/tmp/nocommas":             "/tmp/nocommas",
	} {
		if got := SocketFromEnv(env(map[string]string{"TMUX": in})); got != want {
			t.Errorf("SocketFromEnv(%q) = %q, want %q", in, got, want)
		}
	}
	if got := PaneFromEnv(env(map[string]string{"TMUX_PANE": "%7"})); got != "%7" {
		t.Errorf("PaneFromEnv = %q", got)
	}
}

func TestFakeScriptOrderAndUnscripted(t *testing.T) {
	f := NewFake().On("display-message", "one\n", nil).On("display-message", "two\n", nil)
	c := Client{Runner: f}
	for _, want := range []string{"one", "two", "two"} { // the last reply repeats
		got, err := c.DisplayMessage(context.Background(), "%1", "#{pane_id}")
		if err != nil || got != want {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
	}
	if err := c.KillPane(context.Background(), "%1"); err == nil || !strings.Contains(err.Error(), "no reply scripted") {
		t.Errorf("unscripted call: %v", err)
	}
	if n := len(f.Calls()); n != 4 {
		t.Errorf("recorded %d calls, want 4", n)
	}
}

func TestFakeBlockTimesOut(t *testing.T) {
	f := NewFake().OnReply("list-panes", Reply{Block: true})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := Client{Runner: f}.Panes(ctx, PaneFilter{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}
