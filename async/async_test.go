package async

import "testing"

func TestGenNextAndIs(t *testing.T) {
	var g Gen
	if g.Is(0) || g.Current() != 0 {
		t.Fatal("zero Gen must accept nothing")
	}
	g, a := g.Next()
	if !g.Is(a) || a != 1 || g.Current() != 1 {
		t.Fatalf("after Next: a=%d current=%d", a, g.Current())
	}
	g, b := g.Next()
	if g.Is(a) || !g.Is(b) {
		t.Fatal("an older generation must be stale")
	}
}

func TestGenCopiesAreIndependent(t *testing.T) {
	var g Gen
	g, id := g.Next()
	cp := g
	cp, _ = cp.Next()
	if !g.Is(id) || cp.Is(id) {
		t.Fatal("advancing a copy changed the original")
	}
	// Next has a value receiver: discarding its result changes nothing.
	g.Next()
	if !g.Is(id) {
		t.Fatal("Next mutated its receiver")
	}
}

func TestFlightGuardsOverlap(t *testing.T) {
	var f Flight
	f, id, ok := f.Start()
	if !ok || !f.Busy() || !f.Is(id) {
		t.Fatal("first Start must run")
	}
	if f2, _, ok := f.Start(); ok || !f2.Is(id) {
		t.Fatal("second Start while busy must be refused and change nothing")
	}
	f = f.Done(id)
	if f.Busy() {
		t.Fatal("Done must clear busy")
	}
	f, id2, ok := f.Start()
	if !ok || id2 == id {
		t.Fatal("Start after Done must run with a new id")
	}
}

func TestFlightResetSupersedes(t *testing.T) {
	var f Flight
	f, old, _ := f.Start()
	f = f.Reset()
	if f.Busy() || f.Is(old) {
		t.Fatal("Reset must free the slot and make the old id stale")
	}
	f, cur, ok := f.Start()
	if !ok {
		t.Fatal("Start after Reset must run")
	}
	// The abandoned request finishing late must not free the new one's slot.
	if f = f.Done(old); !f.Busy() {
		t.Fatal("a stale Done cleared the current request's flag")
	}
	if f = f.Done(cur); f.Busy() {
		t.Fatal("current Done must clear the flag")
	}
}

func TestFlightCopiesAreIndependent(t *testing.T) {
	var f Flight
	f, id, _ := f.Start()
	cp := f.Done(id)
	if !f.Busy() || cp.Busy() {
		t.Fatal("Done on a copy changed the original")
	}
}
