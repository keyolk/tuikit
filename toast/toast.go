// Package toast is a value-type notification queue (engine principle E6).
//
// It replaces the single `status string` most apps carry, which failed three
// ways in the fleet audit: it never cleared (a gh auth error covered brd's hint
// bar forever), it had no severity (argx showed "sync failed" in success
// green), and it expired only when some other message arrived because expiry
// was computed in View. Here expiry is a tea.Tick per toast, guarded by id, and
// severity is both a color and a prefix.
//
// Queue is a value. Every function takes a Queue and returns a new one; none
// of them write into a slice the caller's copy can see, so a model copied by
// Bubble Tea never has its toasts changed behind its back.
package toast

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Level is a toast's severity.
type Level int

const (
	Info Level = iota
	Success
	Warn
	Error
)

func (l Level) String() string {
	switch l {
	case Success:
		return "success"
	case Warn:
		return "warn"
	case Error:
		return "error"
	}
	return "info"
}

const (
	// DefaultMax is how many toasts are kept (and drawn) at once.
	DefaultMax = 3
	// DefaultTTL is how long a non-error toast stays when Toast.TTL is zero.
	DefaultTTL = 4 * time.Second
	// MinDwell is how long an error must have been on screen before a key
	// clears it, so a key already in flight when the error lands does not
	// dismiss it unread.
	MinDwell = 750 * time.Millisecond
)

// Toast is one notification.
type Toast struct {
	// Source dedupes: a new toast replaces the queued one with the same
	// Source ("sync", "load"), so a retry loop shows its latest state instead
	// of stacking. Empty Source never dedupes.
	Source string
	Text   string
	Level  Level
	TTL    time.Duration // zero means DefaultTTL; ignored for Error

	id      uint64
	arm     uint64    // which tick may expire it; bumped on Release
	shownAt time.Time // zero while held, i.e. not yet on screen
}

// ID identifies this toast within its queue.
func (t Toast) ID() uint64 { return t.id }

// Expired is the message a toast's tick delivers. Pass it to Update.
type Expired struct {
	ID  uint64
	arm uint64
}

// Queue holds the visible toasts. The zero value is ready to use with
// DefaultMax; New sets another cap.
type Queue struct {
	items []Toast
	next  uint64
	max   int
	held  bool
	// now is the clock; nil means time.Now. A func is safe to share between
	// copies: it is replaced, never mutated.
	now func() time.Time
}

// WithClock returns q reading time from now instead of time.Now, so a test
// can step past MinDwell without sleeping. Ticks still use real timers; build
// their messages with ExpiredMsg instead of waiting for them.
func WithClock(q Queue, now func() time.Time) Queue {
	q.now = now
	return q
}

func (q Queue) clock() time.Time {
	if q.now == nil {
		return time.Now()
	}
	return q.now()
}

// New returns an empty queue showing at most max toasts.
func New(max int) Queue { return Queue{max: max} }

// Len is the number of queued toasts.
func (q Queue) Len() int { return len(q.items) }

// Held reports whether the queue is held for a dialog.
func (q Queue) Held() bool { return q.held }

// Items returns a copy of the queued toasts, oldest first.
func (q Queue) Items() []Toast { return append([]Toast(nil), q.items...) }

func (q Queue) limit() int {
	if q.max <= 0 {
		return DefaultMax
	}
	return q.max
}

// Push queues t and returns the tick that will expire it. Errors are sticky
// (nil cmd): they stay until a key after they were seen (OnKey) or until a
// newer toast from the same Source replaces them. While the queue is held no
// tick is started; Release starts it, so a toast is never shown for less than
// its TTL.
//
// When the cap is exceeded the oldest non-error toast goes first, so a burst
// of successes cannot push an unread error off the screen.
func Push(q Queue, t Toast) (Queue, tea.Cmd) {
	return push(q, t, q.clock())
}

func push(q Queue, t Toast, now time.Time) (Queue, tea.Cmd) {
	items := make([]Toast, 0, len(q.items)+1)
	for _, it := range q.items {
		if t.Source == "" || it.Source != t.Source {
			items = append(items, it)
		}
	}
	q.next++
	t.id, t.arm = q.next, q.next
	t.shownAt = time.Time{}
	if !q.held {
		t.shownAt = now
	}
	items = append(items, t)
	for len(items) > q.limit() {
		items = dropOldest(items)
	}
	q.items = items
	if q.held {
		return q, nil
	}
	return q, tick(t)
}

// dropOldest removes the oldest non-error toast, or the oldest toast when
// every one is an error. items is freshly allocated by the caller.
func dropOldest(items []Toast) []Toast {
	at := 0
	for i, it := range items {
		if it.Level != Error {
			at = i
			break
		}
	}
	return append(items[:at:at], items[at+1:]...)
}

func tick(t Toast) tea.Cmd {
	if t.Level == Error {
		return nil
	}
	d := t.TTL
	if d <= 0 {
		d = DefaultTTL
	}
	id, arm := t.id, t.arm
	return tea.Tick(d, func(time.Time) tea.Msg { return Expired{ID: id, arm: arm} })
}

// Update applies an Expired message. The id and arm must both match: a tick
// from a toast that was since replaced (same Source, new id) or re-armed by
// Release finds nothing to remove, so an old tick never clears a newer toast.
// Other messages return q unchanged.
func Update(q Queue, msg tea.Msg) Queue {
	e, ok := msg.(Expired)
	if !ok || q.held {
		return q
	}
	return q.filter(func(t Toast) bool { return t.id != e.ID || t.arm != e.arm })
}

// ExpiredMsg is the message the live tick of toast id will deliver, for tests
// that drive expiry without waiting on a timer. ok is false when id is not
// queued or is an error (errors have no tick). A message built before a
// Release is stale after it, exactly as the real tick would be.
func (q Queue) ExpiredMsg(id uint64) (Expired, bool) {
	for _, t := range q.items {
		if t.id == id && t.Level != Error {
			return Expired{ID: id, arm: t.arm}, true
		}
	}
	return Expired{}, false
}

// ExpireAll removes every non-error toast as if each tick had fired now.
// Errors stay: they are sticky by design. A held queue is unchanged, as with
// Update.
func ExpireAll(q Queue) Queue {
	if q.held {
		return q
	}
	return q.filter(func(t Toast) bool { return t.Level == Error })
}

// Dismiss removes every toast from source, errors included, held or not: the
// condition it reported is gone (a retry succeeded, a dialog was closed). An
// empty source matches nothing, since empty-source toasts are anonymous.
// Their pending ticks find nothing to remove.
func Dismiss(q Queue, source string) Queue {
	if source == "" {
		return q
	}
	return q.filter(func(t Toast) bool { return t.Source != source })
}

// OnKey clears sticky errors that have been on screen for at least MinDwell.
// Call it at the top of key handling, before the handler can push a new
// error in response to this same key.
func OnKey(q Queue) Queue { return onKey(q, q.clock()) }

func onKey(q Queue, now time.Time) Queue {
	if q.held {
		return q
	}
	return q.filter(func(t Toast) bool {
		return t.Level != Error || t.shownAt.IsZero() || now.Sub(t.shownAt) < MinDwell
	})
}

func (q Queue) filter(keep func(Toast) bool) Queue {
	var items []Toast
	for _, t := range q.items {
		if keep(t) {
			items = append(items, t)
		}
	}
	q.items = items
	return q
}

// Hold pauses the queue while a dialog is open (E6: toasts wait). Pushes still
// queue, but nothing renders and nothing expires until Release.
func Hold(q Queue) Queue {
	q.held = true
	return q
}

// Release resumes the queue and re-arms every non-error toast with a full
// TTL: they were hidden, so their time on screen starts now. Ticks started
// before Hold are invalidated by the new arm.
func Release(q Queue) (Queue, tea.Cmd) { return release(q, q.clock()) }

func release(q Queue, now time.Time) (Queue, tea.Cmd) {
	if !q.held {
		return q, nil
	}
	q.held = false
	items := make([]Toast, len(q.items))
	var cmds []tea.Cmd
	for i, t := range q.items {
		t.shownAt = now
		if t.Level != Error {
			q.next++
			t.arm = q.next
			cmds = append(cmds, tick(t))
		}
		items[i] = t
	}
	q.items = items
	return q, tea.Batch(cmds...)
}

// Styles colors toasts by level. Color alone is not enough — NO_COLOR and
// color-blind readers lose it — so every line also carries a prefix.
type Styles struct {
	Info, Success, Warn, Error lipgloss.Style
	// ASCII swaps the glyph prefixes for words, for terminals whose locale
	// cannot be trusted with ✓/✗.
	ASCII bool
}

// Prefix is the marker drawn before a toast of level l.
func (st Styles) Prefix(l Level) string {
	if l < Info || l > Error {
		l = Info
	}
	if st.ASCII {
		return asciiPrefix[l]
	}
	return glyphPrefix[l]
}

var (
	glyphPrefix = [...]string{Info: "i", Success: "✓", Warn: "!", Error: "✗"}
	asciiPrefix = [...]string{Info: "info:", Success: "ok:", Warn: "warn:", Error: "error:"}
)

func (st Styles) style(l Level) lipgloss.Style {
	switch l {
	case Success:
		return st.Success
	case Warn:
		return st.Warn
	case Error:
		return st.Error
	}
	return st.Info
}

// View draws one line per toast, oldest first, each at most width cells. A
// held queue draws nothing. View reads only; it never expires anything.
func View(q Queue, width int, st Styles) string {
	if q.held || len(q.items) == 0 || width <= 0 {
		return ""
	}
	lines := make([]string, len(q.items))
	for i, t := range q.items {
		text := strings.ReplaceAll(t.Text, "\n", " ")
		line := ansi.Truncate(st.Prefix(t.Level)+" "+text, width, "…")
		lines[i] = st.style(t.Level).Render(line)
	}
	return strings.Join(lines, "\n")
}
