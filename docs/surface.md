# surface: an A2UI v0.9 renderer for the terminal

`surface` draws an [A2UI](https://a2ui.org/) stream as a Bubble Tea TUI. An agent
writes declarative JSON (one message per line); the renderer keeps the surface,
its component tree and its data model, applies updates as they arrive, and
writes user actions back as A2UI `action` messages. The agent writes no TUI
code, and nothing the agent sends can remove the renderer's guarantees.

This is a PoC. It is meant to show which A2UI ideas fit a terminal and which
do not.

## Spec read

| Source | Version |
|---|---|
| `specification/v0_9/` in [google/A2UI](https://github.com/google/A2UI) (now `a2ui-project/a2ui`) | commit `059b8d34874f96c0395dbfe23e6cc8b41cdb0f87`, 2026-10-20 |
| Protocol doc | `docs/a2ui_protocol.md`, "Version: 0.9, Status: Stable" |
| Schemas | `json/server_to_client.json`, `client_to_server.json`, `common_types.json`, `catalogs/basic/catalog.json` |
| Also read | `docs/a2ui_extension_specification.md` (A2A binding, processing rules), `docs/a2ui_custom_functions.md`, `docs/public/concepts/actions.md` |
| a2ui.org | lists v0.9.1 "Current", v0.9 "Stable", v1.0 "Candidate" |

v0.9.1 differs from v0.9 in the envelope only by the version string
(`"enum": ["v0.9", "v0.9.1"]`), so both are accepted on input. Events are
written as `"version": "v0.9"`.

## Messages

### Agent to renderer

| Message | Field | Supported | Notes |
|---|---|---|---|
| `createSurface` | `surfaceId`, `catalogId` | yes | A duplicate id is an error, as the spec says. |
| | `theme.agentDisplayName` | yes | Shown as the title. |
| | `theme.primaryColor`, `iconUrl` | ignored | No raw colors, no images. |
| | `sendDataModel` | stored, not sent | JSONL has no metadata channel; see Out of scope. |
| `updateComponents` | `components` (adjacency list, `root`) | yes | Forward references draw `… id` until the component arrives. |
| `updateDataModel` | `path` (JSON Pointer), `value` | yes | Upsert; `value` omitted removes the key; array removal keeps the length. |
| `deleteSurface` | `surfaceId` | yes | The previous surface is shown again. |
| Message arrays (`[…]` on one line) | | yes | Not a transaction: each element applies or fails alone (extension spec, "Processing Rules"). |

Every surface is kept; the newest one is drawn, and the title says how many exist.

### Renderer to agent

| Message | When |
|---|---|
| `{"version":"v0.9","action":{name, surfaceId, sourceComponentId, timestamp, context}}` | A Button's `action.event` runs (key, click or palette, after confirmation when required). `context` is resolved in the button's scope. |
| `{"version":"v0.9","error":{"code":"VALIDATION_FAILED", surfaceId, path, message}}` | A message was rejected. `path` points into the message (`/updateComponents/components/3`, `/1/...` for an array element), so an agent can correct itself. |

### Data binding

- `{"path": …}` absolute and relative (template scope) paths, as in "Collection scopes".
- `ChildList` templates over arrays (and objects, keys sorted).
- Two-way binding: `Input.value` writes on every keystroke; `List.selected` (terminal extension) writes the selected item's key.
- Type conversion for display follows the spec: null is `""`, objects and arrays are JSON.

### Client-side functions

| Function | Status |
|---|---|
| `formatString` | `${/abs}` and `${rel}` paths, `\${` escape. Function calls inside `${…}` are left as written. |
| `required`, `and`, `or`, `not` | yes |
| `openUrl` (as `action.functionCall`) | yes, http(s) only, through `link.OpenWith` |
| `regex`, `length`, `numeric`, `email`, `formatNumber`, `formatCurrency`, `formatDate`, `pluralize` | no: the component shows "function X is not supported" |
| `checks` on inputs and buttons | ignored (not evaluated, not enforced) |

## The terminal catalog

`catalogId`: **`tuikit.terminal/v1`**. The schema is `surface/catalog/terminal_v1.json`
(embedded as `surface.CatalogJSON`; `tuisurf --catalog` prints it for a prompt).
It uses the v0.9 common types (`ComponentId`, `ChildList`, `DataBinding`,
`Dynamic*`, `Action`), so a v0.9 validator can check it.

Surfaces created with the basic catalog id
(`https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json`) are drawn
with the components the two catalogs share (Text with `variant`, Row, Column,
List, Tabs, Button with a Text `child`, TextField, Card, Divider); the rest
(Image, Icon, Video, AudioPlayer, Modal, CheckBox, ChoicePicker, Slider,
DateTimeInput) draw an "unsupported component" placeholder.

| Component | Draws with | Notes |
|---|---|---|
| Column / Row | line stacking | One-line children keep their height; List, Table, Tabs, Split, Card share the rest by `weight`. Row widths by `weight`. |
| Split | `split.Model` | `left`, `right`, `ratio` (the agent's ratio is the default; the user's `<` `>` `=`, drag and double click are renderer state). |
| Tabs | own header | `[` `]` and clicks switch; the active tab is renderer state. |
| Text | `theme` roles | `style`: `muted`, `accent`, `ok`, `warn`, `err` only. |
| List | `list.List` | One line per item. `itemKey` is the identity, so the cursor follows the item when the agent reorders the array (E3). `selected` is two-way. |
| Table | `render.Pad` | `rows` binding, `columns[].value` relative to each row, `status: true` draws a glyph. |
| Detail | `render.Pad` | Key/value lines. |
| Input (`TextField`) | `input.Field` | Tab focuses it, then keys are text (Korean as typed); esc/enter leave; ctrl+c still quits. |
| Button | `keys` | `label`, `key`, `confirm`, `danger`, `action`. See Actions. |
| Badge (`Status`) | glyph + role | ✓ ! ✗ • lead the word, so status survives NO_COLOR. |
| Link | `link.Wrap` | OSC 8 only for `link.Valid` URLs; `o` opens the first visible one. Anything else is plain text. |
| Progress | `theme.Glyphs.Bar` | `value` 0..1, `label`. |
| Divider, Card | | |

Terminal extension: `selectionOf: "<List id>"` on any component makes its
subtree resolve relative paths against the item selected in that List. This is
how a detail pane follows the list without a round trip to the agent per
cursor move. It is the one addition the protocol has no equivalent for.

Unknown component types draw `⚠ unsupported component X (id)` in their own
rect. Bad properties draw the decode error there. A panic in a component is
caught by `render.Safe` and costs only that rect (E6). A cycle in the
adjacency list stops at the repeated id (and at depth 32).

### Lossy by design

- No images, icons, video or audio.
- No free layout: no `justify`/`align`, no absolute sizes. Rows split width by weight; Columns give one line to leaf components and share the rest.
- List and Table rows are one line; a multi-line template is cut to its first line.
- `theme.primaryColor` and any color are ignored; styling is the five tokens.
- Text "simple Markdown" is drawn as written.

## Actions and keys

The renderer builds **one** `keys.Keymap` (E2) from its built-ins plus every
Button on screen. Built-ins and agent actions share one context, so any clash
is a `keys.Conflicts` Duplicate; built-ins are declared first and always win.

1. A Button with a `key` of the allowed shape (one letter or digit, `ctrl+<letter>`, `f1`-`f12`) is added with that key.
2. `keys.Conflicts` runs over the whole table.
3. If an agent action is in a conflict, its key is removed, the action stays in the palette (`:`), the button shows `[:]`, and a toast says why. Between two agent actions, the first declared keeps the key.

Reserved keys: `q` `ctrl+c` `?` `:` `o` `j` `k` `↑` `↓` `g` `G` `home` `end`
`tab` `shift+tab` `[` `]` `enter` `<` `>` `=`.

`danger: true` always opens the renderer's confirm overlay, even with
`confirm: false`; only `y` confirms. The event is written only after `y`.

The renderer never runs commands. The only thing it opens is a `link.Valid`
http(s) URL, through `o` or `openUrl`.

## Built-ins the agent cannot remove

Footer and `?` help from the binding table (agent actions listed first, with
palette-only notes); toast queue; mouse (`mouse` cell motion: list clicks and
wheel, tab clicks, button clicks, field focus, split divider drag); drag
selection (`selection`, scoped to a Split pane); `o` link open; 80x24 floor
(below it: a "too small" message, and only `q` acts); alt screen; NO_COLOR
(colors gone, reverse video and glyphs kept); Korean-layout normalization
(`keys.Resolve`); no ticks while idle: Init returns nil and only toasts arm
timers, so an idle surface sends Bubble Tea no messages.

## Transport

```go
surface.Run(ctx, in io.Reader, out io.Writer, surface.RunOptions{...})
```

`in` is JSONL (no line length limit), `out` receives events as JSONL through a
bounded queue (a full queue drops the event and toasts it; the UI never blocks
on a slow agent). End of input toasts "stream ended" and keeps the surface.

`cmd/tuisurf`:

- `agent | tuisurf > events.jsonl`: the agent owns stdin and stdout, so the TUI
  opens **`/dev/tty`** for keys and drawing. Without a terminal this mode fails
  and says to use `--listen`.
- `tuisurf --listen /path.sock`: one agent connection carries both
  directions; the terminal stays the TUI's. Prefer this for agents. A stale
  socket file is replaced; a live one or a non-socket file is an error.
- `--log path` (default a temp file, printed on exit) receives rejected
  messages and pane panics.

## Demo

```sh
# socket mode (two panes)
go run ./cmd/tuisurf --listen /tmp/surf.sock
go run ./examples/surface/play --socket /tmp/surf.sock   # prints events

# stdin mode
go run ./examples/surface/play | go run ./cmd/tuisurf > /tmp/events.jsonl
```

`examples/surface/pr-dashboard.jsonl` is three fake PRs in a Split with a
Detail, a Tabs for checks/comments, a comment Input and `m` merge (danger),
`r` refresh, `c` comment, `o` open. The last three lines are incremental
updates: PR #11's pending check flips to success. No network is touched.

## Out of scope

- `sendDataModel`: there is no metadata channel on JSONL. A later version could add an envelope field or a sidecar line.
- Client capabilities exchange (`a2uiClientCapabilities`) and inline catalogs.
- A2A / MCP / SSE bindings; only JSONL over a pipe or a unix socket.
- `checks` validation and the locale-dependent functions.
- Drawing more than one surface at once.
- Persistence of renderer state (`split.State`, tab, cursor) across runs.

## Layout note

The first draft had a private fixed-ratio split behind a `Layout(mouse.Rect)`
interface so it could be swapped for `split` once that PR merged. `split`
merged (5b7fabe) before this PR, so `Split` uses `split.Model` directly and
the private layout is gone.
