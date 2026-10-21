package selection

import (
	"testing"
	"time"
)

// A test that needs a slow (or instant) auto-scroll sets Interval on its own
// Selection; the package variable, shared with parallel tests, is untouched.
func TestIntervalIsPerSelection(t *testing.T) {
	s := Selection{Interval: time.Hour}
	s.Clear() // Clear keeps the configuration
	offset := 0
	panes := func() []Pane { return panesFor(offset) }
	frame := func() string { return frameFor(offset) }
	s.Update(press(textX, 0), panes, frame, t0)
	r, _ := s.Update(motion(textX+3, textH+2), panes, frame, t0)
	if r.AutoScroll == nil {
		t.Fatal("no auto-scroll")
	}
	done := make(chan struct{})
	go func() { r.AutoScroll(); close(done) }()
	select {
	case <-done:
		t.Fatalf("the tick fired at the package interval (%v), not the Selection's hour", AutoScrollInterval)
	case <-time.After(4 * AutoScrollInterval):
	}
	if AutoScrollInterval != 50*time.Millisecond {
		t.Fatalf("the package interval changed to %v", AutoScrollInterval)
	}
}
