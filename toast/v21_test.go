package toast

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// msgs runs cmd and flattens a batch into its tick messages.
func msgs(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	m := cmd()
	b, ok := m.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{m}
	}
	var out []tea.Msg
	for _, c := range b {
		out = append(out, msgs(t, c)...)
	}
	return out
}

func TestProgressIsStickyUntilReplacedOrDismissed(t *testing.T) {
	now := time.Unix(0, 0)
	q := WithClock(Queue{}, func() time.Time { return now })
	q, cmd := Push(q, Toast{Source: "sync", Text: "syncing 1/3", Level: Progress})
	if cmd != nil {
		t.Fatal("Progress has no tick")
	}
	if _, ok := q.ExpiredMsg(q.Items()[0].ID()); ok {
		t.Fatal("Progress has no Expired message")
	}
	now = now.Add(time.Hour)
	if q = OnKey(q); q.Len() != 1 {
		t.Fatal("a key must not clear a running task")
	}
	if q = ExpireAll(q); q.Len() != 1 {
		t.Fatal("ExpireAll must keep Progress")
	}
	q, _ = Hold(q), cmd
	q, cmd = Release(q)
	if len(msgs(t, cmd)) != 0 || q.Len() != 1 {
		t.Fatal("Release must not arm Progress")
	}
	// The result replaces it.
	q, cmd = Push(q, Toast{Source: "sync", Text: "synced", Level: Success, TTL: fast})
	if texts(q) != "synced" || cmd == nil {
		t.Fatalf("result did not replace progress: %q", texts(q))
	}
	q, _ = Push(q, Toast{Source: "load", Text: "loading", Level: Progress})
	if q = Dismiss(q, "load"); texts(q) != "synced" {
		t.Fatalf("Dismiss = %q", texts(q))
	}
}

func TestCapPrefersDroppingNonSticky(t *testing.T) {
	q := New(2)
	q, _ = Push(q, Toast{Source: "p", Text: "running", Level: Progress})
	q, _ = Push(q, Toast{Text: "a", TTL: fast})
	q, _ = Push(q, Toast{Text: "b", TTL: fast})
	if texts(q) != "running,b" {
		t.Fatalf("cap dropped progress: %q", texts(q))
	}
}

func TestAddAndTicks(t *testing.T) {
	before := New(5)
	q := Add(before, Toast{Text: "one", TTL: fast})
	q = Add(q, Toast{Text: "err", Level: Error})
	q = Add(q, Toast{Source: "p", Text: "busy", Level: Progress})
	q = Add(q, Toast{Text: "two", TTL: fast})
	got := msgs(t, Ticks(before, q))
	if len(got) != 2 {
		t.Fatalf("ticks = %d, want 2 (errors and progress are sticky)", len(got))
	}
	for _, m := range got {
		q = Update(q, m)
	}
	if texts(q) != "err,busy" {
		t.Fatalf("after ticks: %q", texts(q))
	}
	// Nothing new: nothing owed.
	if len(msgs(t, Ticks(q, q))) != 0 {
		t.Fatal("no new toasts owe no ticks")
	}
	// Held: Release owes them, and Ticks sees Release's re-arm too.
	h := Add(Hold(q), Toast{Text: "later", TTL: fast})
	if Ticks(q, h) != nil {
		t.Fatal("a held queue owes nothing")
	}
	r, _ := Release(h)
	if n := len(msgs(t, Ticks(h, r))); n != 1 {
		t.Fatalf("after Release Ticks = %d, want 1", n)
	}
	// Add never touches the queue it was given.
	if before.Len() != 0 || q.Len() != 2 {
		t.Fatal("Add mutated its input")
	}
}

func TestRenderItemAndEllipsis(t *testing.T) {
	long := Toast{Text: "a very long message\nwith a newline", Level: Warn}
	got := RenderItem(long, 12, Styles{})
	if ansi.StringWidth(got) > 12 || !strings.HasSuffix(got, "…") || strings.Contains(got, "\n") {
		t.Fatalf("RenderItem = %q", got)
	}
	if got := RenderItem(long, 12, Styles{Ellipsis: "~"}); !strings.HasSuffix(got, "~") {
		t.Fatalf("custom marker = %q", got)
	}
	t.Setenv("LANG", "ko_KR.UTF-8")
	if got := RenderItem(long, 12, Styles{}); strings.Contains(got, "…") {
		t.Fatalf("CJK default = %q", got)
	}
	if RenderItem(long, 0, Styles{}) != "" {
		t.Fatal("zero width is empty")
	}
	if p := (Styles{}).Prefix(Progress); p != "◌" || (Styles{ASCII: true}).Prefix(Progress) != "wait:" {
		t.Fatalf("progress prefix = %q", p)
	}
	q, _ := Push(Queue{}, Toast{Text: "x", Level: Progress, Source: "s"})
	if View(q, 20, Styles{}) != RenderItem(q.Items()[0], 20, Styles{}) {
		t.Fatal("View must draw each toast with RenderItem")
	}
	if Progress.String() != "progress" {
		t.Fatal(Progress.String())
	}
}
