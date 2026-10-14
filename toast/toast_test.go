package toast

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// expire runs a tick command to get its Expired message. TTLs in these tests
// are a millisecond, so this does not slow the suite.
func expire(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a tick, got nil")
	}
	return cmd()
}

const fast = time.Millisecond

func texts(q Queue) string {
	var out []string
	for _, t := range q.Items() {
		out = append(out, t.Text)
	}
	return strings.Join(out, ",")
}

func TestPushAndExpire(t *testing.T) {
	q, cmd := Push(Queue{}, Toast{Text: "saved", Level: Success, TTL: fast})
	if q.Len() != 1 {
		t.Fatalf("len = %d", q.Len())
	}
	msg := expire(t, cmd)
	if e, ok := msg.(Expired); !ok || e.ID != q.Items()[0].ID() {
		t.Fatalf("tick delivered %#v", msg)
	}
	if q = Update(q, msg); q.Len() != 0 {
		t.Fatal("Expired did not remove the toast")
	}
}

func TestUpdateIgnoresOtherMessages(t *testing.T) {
	q, _ := Push(Queue{}, Toast{Text: "a", TTL: fast})
	if Update(q, tea.KeyMsg{}).Len() != 1 {
		t.Fatal("a key message removed a toast")
	}
}

// The case the id guard exists for: a toast replaced by a newer one from the
// same source must not be cleared by the old toast's tick.
func TestOldTickNeverClearsNewerToast(t *testing.T) {
	q, oldTick := Push(Queue{}, Toast{Source: "sync", Text: "syncing", TTL: fast})
	q, _ = Push(q, Toast{Source: "sync", Text: "synced", Level: Success, TTL: time.Hour})
	if texts(q) != "synced" {
		t.Fatalf("dedupe by source failed: %q", texts(q))
	}
	q = Update(q, expire(t, oldTick))
	if texts(q) != "synced" {
		t.Fatalf("old tick cleared the newer toast: %q", texts(q))
	}
}

func TestErrorsAreStickyUntilKeyAfterDwell(t *testing.T) {
	now := time.Now()
	q, cmd := push(Queue{}, Toast{Text: "boom", Level: Error}, now)
	if cmd != nil {
		t.Fatal("an error must not get an expiry tick")
	}
	if q = onKey(q, now.Add(MinDwell/2)); q.Len() != 1 {
		t.Fatal("a key pressed before the error could be read cleared it")
	}
	if q = onKey(q, now.Add(MinDwell)); q.Len() != 0 {
		t.Fatal("a key after the dwell did not clear the error")
	}
}

func TestOnKeyLeavesNonErrors(t *testing.T) {
	now := time.Now()
	q, _ := push(Queue{}, Toast{Text: "fyi", TTL: time.Hour}, now)
	if onKey(q, now.Add(time.Hour)).Len() != 1 {
		t.Fatal("OnKey removed a non-error toast")
	}
	if OnKey(q).Len() != 1 {
		t.Fatal("OnKey removed a non-error toast")
	}
}

func TestCapDropsOldestNonErrorFirst(t *testing.T) {
	q := New(3)
	q, _ = Push(q, Toast{Text: "err", Level: Error})
	for _, s := range []string{"a", "b", "c"} {
		q, _ = Push(q, Toast{Text: s, TTL: time.Hour})
	}
	if texts(q) != "err,b,c" {
		t.Fatalf("queue = %q; the unread error must survive a burst", texts(q))
	}
	all := New(2)
	for _, s := range []string{"e1", "e2", "e3"} {
		all, _ = Push(all, Toast{Text: s, Level: Error})
	}
	if texts(all) != "e2,e3" {
		t.Fatalf("all errors: %q", texts(all))
	}
	var zero Queue
	for _, s := range []string{"1", "2", "3", "4"} {
		zero, _ = Push(zero, Toast{Text: s, TTL: time.Hour})
	}
	if zero.Len() != DefaultMax {
		t.Fatalf("zero-value cap = %d, want %d", zero.Len(), DefaultMax)
	}
}

func TestIDsAreUnique(t *testing.T) {
	q, _ := Push(Queue{}, Toast{Text: "a", TTL: time.Hour})
	q, _ = Push(q, Toast{Text: "b", TTL: time.Hour})
	it := q.Items()
	if it[0].ID() == it[1].ID() || it[0].ID() == 0 {
		t.Fatalf("ids = %d, %d", it[0].ID(), it[1].ID())
	}
}

// Bubble Tea copies the model on every Update. Two copies of one queue must
// not see each other's pushes — the classic trap is append writing into
// spare capacity of a backing array both copies share.
func TestCopiesAreIndependent(t *testing.T) {
	base := New(5)
	base, _ = Push(base, Toast{Text: "x", TTL: time.Hour})
	base, _ = Push(base, Toast{Text: "y", TTL: time.Hour})

	a, b := base, base
	a, _ = Push(a, Toast{Text: "from-a", TTL: time.Hour})
	b, _ = Push(b, Toast{Text: "from-b", TTL: time.Hour})
	if texts(a) != "x,y,from-a" || texts(b) != "x,y,from-b" || texts(base) != "x,y" {
		t.Fatalf("copies aliased: a=%q b=%q base=%q", texts(a), texts(b), texts(base))
	}

	c := Update(a, Expired{ID: a.Items()[0].ID(), arm: a.Items()[0].arm})
	if texts(a) != "x,y,from-a" || texts(c) != "y,from-a" {
		t.Fatalf("Update mutated its input: a=%q c=%q", texts(a), texts(c))
	}

	items := a.Items()
	items[0].Text = "mutated"
	if texts(a) != "x,y,from-a" {
		t.Fatal("Items() exposed the internal slice")
	}
}

func TestHoldDefersRenderAndExpiry(t *testing.T) {
	now := time.Now()
	q, oldTick := push(Queue{}, Toast{Text: "before", TTL: fast}, now)
	q = Hold(q)
	if !q.Held() || View(q, 80, Styles{}) != "" {
		t.Fatal("a held queue must render nothing")
	}
	q, cmd := push(q, Toast{Text: "during", TTL: fast}, now)
	if cmd != nil {
		t.Fatal("no tick while held")
	}
	q, _ = push(q, Toast{Text: "err", Level: Error}, now)
	// tea.Tick's command reads a one-shot timer: running it twice blocks
	// forever, so the message is captured once and replayed.
	oldMsg := expire(t, oldTick)
	if q = Update(q, oldMsg); q.Len() != 3 {
		t.Fatal("a toast expired while held")
	}
	if q = onKey(q, now.Add(time.Hour)); q.Len() != 3 {
		t.Fatal("OnKey cleared an error nobody has seen")
	}

	q, rel := release(q, now)
	if q.Held() || rel == nil {
		t.Fatal("Release must resume and re-arm")
	}
	// The pre-hold tick is stale after re-arming.
	if q = Update(q, oldMsg); q.Len() != 3 {
		t.Fatal("a tick from before Hold expired a re-armed toast")
	}
	// The batch from Release delivers fresh ticks for the two non-errors.
	msgs := expire(t, rel).(tea.BatchMsg)
	if len(msgs) != 2 {
		t.Fatalf("release ticks = %d, want 2", len(msgs))
	}
	for _, c := range msgs {
		q = Update(q, c())
	}
	if texts(q) != "err" {
		t.Fatalf("after release ticks: %q", texts(q))
	}
	if q2, c := Release(q); c != nil || q2.Len() != 1 {
		t.Fatal("Release of an unheld queue is a no-op")
	}
}

func TestViewPrefixesAndFits(t *testing.T) {
	q := New(4)
	q, _ = Push(q, Toast{Text: "fyi", TTL: time.Hour})
	q, _ = Push(q, Toast{Text: "done", Level: Success, TTL: time.Hour})
	q, _ = Push(q, Toast{Text: "careful", Level: Warn, TTL: time.Hour})
	q, _ = Push(q, Toast{Text: "배포 실패: 권한이 없습니다\nsecond line", Level: Error})

	lines := strings.Split(ansi.Strip(View(q, 80, Styles{})), "\n")
	want := []string{"i fyi", "✓ done", "! careful", "✗ 배포 실패: 권한이 없습니다 second line"}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	ascii := ansi.Strip(View(q, 80, Styles{ASCII: true}))
	for _, w := range []string{"info: fyi", "ok: done", "warn: careful", "error: 배포"} {
		if !strings.Contains(ascii, w) {
			t.Errorf("ascii view missing %q:\n%s", w, ascii)
		}
	}
	for _, l := range strings.Split(View(q, 12, Styles{}), "\n") {
		if ansi.StringWidth(l) > 12 {
			t.Errorf("line %q exceeds 12 cells", ansi.Strip(l))
		}
	}
	if View(Queue{}, 80, Styles{}) != "" || View(q, 0, Styles{}) != "" {
		t.Error("empty queue or zero width renders nothing")
	}
}

func TestLevelString(t *testing.T) {
	for l, s := range map[Level]string{Info: "info", Success: "success", Warn: "warn", Error: "error", Level(9): "info"} {
		if l.String() != s {
			t.Errorf("%d.String() = %q", l, l.String())
		}
	}
	if (Styles{}).Prefix(Level(9)) != "i" {
		t.Error("unknown level falls back to info prefix")
	}
}
