// Command demo is a small list + detail app wired through every tuikit
// package. It exists to be read and built, not to be useful.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/async"
	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/prefs"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/toast"
)

const app = "tuikit-demo"

const (
	ctxGlobal keys.Ctx = "global"
	ctxList   keys.Ctx = "list"
	ctxHelp   keys.Ctx = "help"
)

// stacks are every context stack the app can be in, innermost first. The
// conflict test and LoadOverrides both check against them.
var stacks = [][]keys.Ctx{{ctxList, ctxGlobal}, {ctxHelp, ctxGlobal}}

// keymap is the one table that drives dispatch, footer, help and palette.
func keymap() keys.Keymap {
	return keys.New(
		keys.Binding{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: ctxGlobal, Help: "quit", Pinned: true, Footer: 2},
		keys.Binding{Action: "help", Keys: []string{"?"}, Ctx: ctxGlobal, Help: "help", Pinned: true, Footer: 1},
		keys.Binding{Action: "refresh", Keys: []string{"r"}, Ctx: ctxGlobal, Group: "Data", Help: "refresh", Footer: 4},
		// j and k share one footer hint: `jk move`.
		keys.Binding{Action: "list.down", Keys: []string{"j", "down"}, Ctx: ctxList, Group: "Move", Help: "down", Footer: 1, FooterGroup: "move"},
		keys.Binding{Action: "list.up", Keys: []string{"k", "up"}, Ctx: ctxList, Group: "Move", Help: "up", Footer: 1, FooterGroup: "move"},
		// The space bar shows as "space", enter as ↵ under Unicode glyphs.
		keys.Binding{Action: "list.open", Keys: []string{"enter", " "}, Ctx: ctxList, Group: "Act", Help: "open", Footer: 2},
		keys.Binding{Action: "list.fail", Keys: []string{"x"}, Ctx: ctxList, Group: "Act", Help: "fail on purpose", Footer: 5},
		// One key, two meanings: the label follows Opts.State.
		keys.Binding{Action: "list.wrap", Keys: []string{"w"}, Ctx: ctxList, Group: "View", Help: "wrap detail",
			Short: "wrap", Labels: map[string]string{"on": "unwrap"}, Footer: 3},
		// q closes help before it quits; declared, so Conflicts stays clean.
		keys.Binding{Action: "help.close", Keys: []string{"esc", "q"}, Ctx: ctxHelp, Help: "close help", Footer: 1, Shadows: []string{"quit"}},
		keys.Binding{Action: "help.down", Keys: []string{"j", "down"}, Ctx: ctxHelp, Help: "scroll down", Footer: 2, FooterGroup: "scroll"},
		keys.Binding{Action: "help.up", Keys: []string{"k", "up"}, Ctx: ctxHelp, Help: "scroll up", Footer: 2, FooterGroup: "scroll"},
	)
}

type item struct{ ID, Name string }

// uiState is what survives a restart (E12).
type uiState struct {
	SelID string `json:"sel_id"`
}

type (
	detailMsg struct {
		gen  uint64
		body string
	}
	selectMsg  struct{}
	fetchedMsg struct {
		id    uint64
		items []item
	}
)

type model struct {
	km     keys.Keymap // by value: the zero Keymap is safe, see keymap()
	th     theme.Theme
	toasts toast.Queue
	saver  prefs.Saver[uiState]
	logf   func(string, ...any)

	items  []item
	selID  string // the cursor is an ID (E2); the row is derived
	offset int

	detail    string
	wrap      bool
	detailGen async.Gen
	fetch     async.Flight

	help    bool
	helpOff int
	w, h    int
}

// Init cannot change the model — its receiver is a copy and it returns only a
// command — so the first detail load is requested by message and the
// generation is bumped in Update, where the change is kept.
func (m model) Init() tea.Cmd { return func() tea.Msg { return selectMsg{} } }

func (m model) row() int {
	for i, it := range m.items {
		if it.ID == m.selID {
			return i
		}
	}
	return 0
}

func (m model) active() []keys.Ctx {
	if m.help {
		return []keys.Ctx{ctxHelp, ctxGlobal}
	}
	return []keys.Ctx{ctxList, ctxGlobal}
}

// keyOpts is the model's view of its actions, passed to every keys call.
func (m model) keyOpts() keys.Opts {
	return keys.Opts{Enabled: m.enabled, Visible: m.offered, State: m.keyState}
}

// enabled greys out refresh while one is running, so the footer stops
// offering it and the key falls through to nothing.
func (m model) enabled(action string) bool {
	return action != "refresh" || !m.fetch.Busy()
}

// offered narrows the footer without disabling anything: moving in a list of
// one row still works, it just is not worth footer width.
func (m model) offered(action string) bool {
	switch action {
	case "list.down", "list.up":
		return len(m.items) > 1
	}
	return true
}

// keyState picks Binding.Labels: w says "unwrap" while wrapped.
func (m model) keyState(action string) string {
	if action == "list.wrap" && m.wrap {
		return "on"
	}
	return ""
}

// loadDetail bumps the generation so an older load in flight is dropped (E5).
func (m *model) loadDetail() tea.Cmd {
	var gen uint64
	m.detailGen, gen = m.detailGen.Next()
	id := m.selID
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return detailMsg{gen: gen, body: "Details for " + id + "\n\n배포 상태: 정상 ✓"}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case toast.Expired:
		m.toasts = toast.Update(m.toasts, msg)
	case prefs.SaveTick:
		var cmd tea.Cmd
		m.saver, cmd = m.saver.Update(msg)
		return m, cmd
	case prefs.Saved:
		if msg.Err != nil {
			m.logf("save prefs: %v", msg.Err)
		}
		if msg.Final {
			return m, tea.Quit // the flushed state is on disk
		}
	case selectMsg:
		return m, m.loadDetail()
	case detailMsg:
		if m.detailGen.Is(msg.gen) {
			m.detail = msg.body
		}
	case fetchedMsg:
		m.fetch = m.fetch.Done(msg.id)
		if !m.fetch.Is(msg.id) {
			return m, nil
		}
		m.items = msg.items
		var cmd tea.Cmd
		m.toasts, cmd = toast.Push(m.toasts, toast.Toast{Source: "refresh", Text: "refreshed", Level: toast.Success})
		return m, tea.Batch(cmd, m.loadDetail())
	case tea.KeyMsg:
		m.toasts = toast.OnKey(m.toasts)
		b, ok := m.km.Resolve(m.active(), msg, m.keyOpts())
		if !ok {
			return m, nil
		}
		return m.run(b.Action)
	}
	return m, nil
}

func (m model) run(action string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch action {
	case "quit":
		m.saver, cmd = m.saver.Flush() // quit on its Saved{Final: true}
	case "help":
		if m.help {
			return m.run("help.close")
		}
		m.help, m.helpOff = true, 0
		m.toasts = toast.Hold(m.toasts) // toasts wait while a dialog is up (E6)
	case "help.close":
		m.help = false
		m.toasts, cmd = toast.Release(m.toasts)
	case "help.down":
		m.helpOff++
		m.helpOff = m.clampHelp()
	case "help.up":
		m.helpOff = max(0, m.helpOff-1)
	case "list.down", "list.up":
		r := m.row() + 1
		if action == "list.up" {
			r = m.row() - 1
		}
		if r >= 0 && r < len(m.items) {
			m.selID = m.items[r].ID
			// The scroll offset is state, so it is kept here, not in View (E1).
			_, _, m.offset = render.Window(len(m.items), m.listRows(), r, m.offset)
			// Saved debounced: holding j writes once, after the keys stop.
			var save tea.Cmd
			m.saver, save = m.saver.Save(uiState{SelID: m.selID})
			cmd = tea.Batch(m.loadDetail(), save)
		}
	case "list.open":
		// The failure it reported is over; Dismiss clears even a sticky error.
		m.toasts = toast.Dismiss(m.toasts, "fail")
		m.toasts, cmd = toast.Push(m.toasts, toast.Toast{Source: "open", Text: "opened " + m.selID, Level: toast.Info})
	case "list.fail":
		m.toasts, cmd = toast.Push(m.toasts, toast.Toast{Source: "fail", Text: "could not open " + m.selID, Level: toast.Error})
	case "list.wrap":
		m.wrap = !m.wrap
	case "refresh":
		f, id, ok := m.fetch.Start()
		if !ok {
			return m, nil
		}
		m.fetch = f
		items := append([]item(nil), m.items...)
		cmd = tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return fetchedMsg{id: id, items: items} })
	}
	return m, cmd
}

// bodyHeight is the height left for the panes once the title, toasts and
// footer are placed; computed in one place so View and Update agree (E9).
func (m model) bodyHeight(toasts string) int {
	h := m.h - 2 // title + footer
	if toasts != "" {
		h -= lipgloss.Height(toasts)
	}
	return h
}

// listRows is the rows the list draws items in, after its two More lines.
func (m model) listRows() int {
	return m.bodyHeight(toast.View(m.toasts, m.w, m.th.ToastStyles())) - 2
}

func (m model) View() string {
	if m.w < 40 || m.h < 8 {
		return render.FitLines("terminal too small", m.w, m.h)
	}
	footer := m.km.Footer(m.active(), m.w, m.keyOpts(), m.th.FooterStyle())
	toasts := toast.View(m.toasts, m.w, m.th.ToastStyles())
	bodyH := m.bodyHeight(toasts)
	listW := min(30, m.w/2)

	list := render.Safe("list", listW, bodyH, m.logf, func() string { return m.viewList(listW, bodyH) })
	detail := render.Safe("detail", m.w-listW-1, bodyH, m.logf, func() string {
		body := m.detail
		if m.wrap {
			body = lipgloss.NewStyle().Width(m.w - listW - 1).Render(body)
		}
		return render.FitLines(body, m.w-listW-1, bodyH)
	})
	parts := []string{
		m.th.Title.Render(render.Truncate(app, m.w)),
		lipgloss.JoinHorizontal(lipgloss.Top, render.FitLines(list, listW, bodyH), " ", detail),
	}
	if toasts != "" {
		parts = append(parts, toasts)
	}
	frame := strings.Join(append(parts, footer), "\n")
	if !m.help {
		return frame
	}
	boxW, boxH := min(50, m.w-4), min(16, m.h-4)
	// View cannot store the clamped offset (its receiver is a copy), so
	// Update clamps m.helpOff with the same call (clampHelp).
	lines, _ := keys.RenderHelp(m.helpSections(), boxW-4, boxH-2, m.helpOff, m.th.HelpStyle())
	box := m.th.BoxFocus.Render(strings.Join(lines, "\n"))
	return render.Center(render.Dim(frame), box, m.w, m.h) // over, not instead of (E8)
}

// helpSections documents the list's keys even while help is the active
// context: Help merges the stacks and lists each action once.
func (m model) helpSections() []keys.Section {
	return m.km.Help(m.keyOpts(), []keys.Ctx{ctxList, ctxGlobal}, []keys.Ctx{ctxHelp})
}

// clampHelp stores the offset RenderHelp would draw, so holding j does not
// grow it past the last page.
func (m model) clampHelp() int {
	boxW, boxH := min(50, m.w-4), min(16, m.h-4)
	_, off := keys.RenderHelp(m.helpSections(), boxW-4, boxH-2, m.helpOff, keys.HelpStyle{})
	return off
}

func (m model) viewList(w, h int) string {
	cur := m.row()
	start, end, _ := render.Window(len(m.items), h-2, cur, m.offset)
	top, bottom := render.More(start, len(m.items)-end, h)
	lines := []string{m.th.Subtle.Render(top)}
	for i := start; i < end; i++ {
		line := "  " + m.items[i].Name
		if i == cur {
			line = m.th.Selected.Render(render.Pad(m.th.Glyphs.Cursor+" "+m.items[i].Name, w))
		}
		lines = append(lines, render.Truncate(line, w))
	}
	return strings.Join(append(lines, m.th.Subtle.Render(bottom)), "\n")
}

func main() {
	f, err := os.CreateTemp("", app+"-*.log")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	logger := log.New(f, "", log.LstdFlags)

	saved, err := prefs.Load(app, uiState{SelID: "svc-01"})
	if err != nil {
		logger.Printf("load prefs: %v", err)
	}
	var items []item
	for i := 1; i <= 40; i++ {
		items = append(items, item{ID: fmt.Sprintf("svc-%02d", i), Name: fmt.Sprintf("서비스 %02d service", i)})
	}
	// A bad keys.yaml must not stop the app: keep the defaults and say why.
	km, kerr := keymap().LoadOverrides(keys.ConfigPath(app), stacks...)
	if kerr != nil {
		logger.Printf("keys: %v", kerr)
	}
	m := model{
		km:     km,
		saver:  prefs.NewSaver[uiState](app, 0),
		th:     theme.NewWith(theme.NewRenderer(os.Stdout), theme.DefaultRoles(), theme.DetectGlyphs(app)),
		logf:   logger.Printf,
		items:  items,
		selID:  saved.SelID,
		toasts: toast.New(3),
	}
	if kerr != nil {
		// Errors are sticky, so there is no tick to run.
		m.toasts, _ = toast.Push(m.toasts, toast.Toast{Source: "keys", Text: "keys.yaml ignored: " + kerr.Error(), Level: toast.Error})
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
