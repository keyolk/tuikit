package main

import (
	"io"
	"log"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/prefs"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/toast"
)

// The pattern every app using keys should copy: the binding table is checked
// in a test, for every context stack the app can be in.
func TestKeymapHasNoConflicts(t *testing.T) {
	for _, c := range keymap().Conflicts(stacks...) {
		t.Error(c)
	}
}

func testModel(t *testing.T) model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var items []item
	for i := 0; i < 30; i++ {
		items = append(items, item{ID: string(rune('a' + i)), Name: "서비스 " + string(rune('a'+i))})
	}
	m := model{
		km: keymap(), th: theme.NewWith(lipgloss.NewRenderer(io.Discard), theme.DefaultRoles(), theme.Unicode()),
		logf: log.New(io.Discard, "", 0).Printf, items: items, selID: "a", toasts: toast.New(3),
		saver: prefs.NewSaver[uiState](app, time.Hour),
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	return next.(model)
}

func press(m model, k string) model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	switch k {
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		msg = tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	next, _ := m.Update(msg)
	return next.(model)
}

func assertFits(t *testing.T, m model) {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	if len(lines) > m.h {
		t.Fatalf("view is %d lines, terminal %d", len(lines), m.h)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > m.w {
			t.Fatalf("line %d is %d cells > %d", i, w, m.w)
		}
	}
}

func TestDemoNavigatesUnderKoreanLayoutAndStaysInFrame(t *testing.T) {
	m := testModel(t)
	for i := 0; i < 20; i++ {
		m = press(m, "ㅓ") // physical j
	}
	if m.selID != string(rune('a'+20)) {
		t.Fatalf("selID = %q after 20 downs", m.selID)
	}
	assertFits(t, m)
	m = press(m, "x") // error toast
	assertFits(t, m)
	m = press(m, "?")
	if !m.help || !m.toasts.Held() {
		t.Fatal("help did not open or toasts not held")
	}
	assertFits(t, m)
	m = press(m, "esc")
	if m.help || m.toasts.Held() {
		t.Fatal("esc did not close help")
	}
}

func TestStaleDetailIsDropped(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(selectMsg{})
	m = next.(model)
	stale := m.detailGen.Current()
	m = press(m, "j")
	next, _ = m.Update(detailMsg{gen: stale, body: "old row"})
	if next.(model).detail == "old row" {
		t.Fatal("a detail load for the previous selection was drawn")
	}
}

func TestFooterFollowsWrapState(t *testing.T) {
	m := testModel(t)
	if f := m.km.Footer(m.active(), 200, m.keyOpts(), keys.FooterStyle{}); !strings.Contains(f, "w wrap") || !strings.Contains(f, "jk move") {
		t.Fatalf("footer = %q", f)
	}
	m = press(m, "w")
	if f := m.km.Footer(m.active(), 200, m.keyOpts(), keys.FooterStyle{}); !strings.Contains(f, "w unwrap") {
		t.Fatalf("footer = %q after w", f)
	}
}

func TestQuitFlushesThenQuits(t *testing.T) {
	m := testModel(t)
	m = press(m, "j") // a debounced save is now pending (delay is an hour)
	if !m.saver.Pending() {
		t.Fatal("moving must schedule a save")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	saved, ok := cmd().(prefs.Saved)
	if !ok || !saved.Final || saved.Err != nil {
		t.Fatalf("quit cmd = %+v; want the Final flush", saved)
	}
	st, err := prefs.Load(app, uiState{})
	if err != nil || st.SelID != "b" {
		t.Fatalf("flushed state = %+v, %v", st, err)
	}
	if _, cmd := next.Update(saved); cmd == nil {
		t.Fatal("the Final Saved must quit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("the Final Saved must quit")
	}
}

func TestHelpOffsetStopsAtLastPage(t *testing.T) {
	m := testModel(t)
	m = press(m, "?")
	for range 200 {
		m = press(m, "j")
	}
	held := m.helpOff
	if held > 50 {
		t.Fatalf("help offset grew to %d on a held key", held)
	}
	m = press(m, "k")
	if m.helpOff != max(0, held-1) {
		t.Fatalf("one k moved the offset from %d to %d", held, m.helpOff)
	}
	assertFits(t, m)
	m = press(m, "q") // closes help, does not quit
	if m.help {
		t.Fatal("q must close help first")
	}
}

func TestOpenDismissesTheFailure(t *testing.T) {
	m := testModel(t)
	m = press(m, "x")
	m = press(m, " ")
	for _, it := range m.toasts.Items() {
		if it.Source == "fail" {
			t.Fatal("open must dismiss the sticky failure")
		}
	}
}
