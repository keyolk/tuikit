package prefs

import (
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultDelay is the debounce a Saver uses when NewSaver is given zero.
const DefaultDelay = 500 * time.Millisecond

// Saver debounces UI-state writes: a burst of changes (holding j through a
// list) becomes one write once the changes stop for the delay. Writes are
// ordered (see writeCmd), so an older write never lands after a newer one,
// and Flush writes the latest state at once for quit.
//
// Saver is a value like the rest of the model. It holds no pointers that a
// copy could see change: the encoded bytes are never written after Save
// creates them. Wire it in Update:
//
//	case prefs.SaveTick:
//		m.saver, cmd = m.saver.Update(msg)
//	case prefs.Saved:
//		if msg.Err != nil { /* toast */ }
//		if msg.Final { return m, tea.Quit }
//
// and on every change call m.saver, cmd = m.saver.Save(m.uiState()); on quit
// m.saver, cmd = m.saver.FlushWith(m.uiState()) and quit on the Final Saved.
//
// The zero Saver (and one for app "") persists nothing: Save and Update are
// no-ops and Flush and FlushWith report Saved{Final: true} at once. No Saver
// returns tea.Quit itself, the zero one included: the app quits on the Final
// Saved, so a test model built as a literal goes through the same quit path. A model literal in a test, or an
// app run with persistence off, never writes $XDG_STATE_HOME/ui.json — which
// is where app "" would otherwise land (brd, gcl).
type Saver[T any] struct {
	app   string
	delay time.Duration
	gen   uint64 // bumped by Save; only the tick carrying it may write
	data  []byte // latest encoded state; nil until the first Save
	dirty bool   // data changed since the last write was issued
}

// SaveTick is the debounce timer's message. Pass it to Saver.Update.
type SaveTick struct {
	app string
	gen uint64
}

// NewSaver returns a Saver for app's ui.json. delay zero means DefaultDelay;
// a negative delay writes on every Save (still ordered).
func NewSaver[T any](app string, delay time.Duration) Saver[T] {
	if delay == 0 {
		delay = DefaultDelay
	}
	return Saver[T]{app: app, delay: delay}
}

// Pending reports whether a change is waiting for its debounce.
func (s Saver[T]) Pending() bool { return s.dirty }

// Save records v and (re)starts the debounce. v is encoded now, in Update,
// for the same reason as SaveCmd: the write runs on another goroutine. An
// encoding error is reported at once as Saved{Err} and the previous state is
// kept.
func (s Saver[T]) Save(v T) (Saver[T], tea.Cmd) {
	if s.app == "" {
		return s, nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		err = fmt.Errorf("prefs: encode: %w", err)
		app := s.app
		return s, func() tea.Msg { return Saved{App: app, Err: err} }
	}
	s.data, s.dirty = append(data, '\n'), true
	s.gen++
	if s.delay < 0 {
		s.dirty = false
		return s, writeCmd(s.app, s.data, false)
	}
	app, gen := s.app, s.gen
	return s, tea.Tick(s.delay, func(time.Time) tea.Msg { return SaveTick{app: app, gen: gen} })
}

// TickMsg is the message the live debounce timer will deliver, for tests
// that drive the Saver without waiting on it.
func (s Saver[T]) TickMsg() SaveTick { return SaveTick{app: s.app, gen: s.gen} }

// Update handles the debounce tick: the tick of the latest Save writes, any
// older one (re-armed by a later Save) does nothing. Other messages, and
// ticks of another app's Saver, return s unchanged.
func (s Saver[T]) Update(msg tea.Msg) (Saver[T], tea.Cmd) {
	t, ok := msg.(SaveTick)
	if !ok || s.app == "" || t.app != s.app || t.gen != s.gen || !s.dirty {
		return s, nil
	}
	s.dirty = false
	return s, writeCmd(s.app, s.data, false)
}

// Flush writes the latest state now and reports Saved{Final: true} when it is
// on disk, cancelling any pending debounce. It rewrites even when nothing is
// pending: an earlier write may still be in flight, and quitting on its
// Saved could kill it half-done; the flush write is newer, so the earlier
// one cannot land over it. With nothing ever saved it reports Final at once.
func (s Saver[T]) Flush() (Saver[T], tea.Cmd) {
	s.gen++ // a debounce tick still in flight finds a newer gen and does nothing
	s.dirty = false
	if s.data == nil || s.app == "" {
		app := s.app
		return s, func() tea.Msg { return Saved{App: app, Final: true} }
	}
	return s, writeCmd(s.app, s.data, true)
}

// FlushWith records v and flushes it in one step, for quit: the model's
// final state is usually not what the last Save saw. An encoding error is
// reported as Saved{Err, Final: true}, so the app still quits — it keeps the
// previous file and should log the error.
func (s Saver[T]) FlushWith(v T) (Saver[T], tea.Cmd) {
	if s.app == "" {
		return s.Flush()
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		s.gen++
		s.dirty = false
		err = fmt.Errorf("prefs: encode: %w", err)
		app := s.app
		return s, func() tea.Msg { return Saved{App: app, Err: err, Final: true} }
	}
	s.data = append(data, '\n')
	return s.Flush()
}
