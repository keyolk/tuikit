package prefs

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type st struct {
	N int `json:"n"`
}

func readN(t *testing.T, app string) int {
	t.Helper()
	data, err := os.ReadFile(Path(app))
	if err != nil {
		t.Fatal(err)
	}
	var v st
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v.N
}

func run(t *testing.T, cmd tea.Cmd) Saved {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	msg, ok := cmd().(Saved)
	if !ok {
		t.Fatalf("cmd returned %T, want Saved", msg)
	}
	if msg.Err != nil {
		t.Fatal(msg.Err)
	}
	return msg
}

func TestSaverDebouncesToTheLatestTick(t *testing.T) {
	isolate(t)
	s := NewSaver[st]("app", time.Hour)
	var ticks []SaveTick
	for i := 1; i <= 3; i++ {
		s, _ = s.Save(st{N: i})
		ticks = append(ticks, s.TickMsg())
	}
	if !s.Pending() {
		t.Fatal("a saved change must be pending until its tick")
	}
	for _, old := range ticks[:2] {
		if _, cmd := s.Update(old); cmd != nil {
			t.Fatal("a superseded tick must not write")
		}
	}
	if _, cmd := s.Update(SaveTick{app: "other", gen: s.gen}); cmd != nil {
		t.Fatal("another app's tick must not write")
	}
	s, cmd := s.Update(ticks[2])
	if got := run(t, cmd); got.Final || got.App != "app" {
		t.Fatalf("saved = %+v", got)
	}
	if s.Pending() || readN(t, "app") != 3 {
		t.Fatalf("pending=%v n=%d; want the latest state written", s.Pending(), readN(t, "app"))
	}
	if _, cmd := s.Update(ticks[2]); cmd != nil {
		t.Fatal("a tick after its write must not write again")
	}
}

func TestSaverRealTickFires(t *testing.T) {
	isolate(t)
	s, cmd := NewSaver[st]("app", time.Millisecond).Save(st{N: 7})
	s, cmd = s.Update(cmd())
	run(t, cmd)
	if readN(t, "app") != 7 {
		t.Fatal("tick did not write")
	}
}

func TestOlderWriteNeverLandsAfterNewer(t *testing.T) {
	isolate(t)
	s := NewSaver[st]("app", -1) // write on every Save
	s, older := s.Save(st{N: 1})
	_, newer := s.Save(st{N: 2})
	run(t, newer)
	run(t, older) // runs late, as goroutines may
	if n := readN(t, "app"); n != 2 {
		t.Fatalf("n = %d; the older write landed over the newer one", n)
	}
	// SaveCmd takes part in the same ordering.
	stale := SaveCmd("app", st{N: 3})
	run(t, SaveCmd("app", st{N: 4}))
	run(t, stale)
	if n := readN(t, "app"); n != 4 {
		t.Fatalf("n = %d after out-of-order SaveCmds", n)
	}
}

func TestConcurrentWritesKeepTheNewest(t *testing.T) {
	isolate(t)
	s := NewSaver[st]("app", -1)
	var cmds []tea.Cmd
	for i := 1; i <= 50; i++ {
		var c tea.Cmd
		s, c = s.Save(st{N: i})
		cmds = append(cmds, c)
	}
	var wg sync.WaitGroup
	for i := len(cmds) - 1; i >= 0; i-- {
		wg.Add(1)
		go func(c tea.Cmd) { defer wg.Done(); c() }(cmds[i])
	}
	wg.Wait()
	if n := readN(t, "app"); n != 50 {
		t.Fatalf("n = %d; want the newest of 50 concurrent writes", n)
	}
}

func TestFlush(t *testing.T) {
	isolate(t)
	s := NewSaver[st]("app", time.Hour)

	_, cmd := s.Flush()
	if got := run(t, cmd); !got.Final {
		t.Fatal("Flush with nothing saved must still report Final")
	}
	if _, err := os.Stat(Path("app")); !os.IsNotExist(err) {
		t.Fatal("Flush with nothing saved wrote a file")
	}

	s, _ = s.Save(st{N: 5})
	tick := s.TickMsg()
	s, cmd = s.Flush()
	if got := run(t, cmd); !got.Final {
		t.Fatal("flush write is Final")
	}
	if s.Pending() || readN(t, "app") != 5 {
		t.Fatal("Flush did not write the pending state")
	}
	if _, cmd := s.Update(tick); cmd != nil {
		t.Fatal("a debounce tick from before Flush must not write")
	}
	// Nothing pending: Flush rewrites the latest so an in-flight write
	// cannot be the one the app quits on.
	if _, cmd := s.Flush(); !run(t, cmd).Final || readN(t, "app") != 5 {
		t.Fatal("idle Flush must rewrite the latest state")
	}
}

func TestSaverEncodeErrorKeepsState(t *testing.T) {
	isolate(t)
	s := NewSaver[any]("app", time.Hour)
	s, _ = s.Save(st{N: 1})
	s2, cmd := s.Save(func() {})
	if msg := cmd().(Saved); msg.Err == nil {
		t.Fatal("encoding a func must fail")
	}
	if string(s2.data) != string(s.data) || !s2.Pending() {
		t.Fatal("a failed encode must keep the previous state")
	}
}

func TestSaverCopiesAreIndependent(t *testing.T) {
	isolate(t)
	a, _ := NewSaver[st]("app", time.Hour).Save(st{N: 1})
	b, _ := a.Save(st{N: 2})
	if string(a.data) == string(b.data) || a.gen == b.gen {
		t.Fatal("Save on a copy changed the original")
	}
	a, cmd := a.Update(a.TickMsg())
	run(t, cmd)
	if !b.Pending() || a.Pending() {
		t.Fatal("Update on one copy changed the other's pending state")
	}
}
