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
| `keys` | One `[]Binding` drives dispatch (`Resolve`), the footer, `?` help and a command palette, so help cannot drift from what keys do. Contexts resolve innermost-first; disabled bindings fall through. One `Opts{Enabled, Visible, State}` goes to every call: `Enabled` gates dispatch, `Visible` narrows the footer only, `State` picks a `Labels` entry so one key can read "group" or "ungroup". An action bound in several contexts is one entry (pinned if any binding is). Footer drops low-priority hints first, keeps quit/help pinned, marks cuts with `…`, collapses `FooterGroup` members into one hint (`hjkl move`), uses `Short` labels, never exceeds the width. `Names` displays keys (`" "` → `space`, `enter` → `↵` with glyphs, optional `^x`). `Help(opts, stacks...)` merges several stacks, `Table` lists every context, `RenderHelp` takes a `HelpStyle` (optional action column) and returns the clamped offset. Korean 2-beolsik jamo map to QWERTY; `ResolveRaw` skips that for text fields. `Conflicts` finds duplicate keys, pinned keys shadowed by an inner context (unless the inner binding declares `Shadows`), and uppercase keys a Korean layout cannot type. Overrides from YAML (`ConfigPath`), plain `action:` or per-context `ctx.action:`; unknown names, new duplicates and new pinned shadows are errors. The zero `Keymap` is safe (`Or`, `IsZero`, `Must`). | E3, E8 (palette), E13 |
| `toast` | Value-type queue. Newest toast per `Source` wins; cap with errors kept over successes; each non-error toast expires by its own `tea.Tick`, guarded by id so an old tick never clears a newer toast; errors stay until a key after they were on screen; `Dismiss` by source; `Hold`/`Release` around dialogs. Each line has a glyph or word prefix, so severity survives NO_COLOR. Tests use `WithClock`, `Queue.ExpiredMsg` and `ExpireAll` instead of sleeping. | E6 |
| `render` | `Truncate`/`Pad`/`FitLines` in cells, ANSI-safe. `Overlay`/`Center` splice a box over the frame without growing a line, even with a wide character on the seam. `Dim` for the frame under a modal. `Safe` turns a panicking pane into a one-line fallback plus a logged stack. `Window`/`More` keep the cursor visible and say `↑ 3 more` / `↓ 12 more`. | E4, E7, E8 |
| `async` | `Gen` drops superseded results; `Flight` adds an in-flight guard for polls. Both are plain values. | E5 |
| `prefs` | `$XDG_STATE_HOME/<app>/ui.json`; `Load` keeps defaults for missing fields and never mutates them; writes encode at call time and go temp-file-plus-rename with mode 0600, off the UI loop, reporting `Saved{Err}`. Every write carries a sequence number, so an older write never lands after a newer one. `Saver[T]` debounces bursts and `Flush` writes the latest state for quit (`Saved{Final: true}`); `SaveCmd` is the one-shot form. | E12 |
| `theme` | Colors by role (`Warn`, `Subtle`, `DiffAdded`…) as `AdaptiveColor`. NO_COLOR removes every color but keeps bold and reverse, so the selection stays visible. Unicode and ASCII glyph sets of equal widths, chosen by locale or `<APP>_ASCII`. | E10, E11 |

`internal/fuzzy` is the okx fzf-style scorer with smart case, used by the palette.

## Integration

```go
var stacks = [][]keys.Ctx{{"list", "global"}, {"detail", "list", "global"}}

var km = keys.New(
	keys.Binding{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: "global", Help: "quit", Pinned: true},
	keys.Binding{Action: "help", Keys: []string{"?"}, Ctx: "global", Help: "help", Pinned: true},
	keys.Binding{Action: "down", Keys: []string{"j", "down"}, Ctx: "list", Help: "down", Footer: 2, FooterGroup: "move"},
	keys.Binding{Action: "up", Keys: []string{"k", "up"}, Ctx: "list", Help: "up", Footer: 2, FooterGroup: "move"},
	keys.Binding{Action: "open", Keys: []string{"enter", " "}, Ctx: "list", Help: "open", Footer: 1},
	keys.Binding{Action: "group", Keys: []string{"t"}, Ctx: "list", Help: "group",
		Labels: map[string]string{"on": "ungroup"}, Footer: 3},
	keys.Binding{Action: "detail.close", Keys: []string{"esc", "q"}, Ctx: "detail", Help: "close",
		Footer: 1, Shadows: []string{"quit"}}, // q closes the pane first, on purpose
)

type model struct {
	km      keys.Keymap // by value; the zero Keymap is safe
	toasts  toast.Queue
	saver   prefs.Saver[uiState]
	th      theme.Theme
	items   []string
	grouped bool
	w, h    int
}

func (m model) keyOpts() keys.Opts {
	return keys.Opts{
		// Enabled: nil runs everything; Visible narrows the footer only.
		Visible: func(a string) bool { return a != "down" && a != "up" || len(m.items) > 1 },
		State: func(a string) string {
			if a == "group" && m.grouped {
				return "on"
			}
			return ""
		},
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case toast.Expired:
		m.toasts = toast.Update(m.toasts, msg)
	case prefs.SaveTick:
		m.saver, cmd = m.saver.Update(msg)
	case prefs.Saved:
		if msg.Final {
			return m, tea.Quit
		}
	case tea.KeyMsg:
		m.toasts = toast.OnKey(m.toasts)
		b, ok := m.km.Resolve(m.active(), msg, m.keyOpts())
		switch {
		case !ok:
		case b.Action == "quit":
			m.saver, cmd = m.saver.Flush() // quit once the state is on disk
		case b.Action == "group":
			m.grouped = !m.grouped
			m.saver, cmd = m.saver.Save(m.uiState()) // debounced
		}
	}
	return m, cmd
}

func (m model) View() string {
	return strings.Join([]string{
		render.Safe("list", m.w, m.h-2, nil, func() string { return "…rows…" }),
		toast.View(m.toasts, m.w, m.th.ToastStyles()),
		m.km.Footer(m.active(), m.w, m.keyOpts(), m.th.FooterStyle()), // "jk move  space open  t group …"
	}, "\n")
}
```

At startup, user overrides are checked against the same stacks:

```go
km, err := km.LoadOverrides(keys.ConfigPath("myapp"), stacks...) // keys.yaml: {quit: [x], list.open: [o]}
```

And in a test — no filtering, intended shadows are declared on the binding:

```go
func TestKeymapHasNoConflicts(t *testing.T) {
	for _, c := range km.Conflicts(stacks...) {
		t.Error(c)
	}
}
```

Help pages pass the current stack first plus any other worth documenting,
and store the offset `RenderHelp` clamps:

```go
lines, m.helpOff = keys.RenderHelp(km.Help(m.keyOpts(), m.active(), stacks[1]), w, h, m.helpOff, m.th.HelpStyle())
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
