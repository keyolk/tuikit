package main

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/kitty"
	"github.com/keyolk/tuikit/list"
	"github.com/keyolk/tuikit/prefs"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/toast"
	"github.com/keyolk/tuikit/tuitest"
)

func TestMain(m *testing.M) {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "RUNEWIDTH_EASTASIAN"} {
		os.Unsetenv(k)
	}
	os.Setenv("LANG", "en_US.UTF-8")
	os.Exit(m.Run())
}

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
		items = append(items, item{id: string(rune('a' + i)), name: "서비스 " + string(rune('a'+i))})
	}
	m := model{
		km: keymap(), th: theme.NewWith(lipgloss.NewRenderer(io.Discard), theme.DefaultRoles(), theme.Unicode()),
		logf: log.New(io.Discard, "", 0).Printf, all: items, rows: list.New(items), toasts: toast.New(3),
		saver: prefs.NewSaver[uiState](app, time.Hour),
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	return next.(model)
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func press(m model, ks ...string) model {
	for _, k := range ks {
		next, _ := m.Update(keyMsg(k))
		m = next.(model)
	}
	return m
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
	if m.rows.SelectedID() != string(rune('a'+20)) {
		t.Fatalf("selected %q after 20 downs", m.rows.SelectedID())
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

func TestFooterFollowsStateAndLiveLabels(t *testing.T) {
	m := testModel(t)
	f := m.km.Footer(m.active(), 200, m.keyOpts(), keys.FooterStyle{})
	if !strings.Contains(f, "w wrap") || !strings.Contains(f, "jk move") || !strings.Contains(f, "1-9 jump") {
		t.Fatalf("footer = %q", f)
	}
	m = press(m, "w", "/", "서", "enter")
	f = m.km.Footer(m.active(), 200, m.keyOpts(), keys.FooterStyle{})
	if !strings.Contains(f, "w unwrap") || !strings.Contains(f, "/ filter: 서") || !strings.Contains(f, "jk move in matches") {
		t.Fatalf("footer = %q after w and a filter", f)
	}
}

func TestFilterTypesKoreanAndKeepsTheCursorOnItsItem(t *testing.T) {
	m := testModel(t)
	m = press(m, "j", "j") // on c
	m = press(m, "/")
	// q and ? are text here, and jamo are not normalized to shortcuts.
	m = press(m, "q")
	if !m.filtering || m.query != "q" {
		t.Fatalf("q must be typed into the filter: filtering=%v query=%q", m.filtering, m.query)
	}
	m = press(m, "backspace", "서비스 c")
	if m.rows.SelectedID() != "c" || m.rows.Len() == 0 {
		t.Fatalf("filtered to %d rows, cursor on %q", m.rows.Len(), m.rows.SelectedID())
	}
	assertFits(t, m)
	m = press(m, "esc")
	if m.filtering || m.query != "" || m.rows.Len() != 30 || m.rows.SelectedID() != "c" {
		t.Fatalf("esc must clear the filter and keep the cursor: %q on %q", m.query, m.rows.SelectedID())
	}
	// ctrl+c still quits from inside the field.
	m = press(m, "/")
	if _, cmd := m.Update(keyMsg("ctrl+c")); cmd == nil {
		t.Fatal("ctrl+c must quit while typing")
	}
}

func TestReverseAndJumpKeepIDAnchoring(t *testing.T) {
	m := testModel(t)
	m = press(m, "5") // 5 tenths of 30 → row 15
	if m.rows.SelectedID() != string(rune('a'+15)) {
		t.Fatalf("jump selected %q", m.rows.SelectedID())
	}
	m = press(m, "S")
	if m.rows.SelectedID() != string(rune('a'+15)) || m.rows.Index() != 14 {
		t.Fatalf("reverse moved the cursor off its item: %q at %d", m.rows.SelectedID(), m.rows.Index())
	}
	m = press(m, "G")
	if m.rows.Index() != 29 {
		t.Fatal("G goes to the last row")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if m = next.(model); m.rows.Index() >= 29 {
		t.Fatal("ctrl+u moves half a page up")
	}
}

func TestRefreshShowsProgressThenResult(t *testing.T) {
	m := testModel(t)
	next, cmd := m.Update(keyMsg("r"))
	m = next.(model)
	if it := m.toasts.Items(); len(it) != 1 || it[0].Level != toast.Progress {
		t.Fatalf("toasts = %+v", it)
	}
	// The fetch is a 400ms tick; Drain waits for it and skips nothing else.
	var fetched tea.Msg
	for _, msg := range tuitest.Drain(cmd, time.Second) {
		if _, ok := msg.(fetchedMsg); ok {
			fetched = msg
		}
	}
	if fetched == nil {
		t.Fatal("refresh produced no fetch result")
	}
	next, _ = m.Update(fetched)
	m = next.(model)
	if it := m.toasts.Items(); len(it) != 1 || it[0].Level != toast.Success || it[0].Text != "refreshed" {
		t.Fatalf("the result must replace the progress toast: %+v", it)
	}
	if f := m.km.Footer(m.active(), 200, m.keyOpts(), keys.FooterStyle{}); !strings.Contains(f, "r refresh (1)") {
		t.Fatalf("live label missing: %q", f)
	}
}

func TestFooterShowsPositionFlushRight(t *testing.T) {
	m := testModel(t)
	m = press(m, "j")
	last := strings.Split(m.View(), "\n")
	footer := last[len(last)-1]
	if !strings.HasSuffix(ansi.Strip(footer), " 2/30") {
		t.Fatalf("footer = %q", footer)
	}
	if lipgloss.Width(footer) != m.w {
		t.Fatalf("footer is %d cells, want %d", lipgloss.Width(footer), m.w)
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
	if held == 0 || held > 50 {
		t.Fatalf("help offset = %d after a held key", held)
	}
	m = press(m, "k")
	if m.helpOff != held-1 {
		t.Fatalf("one k moved the offset from %d to %d", held, m.helpOff)
	}
	assertFits(t, m)
	if !strings.Contains(m.View(), "keys") {
		t.Fatal("the help box has its title")
	}
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

func TestPictureTransmitsOnceAndFits(t *testing.T) {
	var tty strings.Builder
	m := testModel(t)
	m.pic = newPicture(kitty.Caps{Supported: true}, kitty.NewOut(&tty))
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	m = next.(model)
	for _, msg := range tuitest.Drain(cmd, 0) {
		if w, ok := msg.(kitty.Wrote); !ok || w.Err != nil {
			t.Fatalf("resize produced %#v", msg)
		}
	}
	if !strings.Contains(tty.String(), "a=t,t=d,f=100,i=1") || !strings.Contains(tty.String(), "U=1") {
		t.Fatalf("no transmit and placement on the tty: %q", tty.String()[:min(60, tty.Len())])
	}
	if strings.Contains(m.View(), "a=t") {
		t.Fatal("image bytes leaked into the frame")
	}
	if !strings.ContainsRune(m.View(), kitty.PlaceholderRune) {
		t.Fatal("no placeholder cells in the detail pane")
	}
	assertFits(t, m)

	// Same size again: nothing to send.
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: 60, Height: 14}); len(tuitest.Drain(cmd, 0)) != 0 {
		t.Fatal("an unchanged picture was sent again")
	}

	// A failed write is shown, not dropped.
	next, _ = m.Update(kitty.Wrote{ID: 1, Err: errors.New("tty gone")})
	if v := next.(model).View(); !strings.Contains(v, "tty gone") {
		t.Fatalf("write error not shown:\n%s", v)
	}
}

func TestPictureFallsBackToAltText(t *testing.T) {
	m := testModel(t)
	m.pic = newPicture(kitty.Caps{}, kitty.Out{})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	m = next.(model)
	if len(tuitest.Drain(cmd, 0)) != 0 {
		t.Fatal("no graphics, nothing to send")
	}
	if v := m.View(); !strings.Contains(v, "그림 자리") || strings.ContainsRune(v, kitty.PlaceholderRune) {
		t.Fatalf("alt text missing:\n%s", v)
	}
	assertFits(t, m)
}
