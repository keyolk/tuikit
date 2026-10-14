// Command demo is a small list + detail app wired through every tuikit
// package. It exists to be read and built, not to be useful.
package main

import (
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/keyolk/tuikit/async"
	"github.com/keyolk/tuikit/fuzzy"
	"github.com/keyolk/tuikit/input"
	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/list"
	"github.com/keyolk/tuikit/prefs"
	"github.com/keyolk/tuikit/render"
	"github.com/keyolk/tuikit/theme"
	"github.com/keyolk/tuikit/toast"
)

const app = "tuikit-demo"

const (
	ctxGlobal keys.Ctx = "global"
	ctxList   keys.Ctx = "list"
	ctxFilter keys.Ctx = "filter" // the filter field owns the keyboard
	ctxHelp   keys.Ctx = "help"
)

// stacks are every context stack the app can be in, innermost first. The
// conflict test and LoadOverrides both check against them.
var stacks = [][]keys.Ctx{{ctxList, ctxGlobal}, {ctxFilter, ctxGlobal}, {ctxHelp, ctxGlobal}}

var digits = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}

// keymap is the one table that drives dispatch, footer, help and palette.
func keymap() keys.Keymap {
	return keys.New(
		keys.Binding{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: ctxGlobal, Help: "quit", Pinned: true, Footer: 2},
		keys.Binding{Action: "help", Keys: []string{"?"}, Ctx: ctxGlobal, Help: "help", Pinned: true, Footer: 1},
		keys.Binding{Action: "refresh", Keys: []string{"r"}, Ctx: ctxGlobal, Group: "Data", Help: "refresh", Footer: 5},
		// j and k share one footer hint, `jk move`, which reads `jk move in
		// matches` while a filter is applied (GroupLabels follows State).
		keys.Binding{Action: "list.down", Keys: []string{"j", "down"}, Ctx: ctxList, Group: "Move", Section: "list.move",
			Help: "down", Footer: 1, FooterGroup: "move", GroupLabels: map[string]string{"filtered": "move in matches"}},
		keys.Binding{Action: "list.up", Keys: []string{"k", "up"}, Ctx: ctxList, Group: "Move", Section: "list.move",
			Help: "up", Footer: 1, FooterGroup: "move"},
		keys.Binding{Action: "list.top", Keys: []string{"g", "home"}, Ctx: ctxList, Group: "Move", Section: "list.move", Help: "first row"},
		// G alone is dead under a Korean input source; with end it is not,
		// because reachability is judged per action.
		keys.Binding{Action: "list.bottom", Keys: []string{"G", "end"}, Ctx: ctxList, Group: "Move", Section: "list.move", Help: "last row"},
		keys.Binding{Action: "list.halfdown", Keys: []string{"ctrl+d"}, Ctx: ctxList, Group: "Move", Section: "list.move", Help: "half page down", NoPalette: true},
		keys.Binding{Action: "list.halfup", Keys: []string{"ctrl+u"}, Ctx: ctxList, Group: "Move", Section: "list.move", Help: "half page up", NoPalette: true},
		// One action for nine keys: "1-9" in footer and help, KeyIndex in
		// the handler, and no nine palette entries.
		keys.Binding{Action: "list.jump", Keys: digits, KeyLabel: "1-9", Ctx: ctxList, Group: "Move", Section: "list.move",
			Help: "jump to n tenths", Short: "jump", Footer: 6, NoPalette: true},
		// The space bar shows as "space", enter as ↵ under Unicode glyphs.
		keys.Binding{Action: "list.open", Keys: []string{"enter", " "}, Ctx: ctxList, Group: "Act", Help: "open", Footer: 2},
		keys.Binding{Action: "list.filter", Keys: []string{"/"}, Ctx: ctxList, Group: "Act", Help: "filter", Footer: 3},
		keys.Binding{Action: "list.fail", Keys: []string{"x"}, Ctx: ctxList, Group: "Act", Help: "fail on purpose", Footer: 7},
		// Intentionally uppercase-only: declared, so Conflicts stays clean.
		keys.Binding{Action: "list.reverse", Keys: []string{"S"}, Ctx: ctxList, Group: "View", Help: "reverse order", LatinOnly: true},
		// One key, two meanings: the label follows Opts.State.
		keys.Binding{Action: "list.wrap", Keys: []string{"w"}, Ctx: ctxList, Group: "View", Help: "wrap detail",
			Short: "wrap", Labels: map[string]string{"on": "unwrap"}, Footer: 4},
		keys.Binding{Action: "filter.apply", Keys: []string{"enter"}, Ctx: ctxFilter, Help: "apply", Footer: 1},
		keys.Binding{Action: "filter.clear", Keys: []string{"esc"}, Ctx: ctxFilter, Help: "clear", Footer: 2},
		// While typing, q is text; quit is ctrl+c, listed first here so the
		// footer names it.
		keys.Binding{Action: "quit", Keys: []string{"ctrl+c"}, Ctx: ctxFilter, Help: "quit", Pinned: true, Footer: 2},
		// q closes help before it quits; declared, so Conflicts stays clean.
		keys.Binding{Action: "help.close", Keys: []string{"esc", "q"}, Ctx: ctxHelp, Help: "close help", Footer: 1, Shadows: []string{"quit"}},
		// Same title as the list's section, kept apart by Section.
		keys.Binding{Action: "help.down", Keys: []string{"j", "down"}, Ctx: ctxHelp, Group: "Move", Section: "help.move",
			Help: "scroll down", Footer: 2, FooterGroup: "scroll"},
		keys.Binding{Action: "help.up", Keys: []string{"k", "up"}, Ctx: ctxHelp, Group: "Move", Section: "help.move",
			Help: "scroll up", Footer: 2, FooterGroup: "scroll"},
	)
}

type item struct{ id, name string }

// ID makes item a list.Item: the cursor follows it across refreshes.
func (it item) ID() string { return it.id }

// uiState is what survives a restart (E12).
type uiState struct {
	SelID string `json:"sel_id"`
	Query string `json:"query"`
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

	all      []item
	rows     list.List[item] // the filtered view; the cursor is an ID (E2)
	reversed bool

	filter    input.Field
	filtering bool
	query     string // the applied filter

	detail    string
	wrap      bool
	detailGen async.Gen
	fetch     async.Flight
	refreshes int

	help    bool
	helpOff int
	w, h    int
}

// Init cannot change the model — its receiver is a copy and it returns only a
// command — so the first detail load is requested by message and the
// generation is bumped in Update, where the change is kept.
func (m model) Init() tea.Cmd { return func() tea.Msg { return selectMsg{} } }

func (m model) active() []keys.Ctx {
	switch {
	case m.help:
		return stacks[2]
	case m.filtering:
		return stacks[1]
	}
	return stacks[0]
}

// keyOpts is the model's view of its actions, passed to every keys call.
func (m model) keyOpts() keys.Opts {
	return keys.Opts{Enabled: m.enabled, Visible: m.offered, State: m.keyState, Label: m.liveLabel}
}

// enabled greys out refresh while one is running, so the footer stops
// offering it and the key falls through to nothing.
func (m model) enabled(action string) bool {
	return action != "refresh" || !m.fetch.Busy()
}

// offered narrows the footer without disabling anything: moving in a list of
// one row still works, it just is not worth footer width. While typing a
// filter, ? is text, so help is not offered.
func (m model) offered(action string) bool {
	switch action {
	case "list.down", "list.up":
		return m.rows.Len() > 1
	case "help", "refresh":
		return !m.filtering
	}
	return true
}

// keyState picks Binding.Labels and GroupLabels.
func (m model) keyState(action string) string {
	switch {
	case action == "list.wrap" && m.wrap:
		return "on"
	case action == "list.down" && m.query != "":
		return "filtered"
	}
	return ""
}

// liveLabel carries values a fixed Labels map cannot: the applied filter
// and how many refreshes ran.
func (m model) liveLabel(action string) (string, bool) {
	switch {
	case action == "list.filter" && m.query != "":
		return "filter: " + m.query, true
	case action == "refresh" && m.refreshes > 0:
		return fmt.Sprintf("refresh (%d)", m.refreshes), true
	}
	return "", false
}

func (m model) uiState() uiState { return uiState{SelID: m.rows.SelectedID(), Query: m.query} }

// visible is the filtered, possibly reversed, view of all.
func (m model) visible() []item {
	out := m.all
	if m.query != "" {
		names := make([]string, len(m.all))
		for i, it := range m.all {
			names[i] = it.name
		}
		out = nil
		for _, hit := range fuzzy.Filter(names, m.query) {
			out = append(out, m.all[hit.Index])
		}
	}
	if m.reversed {
		out = slices.Clone(out)
		slices.Reverse(out)
	}
	return out
}

// loadDetail bumps the generation so an older load in flight is dropped (E5).
func (m *model) loadDetail() tea.Cmd {
	var gen uint64
	m.detailGen, gen = m.detailGen.Next()
	id := m.rows.SelectedID()
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return detailMsg{gen: gen, body: "Details for " + id + "\n\n배포 상태: 정상 ✓"}
	})
}

// Update starts, in one place, the expiry tick of every toast the handlers
// added: they call toast.Add and never thread a command out.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.toasts
	m, cmd := m.update(msg)
	return m, tea.Batch(cmd, toast.Ticks(before, m.toasts))
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.rows = m.rows.SetHeight(m.listRows())
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
		m.all = msg.items
		m.rows = m.rows.SetItems(m.visible()) // the cursor stays on its item
		m.refreshes++
		// Same Source: the result replaces the Progress toast.
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "refresh", Text: "refreshed", Level: toast.Success})
		return m, m.loadDetail()
	case tea.KeyMsg:
		m.toasts = toast.OnKey(m.toasts)
		if m.filtering {
			return m.typeFilter(msg)
		}
		b, ok := m.km.Resolve(m.active(), msg, m.keyOpts())
		if !ok {
			return m, nil
		}
		return m.run(b, msg)
	}
	return m, nil
}

// typeFilter routes a key while the field owns the keyboard: the filter
// context's own keys first (raw, so a jamo is text, not a shortcut), then
// the field, then whatever outer key is left (ctrl+c).
func (m model) typeFilter(k tea.KeyMsg) (model, tea.Cmd) {
	if b, ok := m.km.ResolveRaw([]keys.Ctx{ctxFilter}, k, m.keyOpts()); ok {
		return m.run(b, k)
	}
	if f, ok := m.filter.Update(k); ok {
		m.filter = f
		return m.applyQuery(f.Value())
	}
	if b, ok := m.km.ResolveRaw(m.active(), k, m.keyOpts()); ok {
		return m.run(b, k)
	}
	return m, nil
}

// applyQuery filters live as the user types, keeping the cursor on its item
// whenever it still matches.
func (m model) applyQuery(q string) (model, tea.Cmd) {
	prev := m.rows.SelectedID()
	m.query = q
	m.rows = m.rows.SetItems(m.visible())
	var save tea.Cmd
	m.saver, save = m.saver.Save(m.uiState())
	if m.rows.SelectedID() != prev {
		return m, tea.Batch(save, m.loadDetail())
	}
	return m, save
}

func (m model) run(b keys.Binding, k tea.KeyMsg) (model, tea.Cmd) {
	var cmd tea.Cmd
	prev := m.rows.SelectedID()
	switch b.Action {
	case "quit":
		m.saver, cmd = m.saver.FlushWith(m.uiState()) // quit on its Saved{Final: true}
		return m, cmd
	case "help":
		if m.help {
			return m.run(keys.Binding{Action: "help.close"}, k)
		}
		m.help, m.helpOff = true, 0
		m.toasts = toast.Hold(m.toasts) // toasts wait while a dialog is up (E6)
	case "help.close":
		m.help = false
		// Update's toast.Ticks starts the re-armed ticks; Release's own
		// command would only duplicate them.
		m.toasts, _ = toast.Release(m.toasts)
	case "help.down":
		m.helpOff = m.clampHelp(m.helpOff + 1)
	case "help.up":
		m.helpOff = m.clampHelp(m.helpOff - 1)
	case "list.down":
		m.rows = m.rows.Move(1)
	case "list.up":
		m.rows = m.rows.Move(-1)
	case "list.top":
		m.rows = m.rows.Top()
	case "list.bottom":
		m.rows = m.rows.Bottom()
	case "list.halfdown":
		m.rows = m.rows.HalfDown()
	case "list.halfup":
		m.rows = m.rows.HalfUp()
	case "list.jump":
		// KeyIndex is the position of the pressed key in b.Keys: 0 for 1.
		n := b.KeyIndex(k) + 1
		m.rows = m.rows.SelectIndex(n * m.rows.Len() / 10)
	case "list.reverse":
		m.reversed = !m.reversed
		m.rows = m.rows.SetItems(m.visible())
	case "list.filter":
		m.filtering = true
		m.filter = input.New(m.query)
	case "filter.apply":
		m.filtering = false
	case "filter.clear":
		m.filtering = false
		m.filter = input.Field{}
		return m.applyQuery("")
	case "list.open":
		// The failure it reported is over; Dismiss clears even a sticky error.
		m.toasts = toast.Dismiss(m.toasts, "fail")
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "open", Text: "opened " + m.rows.SelectedID(), Level: toast.Info})
	case "list.fail":
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "fail", Text: "could not open " + m.rows.SelectedID(), Level: toast.Error})
	case "list.wrap":
		m.wrap = !m.wrap
	case "refresh":
		f, id, ok := m.fetch.Start()
		if !ok {
			return m, nil
		}
		m.fetch = f
		// Progress has no TTL: the "refreshed" result replaces it.
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "refresh", Text: "refreshing…", Level: toast.Progress})
		items := slices.Clone(m.all)
		cmd = tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return fetchedMsg{id: id, items: items} })
	}
	if m.rows.SelectedID() != prev {
		// Saved debounced: holding j writes once, after the keys stop.
		var save tea.Cmd
		m.saver, save = m.saver.Save(m.uiState())
		cmd = tea.Batch(cmd, m.loadDetail(), save)
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

// listRows is the rows the list draws items in, after its More line.
func (m model) listRows() int {
	return m.bodyHeight(toast.View(m.toasts, m.w, m.th.ToastStyles())) - 1
}

func (m model) View() string {
	if m.w < 40 || m.h < 8 {
		return render.FitLines("terminal too small", m.w, m.h)
	}
	fs := m.th.FooterStyle()
	if n := m.rows.Len(); n > 0 && !m.help {
		// Right is kept free of hints: they drop first.
		fs.Right = m.th.Muted.Render(render.PadLeft(fmt.Sprintf("%d/%d", m.rows.Index()+1, n), 7))
	}
	footer := m.km.Footer(m.active(), m.w, m.keyOpts(), fs)
	toasts := toast.View(m.toasts, m.w, m.th.ToastStyles())
	bodyH := m.bodyHeight(toasts)
	listW := min(30, m.w/2)

	list := render.Safe("list", listW, bodyH, m.logf, func() string { return m.viewList(listW, bodyH) })
	detail := render.Safe("detail", m.w-listW-1, bodyH, m.logf, func() string {
		body := m.detail
		if m.wrap {
			body = lipgloss.NewStyle().Width(m.w - listW - 1).Render(body)
		}
		if rows := m.chartRows(m.w - listW - 1); rows != "" {
			body += "\n\n" + rows
		}
		return render.FitLines(body, m.w-listW-1, bodyH)
	})
	title := m.th.Title.Render(app)
	if m.filtering {
		title += "  / " + m.filter.View(m.w-len(app)-4, true, input.Style{Placeholder: m.th.Subtle})
	}
	parts := []string{
		render.Truncate(title, m.w),
		lipgloss.JoinHorizontal(lipgloss.Top, render.FitLines(list, listW, bodyH), " ", detail),
	}
	if toasts != "" {
		parts = append(parts, toasts)
	}
	frame := strings.Join(append(parts, footer), "\n")
	if !m.help {
		return frame
	}
	boxW, boxH := m.helpBox()
	cw, ch := render.BoxContent(boxW, boxH, m.th.BoxFocus)
	// View cannot store the clamped offset (its receiver is a copy), so
	// Update clamps m.helpOff against HelpLineCount (clampHelp).
	lines, _ := keys.RenderHelp(m.helpSections(), cw, ch, m.helpOff, m.th.HelpStyle())
	box := render.Box("keys", strings.Join(lines, "\n"), boxW, boxH, m.th.BoxFocus)
	return render.Center(render.Dim(frame), box, m.w, m.h) // over, not instead of (E8)
}

func (m model) helpBox() (w, h int) { return min(50, m.w-4), min(16, m.h-4) }

// helpSections documents the list's keys even while help is the active
// context, plus a prose note.
func (m model) helpSections() []keys.Section {
	secs := m.km.Help(m.keyOpts(), stacks[0], stacks[2])
	return append(secs, keys.Section{Title: "Notes", Notes: []string{
		"The cursor follows its item across refresh, filter and reverse. " +
			"State is saved to " + prefs.Path(app) + ".",
	}})
}

// clampHelp bounds the offset by the laid-out line count, so holding j
// does not grow it past the last page.
func (m model) clampHelp(off int) int {
	w, h := m.helpBox()
	cw, ch := render.BoxContent(w, h, m.th.BoxFocus)
	return max(0, min(off, keys.HelpLineCount(m.helpSections(), cw)-ch))
}

func (m model) viewList(w, h int) string {
	if m.rows.Len() == 0 {
		esc, _ := m.km.KeyFor(stacks[1], "filter.clear", m.keyOpts())
		return m.th.Subtle.Render(render.Truncate("no match — / then "+m.th.KeyNames().Name(esc)+" clears", w))
	}
	start, end := m.rows.Window()
	cur := m.rows.Index()
	var lines []string
	for i := start; i < end; i++ {
		it := m.rows.At(i)
		line := "  " + it.name
		if i == cur {
			line = m.th.Selected.Render(render.Pad(m.th.Glyphs.Cursor+" "+it.name, w))
		}
		lines = append(lines, render.Truncate(line, w))
	}
	return strings.Join(append(lines, m.th.Subtle.Render(m.rows.MoreLine())), "\n")
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
		items = append(items, item{id: fmt.Sprintf("svc-%02d", i), name: fmt.Sprintf("서비스 %02d service", i)})
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
		all:    items,
		query:  saved.Query,
		toasts: toast.New(3),
	}
	m.rows = list.New(m.visible())
	m.rows, _ = m.rows.Select(saved.SelID) // false: the saved row is gone; keep the first
	if kerr != nil {
		// Errors are sticky, so there is no tick to run.
		m.toasts = toast.Add(m.toasts, toast.Toast{Source: "keys", Text: "keys.yaml ignored: " + kerr.Error(), Level: toast.Error})
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
