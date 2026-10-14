package prefs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The zero Saver must never write $XDG_STATE_HOME/ui.json (brd, gcl).
func TestZeroSaverIsANoOp(t *testing.T) {
	isolate(t)
	for name, s := range map[string]Saver[st]{"zero": {}, "app \"\"": NewSaver[st]("", 0)} {
		s2, cmd := s.Save(st{N: 1})
		if cmd != nil || s2.Pending() {
			t.Fatalf("%s: Save scheduled work", name)
		}
		if _, cmd := s2.Update(SaveTick{}); cmd != nil {
			t.Fatalf("%s: Update wrote", name)
		}
		_, cmd = s2.FlushWith(st{N: 2})
		if got := run(t, cmd); !got.Final {
			t.Fatalf("%s: FlushWith must still report Final so the app quits", name)
		}
		_, cmd = s2.Flush()
		if got := run(t, cmd); !got.Final {
			t.Fatalf("%s: Flush must still report Final", name)
		}
	}
	if SaveCmd("", st{N: 1}) != nil {
		t.Fatal(`SaveCmd("") must be nil`)
	}
	if v, err := Load("", st{N: 7}); err != nil || v.N != 7 {
		t.Fatalf(`Load("") = %v, %v`, v, err)
	}
	entries, _ := os.ReadDir(os.Getenv("XDG_STATE_HOME"))
	if len(entries) != 0 {
		t.Fatalf("state dir not empty: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "ui.json")); !os.IsNotExist(err) {
		t.Fatal("ui.json was written at the state root")
	}
}

func TestFlushWithWritesTheGivenState(t *testing.T) {
	isolate(t)
	s := NewSaver[st]("app", time.Hour)
	s, _ = s.Save(st{N: 1})
	tick := s.TickMsg()
	s, cmd := s.FlushWith(st{N: 9})
	if got := run(t, cmd); !got.Final {
		t.Fatal("FlushWith is Final")
	}
	if s.Pending() || readN(t, "app") != 9 {
		t.Fatalf("FlushWith wrote %d", readN(t, "app"))
	}
	if _, cmd := s.Update(tick); cmd != nil {
		t.Fatal("the debounce tick from before FlushWith must not write")
	}
}

func TestFlushWithEncodeErrorStillQuits(t *testing.T) {
	isolate(t)
	s := NewSaver[any]("app", time.Hour)
	s, _ = s.Save(st{N: 3})
	_, cmd := s.FlushWith(func() {})
	msg := cmd().(Saved)
	if msg.Err == nil || !msg.Final {
		t.Fatalf("FlushWith(func) = %+v; want Err and Final", msg)
	}
}

func TestFlushWithCopiesAreIndependent(t *testing.T) {
	isolate(t)
	a := NewSaver[st]("app", time.Hour)
	a, _ = a.Save(st{N: 1})
	b, _ := a.FlushWith(st{N: 2})
	if string(a.data) == string(b.data) || !a.Pending() || b.Pending() {
		t.Fatal("FlushWith on a copy changed the original")
	}
}
