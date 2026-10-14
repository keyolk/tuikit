package tmux

import (
	"errors"
	"testing"

	"github.com/keyolk/tuikit/kitty"
)

// One Client serves kitty's queries too.
var _ kitty.TmuxQuerier = Client{}

func TestClientAsKittyQuerier(t *testing.T) {
	f := NewFake().On("show-options", "on\n", nil).On("show-environment", "TERM_PROGRAM=kitty\n", nil)
	var q kitty.TmuxQuerier = Client{Runner: f}
	if v, err := q.Run("show-options", "-gv", "allow-passthrough"); err != nil || v != "on" {
		t.Fatalf("%q %v", v, err)
	}
	caps := kitty.Detect(env(map[string]string{"TMUX": "/tmp/s,1,0", "TMUX_PANE": "%1"}), Client{Runner: f})
	if caps.Terminal != "kitty" {
		t.Errorf("Detect through tmux.Client: %+v (calls %q)", caps, f.CallLines())
	}
	if _, err := New(env(nil)).Run("show-options", "-gv", "x"); !errors.Is(err, ErrNotInTmux) {
		t.Errorf("outside tmux: %v", err)
	}
}
