package list

import (
	"fmt"
	"testing"
)

type row struct{ id, name string }

func (r row) ID() string { return r.id }

func rows(ids ...string) []row {
	out := make([]row, len(ids))
	for i, id := range ids {
		out[i] = row{id: id, name: "row " + id}
	}
	return out
}

func many(n int) []row {
	var ids []string
	for i := range n {
		ids = append(ids, fmt.Sprint(i))
	}
	return rows(ids...)
}

func TestZeroListIsEmptyAndSafe(t *testing.T) {
	var l List[row]
	if _, ok := l.Selected(); ok || l.Index() != -1 || l.SelectedID() != "" {
		t.Fatal("zero list has no selection")
	}
	l = l.Move(3).Top().Bottom().HalfDown().PageUp().SetHeight(5)
	if s, e := l.Window(); s != 0 || e != 0 || l.MoreLine() != "" {
		t.Fatal("zero list window is empty")
	}
	if _, ok := l.Select("x"); ok {
		t.Fatal("nothing to select")
	}
}

func TestReselectsByIDAfterReplace(t *testing.T) {
	l := New(rows("a", "b", "c", "d"))
	l, _ = l.Select("c")
	// A refresh inserts rows above and re-sorts: still on c.
	l = l.SetItems(rows("x", "y", "d", "c", "a"))
	if l.SelectedID() != "c" || l.Index() != 3 {
		t.Fatalf("after reorder: %q at %d", l.SelectedID(), l.Index())
	}
	// c is deleted: the row that took its place is selected, not the top.
	l = l.SetItems(rows("x", "y", "d", "a"))
	if l.SelectedID() != "a" || l.Index() != 3 {
		t.Fatalf("after delete: %q at %d", l.SelectedID(), l.Index())
	}
	// The last rows go: clamp.
	l = l.SetItems(rows("x"))
	if l.SelectedID() != "x" {
		t.Fatalf("after shrink: %q", l.SelectedID())
	}
	l = l.SetItems(nil)
	if l.Index() != -1 || l.SelectedID() != "" {
		t.Fatal("empty list has no selection")
	}
	// Empty before: the first item.
	if l = l.SetItems(rows("p", "q")); l.SelectedID() != "p" {
		t.Fatalf("from empty: %q", l.SelectedID())
	}
}

func TestMovesAndWindow(t *testing.T) {
	l := New(many(100)).SetHeight(10)
	l = l.Move(15)
	if s, e := l.Window(); s != 6 || e != 16 || l.Index() != 15 {
		t.Fatalf("window = %d..%d at %d", s, e, l.Index())
	}
	if a, b := l.Hidden(); a != 6 || b != 84 || l.MoreLine() != "↑ 6 ↓ 84 more" {
		t.Fatalf("hidden = %d %d %q", a, b, l.MoreLine())
	}
	l = l.HalfDown()
	if l.Index() != 20 {
		t.Fatalf("half down = %d", l.Index())
	}
	l = l.HalfUp().HalfUp()
	if l.Index() != 10 {
		t.Fatalf("half up = %d", l.Index())
	}
	l = l.PageDown()
	if l.Index() != 20 {
		t.Fatalf("page down = %d", l.Index())
	}
	l = l.Bottom()
	if s, e := l.Window(); s != 90 || e != 100 || l.Index() != 99 {
		t.Fatalf("bottom window = %d..%d", s, e)
	}
	l = l.Move(5).PageUp().Top().Move(-3)
	if l.Index() != 0 {
		t.Fatalf("clamped top = %d", l.Index())
	}
	// Growing the pane keeps the cursor visible without jumping.
	l = l.SelectIndex(50).SetHeight(4)
	if s, _ := l.Window(); s > 50 || s+4 <= 50 {
		t.Fatalf("cursor off screen after shrink: start %d", s)
	}
	l = l.SetHeight(40)
	if s, e := l.Window(); s > 50 || e <= 50 {
		t.Fatalf("cursor off screen after grow: %d..%d", s, e)
	}
	// Height 0: half page still moves one row.
	if New(many(5)).HalfDown().Index() != 1 {
		t.Fatal("height 0 half page moves 1")
	}
}

func TestCopiesAreIndependent(t *testing.T) {
	src := rows("a", "b", "c")
	a := New(src)
	src[0] = row{id: "mutated"}
	if a.At(0).ID() != "a" {
		t.Fatal("New must copy the items")
	}
	b := a.Move(2).SetItems(rows("z", "c"))
	if a.SelectedID() != "a" || a.Len() != 3 || b.SelectedID() != "c" {
		t.Fatalf("copies share state: a=%q b=%q", a.SelectedID(), b.SelectedID())
	}
	items := a.Items()
	items[1] = row{id: "mutated"}
	if a.At(1).ID() != "b" {
		t.Fatal("Items must return a copy")
	}
}
