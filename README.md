# tuikit

Small Go packages that give a standalone [Bubble Tea](https://github.com/charmbracelet/bubbletea)
app the guarantees Claude Code's mod UI engine gives a mod: one key table,
toasts that expire, panes that fail alone, overlays that keep the frame,
async results that cannot land on the wrong row, and UI state that survives a
restart.

Every type works as a value inside a Bubble Tea model: nothing aliases across
copies, and nothing captures the model in a closure.

Dependencies: bubbletea v1.3.10, lipgloss v1.1.0, charmbracelet/x/ansi,
gopkg.in/yaml.v3 (keymap overrides only). `muesli/termenv` is imported for its
profile constant; it is already lipgloss's dependency.

## Packages

| Package | Guarantees | Engine principle |
|---|---|---|
| `keys` | One `[]Binding` drives dispatch (`Resolve`), the footer, `?` help and a command palette, so help cannot drift from what keys do. Contexts resolve innermost-first; disabled bindings fall through. Footer drops low-priority hints first, keeps quit/help pinned, marks cuts with `…`, never exceeds the width. Korean 2-beolsik jamo map to QWERTY (shifted jamo to uppercase); `ResolveRaw` skips that for text fields. `Conflicts` finds duplicate keys, pinned keys shadowed by an inner context, and uppercase keys a Korean layout cannot type. User overrides from YAML; unknown actions are an error. | E3, E8 (palette), E13 |
| `toast` | Value-type queue. Newest toast per `Source` wins; cap with errors kept over successes; each non-error toast expires by its own `tea.Tick`, guarded by id so an old tick never clears a newer toast; errors stay until a key after they were on screen; `Hold`/`Release` around dialogs. Each line has a glyph or word prefix, so severity survives NO_COLOR. | E6 |
| `render` | `Truncate`/`Pad`/`FitLines` in cells, ANSI-safe. `Overlay`/`Center` splice a box over the frame without growing a line, even with a wide character on the seam. `Dim` for the frame under a modal. `Safe` turns a panicking pane into a one-line fallback plus a logged stack. `Window`/`More` keep the cursor visible and say `↑ 3 more` / `↓ 12 more`. | E4, E7, E8 |
| `async` | `Gen` drops superseded results; `Flight` adds an in-flight guard for polls. Both are plain values. | E5 |
| `prefs` | `$XDG_STATE_HOME/<app>/ui.json`; `Load` keeps defaults for missing fields and never mutates them; `SaveCmd` encodes at call time and writes temp-file-plus-rename with mode 0600, off the UI loop, reporting `Saved{Err}`. | E12 |
| `theme` | Colors by role (`Warn`, `Subtle`, `DiffAdded`…) as `AdaptiveColor`. NO_COLOR removes every color but keeps bold and reverse, so the selection stays visible. Unicode and ASCII glyph sets of equal widths, chosen by locale or `<APP>_ASCII`. | E10, E11 |

`internal/fuzzy` is the okx fzf-style scorer with smart case, used by the palette.

## Integration

```go
var km = keys.New(
	keys.Binding{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: "global", Help: "quit", Pinned: true},
	keys.Binding{Action: "help", Keys: []string{"?"}, Ctx: "global", Help: "help", Pinned: true},
	keys.Binding{Action: "open", Keys: []string{"enter"}, Ctx: "list", Help: "open", Footer: 1},
)

type model struct {
	toasts toast.Queue
	detail async.Gen
	th     theme.Theme
	w, h   int
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case toast.Expired:
		m.toasts = toast.Update(m.toasts, msg)
	case tea.KeyMsg:
		m.toasts = toast.OnKey(m.toasts)
		if b, ok := km.Resolve([]keys.Ctx{"list", "global"}, msg, nil); ok && b.Action == "open" {
			var cmd tea.Cmd
			m.toasts, cmd = toast.Push(m.toasts, toast.Toast{Source: "open", Text: "opened"})
			return m, cmd
		}
	}
	return m, nil
}

func (m model) View() string {
	body := render.Safe("list", m.w, m.h-2, nil, func() string { return "…rows…" })
	return strings.Join([]string{body,
		toast.View(m.toasts, m.w, m.th.ToastStyles()),
		km.Footer([]keys.Ctx{"list", "global"}, m.w, nil, m.th.FooterStyle())}, "\n")
}
```

And in a test:

```go
func TestKeymapHasNoConflicts(t *testing.T) {
	for _, c := range km.Conflicts([]keys.Ctx{"list", "global"}) {
		t.Error(c)
	}
}
```

`examples/demo` wires every package into a list + detail app: `go build ./examples/demo`.

## Where the code came from

| Piece | Source |
|---|---|
| CJK normalization | okx `internal/tui/keymap.go`, ccproxy `internal/keymap`, ccx `keymap_langmap.go` + CPLAT-12413 reachability test |
| fuzzy | okx `internal/fuzzy` (+ tmc smart case) |
| footer dropping | argx `view_chrome.go`, gcl `hints.go` |
| overlay splice | argx `view_chrome.go` (`spliceOverlay`, dd1c8c9) |
| truncate/pad | argx `render_util.go` |
| styles, NO_COLOR, glyphs | dayx `styles.go`, okx `styles.go`, argx `glyphs.go` |
