// Package async tags superseded work so its late results are dropped (engine
// principle E5).
//
// A tea.Cmd runs outside Update and its result arrives whenever it arrives.
// Without a tag, a slow earlier request lands after a fast later one and
// installs stale data: narwhal drew the previous run's tasks after switching,
// dayx showed a filter result after Esc cleared the filter, argx's tick,
// ctrl+r and post-sync reloads raced each other.
//
// Both types are plain values with no pointers, so a copy of the model holds
// an independent copy of the counter — exactly the semantics Bubble Tea's
// value models need.
//
// # Poll with an in-flight guard
//
// A poll whose period can be shorter than the call must not overlap itself
// (narwhal started a 5 s call every 1 s and drew snapshots out of order):
//
//	case tickMsg:
//		f, id, ok := m.poll.Start()
//		if !ok {
//			return m, nextTick() // one already running; skip this beat
//		}
//		m.poll = f
//		return m, tea.Batch(fetch(id), nextTick())
//	case fetchedMsg:
//		m.poll = m.poll.Done(msg.id)
//		if !m.poll.Is(msg.id) {
//			return m, nil // superseded by Reset
//		}
//		m.rows = msg.rows
//
// # Per-selection detail load
//
// Bump the generation whenever the selection changes — including when a
// refresh replaces the list — and drop results for any other generation.
// Otherwise the old row's detail passes the check and is drawn for a row that
// no longer exists (the Codex finding on ccproxy, E5):
//
//	func (m model) selectionChanged() (model, tea.Cmd) {
//		var id uint64
//		m.detailGen, id = m.detailGen.Next()
//		return m, loadDetail(m.selID, id)
//	}
//	case detailMsg:
//		if !m.detailGen.Is(msg.gen) {
//			return m, nil
//		}
//		m.detail = msg.body
package async

// Gen is a generation counter. The zero value is generation 0, which no
// request ever carries, so a result tagged before the first Next is stale.
type Gen struct{ n uint64 }

// Next returns the advanced counter and the new generation to tag a request
// with. Store the returned Gen; the receiver is unchanged.
func (g Gen) Next() (Gen, uint64) {
	g.n++
	return g, g.n
}

// Is reports whether id is the current generation.
func (g Gen) Is(id uint64) bool { return id != 0 && id == g.n }

// Current is the current generation (0 before the first Next).
func (g Gen) Current() uint64 { return g.n }

// Flight is a Gen plus an in-flight flag: at most one request runs at a time,
// and only the newest one's result is current.
type Flight struct {
	gen  Gen
	busy bool
}

// Start begins a request unless one is already in flight. It returns the
// updated Flight, the id to tag the request with, and false (with the Flight
// unchanged) when another request is still running.
func (f Flight) Start() (Flight, uint64, bool) {
	if f.busy {
		return f, 0, false
	}
	var id uint64
	f.gen, id = f.gen.Next()
	f.busy = true
	return f, id, true
}

// Done marks the request id finished. A Done for a request that is not
// current (one Reset since superseded) leaves the flag alone: the newer
// request is still running.
func (f Flight) Done(id uint64) Flight {
	if f.gen.Is(id) {
		f.busy = false
	}
	return f
}

// Reset abandons the request in flight: its result will fail Is, and Start
// may run a new one immediately. Use it when the thing being fetched changes
// (another group, another run) rather than waiting out a stale call.
func (f Flight) Reset() Flight {
	f.gen, _ = f.gen.Next()
	f.busy = false
	return f
}

// Busy reports whether a request is in flight.
func (f Flight) Busy() bool { return f.busy }

// Is reports whether id is the current request.
func (f Flight) Is(id uint64) bool { return f.gen.Is(id) }
