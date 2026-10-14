# tuikit

Small Go packages that give a standalone [Bubble Tea](https://github.com/charmbracelet/bubbletea)
app the guarantees Claude Code's mod UI engine gives a mod: one key table,
toasts that expire, panes that fail alone, overlays that keep the frame,
async results that cannot land on the wrong row, and UI state that survives a
restart.

Every type works as a value inside a Bubble Tea model: nothing aliases across
copies, and nothing captures the model in a closure.

Dependencies: bubbletea v1.3.10, lipgloss v1.1.0, charmbracelet/x/ansi,
gopkg.in/yaml.v3 (keymap overrides only), golang.org/x/sys (`kitty` cell size only). `muesli/termenv` is imported for its
profile constant; it is already lipgloss's dependency.

## Packages

| Package | Guarantees | Engine principle |
|---|---|---|
| `keys` | One `[]Binding` drives dispatch (`Resolve`), the footer, `?` help and a command palette, so help cannot drift from what keys do. Contexts resolve innermost-first; disabled bindings fall through; `ResolveAll` lists every candidate so a handler can decline. One `Opts{Enabled, Visible, State, Label}` goes to every call: `Enabled` gates dispatch, `Visible` narrows the footer only, `State` picks a `Labels` entry so one key can read "group" or "ungroup" (and a `GroupLabels` entry for a `FooterGroup`), `Label` is a per-call label for live values ("3 scheduled"). An action bound in several contexts is one entry (pinned if any binding is). Footer drops low-priority hints first, keeps quit/help pinned, marks cuts with an `Ellipsis` marker that never pushes pinned hints off, reserves `Right` text (a scroll position), collapses `FooterGroup` members into one hint (`hjkl move`), uses `Short` labels and `KeyLabel` ("1-9"), never exceeds the width. `KeyIndex` says which of a binding's keys fired; `KeyFor` names the key that runs an action now (overrides and shadows included). `Names` displays keys (`" "` → `space`, `enter` → `↵` with glyphs, optional `^x`). `Help(opts, stacks...)` merges several stacks into sections keyed by `Section` (else `Group`), `Table` lists every context, `RenderHelp` draws entries and word-wrapped `Notes` and returns the clamped offset, `HelpLineCount` counts the lines. `NoPalette` keeps a binding out of `Entries`. Korean 2-beolsik jamo map to QWERTY; `ResolveRaw` skips that for text fields and `TypedText` turns a key into typed text. `Conflicts` finds duplicate keys, pinned keys shadowed by an inner context (unless the inner binding declares `Shadows`), and actions none of whose keys a Korean layout can type (`LatinOnly` opts out). Overrides from YAML (`ConfigPath(app, sub...)`), plain `action:` or per-context `ctx.action:`; unknown names, new duplicates and new pinned shadows are errors; `ReadOverrides` + `Known` split one file across tables. The zero `Keymap` is safe (`Or`, `IsZero`, `Must`). | E3, E8 (palette), E13 |
| `list` | `List[T Item]`: a cursor held by ID over items with an `ID()`. `SetItems` reselects by ID after a refresh (or stays on the row when the item is gone, and remembers the ID through an empty filter); `Move`, `Top`/`Bottom`, `HalfDown`/`HalfUp`, `PageDown`/`PageUp`; `SetHeight` keeps the scroll offset in the value, `Window`/`Hidden`/`MoreLine` feed View. | E2, E7 |
| `input` | `Field`: a one-line text field as a value. Typed text, batched runes and pastes (newlines become spaces), Korean as typed, backspace/delete, `ctrl+u`/`ctrl+k`/`ctrl+w`, rune and word moves, home/end, optional `Limit` and `Placeholder`. `Update` reports whether it used the key; `View` fits exactly `width` cells, keeps the cursor visible without a stored offset and never splits a wide character. | E13 |
| `toast` | Value-type queue. Newest toast per `Source` wins; cap with errors and progress kept over successes; each non-sticky toast expires by its own `tea.Tick`, guarded by id so an old tick never clears a newer toast; errors stay until a key after they were on screen; `Progress` stays until its `Source` pushes the result or is dismissed; `Dismiss` by source; `Hold`/`Release` around dialogs. `Add` + `Ticks(old, new)` let helpers deep in Update add toasts without threading commands. `RenderItem` draws one toast inline. Each line has a glyph or word prefix, so severity survives NO_COLOR. Tests use `WithClock`, `Queue.ExpiredMsg` and `ExpireAll` instead of sleeping. | E6 |
| `render` | `Truncate`/`Pad`/`FitLines` in cells, ANSI-safe, with an ASCII fast path; `TruncateMarker` takes the marker; `PadRight`/`PadLeft` never truncate. `Ellipsis()` is `…`, or `~` under a CJK locale where `…` draws two cells. `Box`/`BoxContent` draw an exact w×h box with a title and say how much room the body has. `Overlay`/`Center` splice a box over the frame without growing a line, even with a wide character on the seam. `Dim` for the frame under a modal. `Safe` turns a panicking pane into a one-line fallback plus a logged stack. `Window`/`More`/`MoreLine` keep the cursor visible and say `↑ 3 more` / `↓ 12 more`. | E4, E7, E8 |
| `async` | `Gen` drops superseded results; `Flight` adds an in-flight guard for polls. Both are plain values. | E5 |
| `prefs` | `$XDG_STATE_HOME/<app>/ui.json`; `Load` keeps defaults for missing fields and never mutates them; writes encode at call time and go temp-file-plus-rename with mode 0600, off the UI loop, reporting `Saved{Err}`. Every write carries a sequence number, so an older write never lands after a newer one. `Saver[T]` debounces bursts; `Flush`/`FlushWith(v)` write the latest state for quit (`Saved{Final: true}`); `SaveCmd` is the one-shot form. The zero `Saver` and app `""` persist nothing. | E12 |
| `theme` | Colors by role (`Warn`, `Subtle`, `DiffAdded`…) as `AdaptiveColor`. NO_COLOR removes every color but keeps bold and reverse, so the selection stays visible. Unicode and ASCII glyph sets of equal widths (spinner frames, `Bar` glyphs included), chosen by locale or `<APP>_ASCII`; a CJK locale gets an ASCII `Ellipsis`. The zero `Theme` draws plain words. | E10, E11 |
| `kitty` | Kitty graphics (Kitty, Ghostty, WezTerm, also behind tmux) with no package state. `Detect(env, tmux)` returns `Caps` (`Supported`, `Terminal`, `InTmux`, `NeedsPassthrough`, `PassthroughMode`, `Pane`) from an injected env and `TmuxQuerier`; the app calls it once in main and keeps the value. `EnsurePassthroughCmd` sets tmux `allow-passthrough=all` off the loop and returns its error. Escape builders are `Caps` methods, so tmux wrapping (ESC doubled, one frame per 4096-byte chunk) follows the value: `Transmit`/`TransmitImage`/`TransmitFile`/`TransmitShared`, `Place` (z-index, virtual `U=1`, cursor-saved `MoveTo` with pane offsets), `Delete`, `DeleteAll`. `Placeholder`/`PlaceholderAt` give U+10EEEE cells with the id in the foreground color and the row mark on the first cell only, so the image scrolls and clips with text inside tmux. `Fit` sizes a box by the cell's pixels (`CellSizeCmd`, `CellSizeQuery` + `ParseCellSize`); `KeyboardQuery` + `ParseKeyboardFlags` detect CSI u. `Image{ID, Source, Cols, Rows, Alt}`: `View` is exactly Cols×Rows cells, placeholders or the alt text; `Cmds(prev, …)` transmits on change and deletes on hide, encoding (shrunk to the box) inside the command; writes go through `Out`, one lock shared by copies. `QueryPane`/`WatchPane` run the tmux visibility poll as a command that is silent until something changes. | E1, E5, E9 |
| `fuzzy` | The okx fzf-style scorer with smart case (`Score`, `Filter`, `Match`), used by the palette and public for app filters. | — |
| `tuitest` | `Drain(cmd, wait)` runs a command batch or sequence and returns its messages without waiting on toast or saver timers. | — |

## Cell width: one rule

Every width in the kit (`render.Width`, `Truncate`, `Pad`, footer and help layout)
counts cells the way lipgloss does, because lipgloss is what Bubble Tea renders with.
Under a CJK locale `go-runewidth` counts East Asian *ambiguous* characters (`—`, `…`,
`·`) as two cells, lipgloss and the kit count them as one. Mixing the two in one app
misaligns columns, so:

- measure with `render.Width` (or `lipgloss.Width`), never `runewidth.StringWidth`
  directly in an app or its tests;
- keep ambiguous-width glyphs out of aligned columns — the terminal, not the locale,
  decides how many cells it actually draws for them. That is why `render.Ellipsis()`
  is `~` under a CJK locale and why the glyph sets avoid ambiguous characters.

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
			m.saver, cmd = m.saver.FlushWith(m.uiState()) // quit once the state is on disk
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

### Recipes

Uppercase keys: reachability is judged per action, so `G` with an `end`
alias passes `Conflicts`; a key meant to be Latin-only says so.

```go
keys.Binding{Action: "bottom", Keys: []string{"G", "end"}, Ctx: "list", Help: "last row"},
keys.Binding{Action: "reverse", Keys: []string{"S"}, Ctx: "list", Help: "reverse", LatinOnly: true},
```

One action for a key range, shown compactly, out of the palette:

```go
keys.Binding{Action: "tab", Keys: []string{"1", "2", "3", "4"}, KeyLabel: "1-4", Ctx: "list", Help: "tab", Footer: 2, NoPalette: true}
// in Update: tab := b.KeyIndex(msg) // 0 for "1"
```

Labels that follow the model, per call:

```go
keys.Binding{Action: "prev", Keys: []string{"h"}, FooterGroup: "day", GroupLabels: map[string]string{"week": "week"}, ...}
opts := keys.Opts{
	State: func(a string) string { if m.weekView { return "week" }; return "" },
	Label: func(a string) (string, bool) {
		if a == "sched" { return fmt.Sprintf("%d scheduled", m.n), true }
		return "", false
	},
}
k, ok := km.KeyFor(m.active(), "refresh", opts) // the key that runs it now; display with Names.Name
```

Footer with a reserved right side, help in a box with notes:

```go
st := m.th.FooterStyle()        // Ellipsis follows the locale: ~ under ko/ja/zh
st.Right = render.PadLeft(fmt.Sprintf("%d/%d", m.rows.Index()+1, m.rows.Len()), 7)
footer := km.Footer(m.active(), m.w, opts, st)

secs := append(km.Help(opts, m.active()), keys.Section{Title: "Notes", Notes: []string{"Saved to " + prefs.Path(app)}})
cw, ch := render.BoxContent(boxW, boxH, m.th.BoxFocus)
lines, _ := keys.RenderHelp(secs, cw, ch, m.helpOff, m.th.HelpStyle())
box := render.Box("keys", strings.Join(lines, "\n"), boxW, boxH, m.th.BoxFocus)
m.helpOff = max(0, min(m.helpOff, keys.HelpLineCount(secs, cw)-ch)) // in Update
```

A filter field and an ID-anchored list:

```go
case tea.KeyMsg:
	if m.filtering {
		if f, ok := m.filter.Update(msg); ok { // input.Field
			m.filter = f
			m.rows = m.rows.SetItems(match(m.all, f.Value())) // list.List keeps the cursor on its item
			return m, nil
		}
	}
// View
start, end := m.rows.Window()
more := m.rows.MoreLine()
```

Toasts from deep helpers, and a task that reports progress:

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.toasts
	m, cmd := m.update(msg) // helpers call toast.Add, never return a tick
	return m, tea.Batch(cmd, toast.Ticks(before, m.toasts))
}
m.toasts = toast.Add(m.toasts, toast.Toast{Source: "sync", Text: "syncing…", Level: toast.Progress})
m.toasts = toast.Add(m.toasts, toast.Toast{Source: "sync", Text: "synced", Level: toast.Success}) // replaces it
```

Several tables sharing one `keys.yaml`, or one file each:

```go
o, err := keys.ReadOverrides(keys.ConfigPath("ghx"))
mine, _ := adminKeys.Known(o)
adminKeys, err = adminKeys.Override(mine, adminStacks...)
// or: adminKeys.LoadOverrides(keys.ConfigPath("ghx", "admin")) // ghx/keys-admin.yaml
```

Tests run command batches without sleeping on timers:

```go
msgs := tuitest.Drain(cmd, 0) // toast and saver ticks are skipped
```

An image in a pane — detect in main, send beside the frame, draw as text:

```go
caps := kitty.Detect(os.Getenv, kitty.ExecTmux{}) // may run tmux: once, before the program
out, tty, err := kitty.OpenTTY()                  // writes go beside Bubble Tea's frame
// Init: tea.Batch(kitty.CellSizeCmd(os.Stdout.Fd()), kitty.EnsurePassthroughCmd(caps, kitty.ExecTmux{}))

case tea.WindowSizeMsg:
	next := m.img // kitty.Image{ID: 1, Source: kitty.Source{Key: path, Data: png}, Alt: "screenshot"}
	next.Cols, next.Rows = kitty.Fit(imgW, imgH, paneW, paneH, m.cell.W, m.cell.H)
	cmd = next.Cmds(m.img, m.caps, m.out, m.cell) // nil when nothing changed
	m.img = next
case kitty.Wrote:
	// msg.Err: show it
// View: m.img.View(m.caps) — exactly Cols×Rows cells; frame it, never truncate it
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
| kitty detection, passthrough, pane offset, `Fit` | ccx `internal/kitty/graphics.go`; visibility poll as a command from sendbird/ccx#175 (`kitty_watch.go`) |
| kitty chunking, placeholders, payload shrinking, `Out` lock | ccproxy `internal/kitty`, `cmd/ccproxy/monitor_tui_detail.go`; diacritic table regenerated from Kitty's `rowcolumn-diacritics.txt` |
| styles, NO_COLOR, glyphs | dayx `styles.go`, okx `styles.go`, argx `glyphs.go` |
