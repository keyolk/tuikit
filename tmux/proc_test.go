package tmux

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const psOut = `    1     0 /sbin/launchd
  100     1 -zsh
  150   100 ccproxy --entrypoint claude
  200   150 claude --resume abc
  210   200 node mcp-server
  300     1 orphan
  400   401 loop-a
  401   400 loop-b
garbage line
  500
`

func TestParsePS(t *testing.T) {
	p := ParsePS([]byte(psOut))
	if len(p) != 8 {
		t.Fatalf("parsed %d rows: %+v", len(p), p)
	}
	if p[150] != (Proc{PID: 150, PPID: 100, Command: "ccproxy --entrypoint claude"}) {
		t.Errorf("row 150 = %+v", p[150])
	}
	if p := ParsePS([]byte("12 1\n")); p[12].PPID != 1 || p[12].Command != "" {
		t.Errorf("pid/ppid-only row: %+v", p)
	}
}

func TestReadProcsUsesInjectedSource(t *testing.T) {
	p, err := ReadProcs(context.Background(), func(context.Context) ([]byte, error) { return []byte(psOut), nil })
	if err != nil || len(p) != 8 {
		t.Fatalf("%d %v", len(p), err)
	}
	boom := errors.New("ps failed")
	if p, err := ReadProcs(context.Background(), func(context.Context) ([]byte, error) { return nil, boom }); p != nil || !errors.Is(err, boom) {
		t.Errorf("source error: %v %v", p, err)
	}
}

func TestWalkTo(t *testing.T) {
	p := ParsePS([]byte(psOut))
	stop := map[int]bool{100: true}
	cases := map[string]struct{ start, want int }{
		"pane shell itself":     {100, 100},
		"direct child":          {150, 100},
		"behind a wrapper":      {200, 100},
		"grandchild of wrapper": {210, 100},
		"orphan under init":     {300, 0},
		"cycle":                 {400, 0},
		"unknown pid":           {999, 0},
		"init":                  {1, 0},
	}
	for name, c := range cases {
		if got := p.WalkTo(c.start, stop); got != c.want {
			t.Errorf("%s: WalkTo(%d) = %d, want %d", name, c.start, got, c.want)
		}
	}
	if got := (Procs{}).WalkTo(5, stop); got != 0 {
		t.Errorf("empty table: %d", got)
	}
}

func TestPaneOf(t *testing.T) {
	p := ParsePS([]byte(psOut))
	panes := []Pane{{ID: "%1", PID: 100}, {ID: "%2", PID: 0}}
	if got, ok := p.PaneOf(210, panes); !ok || got.ID != "%1" {
		t.Errorf("PaneOf(210) = %+v %v", got, ok)
	}
	if _, ok := p.PaneOf(300, panes); ok {
		t.Error("orphan attributed to a pane")
	}
	if _, ok := p.PaneOf(400, panes); ok {
		t.Error("cycle attributed to a pane")
	}
}

func TestDescendants(t *testing.T) {
	p := ParsePS([]byte(psOut))
	if got := p.Descendants(100); !reflect.DeepEqual(got, []int{150, 200, 210}) {
		t.Errorf("Descendants(100) = %v", got)
	}
	if got := p.Descendants(400); !reflect.DeepEqual(got, []int{401}) {
		t.Errorf("cycle: Descendants(400) = %v", got)
	}
	if got := p.Descendants(999); len(got) != 0 {
		t.Errorf("unknown root: %v", got)
	}
	isClaude := func(pr Proc) bool { return strings.HasPrefix(pr.Command, "claude") }
	if !p.HasDescendant(100, isClaude) || p.HasDescendant(300, isClaude) {
		t.Error("HasDescendant")
	}
}
