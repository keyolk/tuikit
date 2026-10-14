package main

import (
	"io"
	"log"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/toast"
)

// The pattern every app using keys should copy: the binding table is checked
// in a test, for every context stack the app can be in.
func TestKeymapHasNoConflicts(t *testing.T) {
	for _, c := range keymap().Conflicts([]keys.Ctx{ctxList, ctxGlobal}, []keys.Ctx{ctxHelp, ctxGlobal}) {
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
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	return next.(model)
}

func press(m model, k string) model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	if k == "esc" {
		msg = tea.KeyMsg{Type: tea.KeyEsc}
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
