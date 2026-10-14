// Package keys is one binding table that drives key dispatch, the footer, the
// `?` help and a command palette (engine principle E3), plus Korean-layout
// normalization (E13).
//
// Hand-written help drifts from the dispatcher: ccproxy's help said `tab`
// cycles detail tabs while the code routed it to the session list, ghx listed
// `h / l` twice with different meanings. Deriving every surface from the same
// []Binding makes that drift impossible.
package keys

import (
	"maps"
	"os"
	"path/filepath"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

// Ctx names a key context: a pane, a modal, a text input, or "global". The app
// decides which contexts are active and passes them innermost-first.
type Ctx string

// Binding is one named action and the keys that trigger it in one context.
type Binding struct {
	Action string   // stable name, e.g. "list.open"; overrides address this
	Keys   []string // tea.KeyMsg.String() forms: "q", "ctrl+c", "enter", " "
	Ctx    Ctx
	Group  string // help section; empty falls back to the context name
	Help   string // short description shown in footer, help and palette
	// Short replaces Help in the footer only, where width is scarce: Help
	// "toggle request timing", Short "timing". A state label (Labels) still
	// wins, since it says what the key does now.
	Short string

	// Labels replaces Help while the action is in a named state, for one key
	// whose meaning flips with the model: t is "group" until the board is
	// grouped, then "ungroup". Opts.State names the current state; a state
	// with no entry here (or a nil State) shows Help.
	//
	// Why a map plus a per-call State func, and not a HelpFn on the binding:
	// a func stored in the table is built once, so it closes over whichever
	// model existed then, and Bubble Tea hands every Update a fresh copy — a
	// HelpFn would keep reading the stale one. State is passed per call, like
	// Enabled, so it always reads the model being rendered. Labels stay plain
	// data: comparable in tests, printable, cloned by New so copies never
	// share it. Declaring two bindings on one key (group.on/group.off gated by
	// Enabled) worked but was a Duplicate conflict and two actions to rebind.
	Labels map[string]string

	// Footer is the footer priority: 0 keeps the binding out of the footer,
	// and a lower number is kept longer when the terminal is narrow.
	Footer int
	// FooterGroup collapses every footer hint sharing it into one hint whose
	// keys are the members' first keys and whose label is FooterGroup:
	// h/j/k/l with FooterGroup "move" render as `hjkl move`. Members keep
	// their own Help in the help screen and palette. The hint sits where its
	// first member would and takes the lowest Footer among the members.
	FooterGroup string
	// Pinned bindings (quit, help) are never dropped from the footer: they are
	// how a lost user gets out or finds the keys that did not fit.
	Pinned bool

	// Shadows lists actions this binding deliberately hides in outer
	// contexts, so Conflicts does not report it: a detail pane whose q closes
	// the pane before it quits declares Shadows: []string{"quit"}.
	//
	// Actions, not keys, because user overrides change keys: a declaration on
	// "q" would silently stop covering the pair once the user moved both to
	// x, and would keep excusing q after quit moved away from it.
	Shadows []string
}

// Key returns the first key, the one surfaces display.
func (b Binding) Key() string {
	if len(b.Keys) == 0 {
		return ""
	}
	return b.Keys[0]
}

// Label is the text shown for b: Labels[state(b.Action)] when state names a
// labelled state, else Help.
func (b Binding) Label(state func(action string) string) string {
	if state != nil {
		if l, ok := b.Labels[state(b.Action)]; ok {
			return l
		}
	}
	return b.Help
}

// footerLabel is the footer text: a state label, else Short, else Help.
func (b Binding) footerLabel(state func(string) string) string {
	if state != nil {
		if l, ok := b.Labels[state(b.Action)]; ok {
			return l
		}
	}
	if b.Short != "" {
		return b.Short
	}
	return b.Help
}

// clone deep-copies the reference fields, so a Binding handed out or taken in
// never aliases the table.
func (b Binding) clone() Binding {
	b.Keys = slices.Clone(b.Keys)
	b.Shadows = slices.Clone(b.Shadows)
	b.Labels = maps.Clone(b.Labels)
	return b
}

// Opts carries the model's view of its actions to Resolve, Footer, Help and
// Entries. Every field may be nil. The funcs are passed per call, never stored
// in the Keymap, so they read the model being handled rather than a copy
// captured when the table was built. An app typically builds one per frame:
//
//	func (m model) keyOpts() keys.Opts {
//		return keys.Opts{Enabled: m.canRun, Visible: m.worthOffering, State: m.keyState}
//	}
type Opts struct {
	// Enabled says whether an action can run now. Disabled bindings do not
	// resolve (the key falls through to outer contexts) and are left out of
	// the footer and palette. nil enables everything.
	Enabled func(action string) bool
	// Visible narrows the footer only: an enabled action whose key would do
	// nothing visible right now (move with an empty list) can stay runnable
	// while the footer spends its width on keys that matter. The footer shows
	// a binding when Enabled and Visible both allow it, so Visible need not
	// repeat Enabled's checks. A hidden binding still claims its key from
	// outer contexts — it still runs. nil shows every enabled binding.
	Visible func(action string) bool
	// State names the action's current state for Binding.Labels. nil, or a
	// state with no label, shows Help.
	State func(action string) string
}

func (o Opts) enabled(action string) bool { return o.Enabled == nil || o.Enabled(action) }
func (o Opts) visible(action string) bool { return o.Visible == nil || o.Visible(action) }

// Keymap is an immutable binding table.
//
// It is safe to copy into a Bubble Tea model and copy again on every Update:
// nothing in it is mutated after construction, so the slices and the index map
// shared between copies never change underneath either copy. Override returns
// a new Keymap instead of editing this one.
//
// The zero Keymap is safe — every method works and it binds nothing — so a
// model can hold a Keymap by value, not a *Keymap it must nil-check. Tests
// that build model literals without the table can still dispatch through
// m.km.Or(defaultKeymap).
type Keymap struct {
	bindings []Binding
	// index maps ctx → key → positions in bindings, in declaration order.
	index map[Ctx]map[string][]int
}

// New builds a Keymap. The bindings are deep-copied, so the caller may reuse
// its slices and maps.
func New(bindings ...Binding) Keymap {
	bs := make([]Binding, len(bindings))
	for i, b := range bindings {
		bs[i] = b.clone()
	}
	return Keymap{bindings: bs, index: buildIndex(bs)}
}

// Must returns km, panicking if err is non-nil. It is for package-level
// tables whose overrides are part of the program, not user input:
//
//	var base = keys.Must(keys.New(defaults...).Override(platformKeys))
func Must(km Keymap, err error) Keymap {
	if err != nil {
		panic(err)
	}
	return km
}

// IsZero reports whether km is the zero Keymap (not built by New).
func (km Keymap) IsZero() bool { return km.bindings == nil }

// Or returns km, or fallback when km is the zero Keymap.
func (km Keymap) Or(fallback Keymap) Keymap {
	if km.IsZero() {
		return fallback
	}
	return km
}

// ConfigPath is the user override file for app:
// $XDG_CONFIG_HOME/<app>/keys.yaml, falling back to ~/.config/<app>/keys.yaml.
// Config, not state: the user writes it and the program only reads it (see
// LoadOverrides).
func ConfigPath(app string) string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, app, "keys.yaml")
}

func buildIndex(bs []Binding) map[Ctx]map[string][]int {
	idx := map[Ctx]map[string][]int{}
	for i, b := range bs {
		if idx[b.Ctx] == nil {
			idx[b.Ctx] = map[string][]int{}
		}
		for _, k := range b.Keys {
			idx[b.Ctx][k] = append(idx[b.Ctx][k], i)
		}
	}
	return idx
}

// Bindings returns a deep copy of the table, in declaration order.
func (km Keymap) Bindings() []Binding {
	out := make([]Binding, len(km.bindings))
	for i, b := range km.bindings {
		out[i] = b.clone()
	}
	return out
}

// Resolve finds the binding for k in the active contexts, innermost first.
// k is normalized with NormalizeCJK first, so shortcuts work under a Korean
// input source. Only o.Enabled is consulted.
//
// A context that binds the key but whose binding is disabled does not
// swallow the key: resolution falls through to outer contexts. That mirrors
// the engine's "a key no binding in the active context claims passes to the
// parent" — a disabled binding claims nothing. An app that wants a disabled
// key to be inert rather than fall through binds it enabled and shows a
// reason in its handler.
//
// Within one context the first enabled binding in declaration order wins.
func (km Keymap) Resolve(active []Ctx, k tea.KeyMsg, o Opts) (Binding, bool) {
	return km.ResolveRaw(active, NormalizeCJK(k), o)
}

// ResolveRaw is Resolve without CJK normalization, for when a text field owns
// the keyboard and a jamo is the intended input.
func (km Keymap) ResolveRaw(active []Ctx, k tea.KeyMsg, o Opts) (Binding, bool) {
	return km.resolveKey(active, k.String(), o)
}

func (km Keymap) resolveKey(active []Ctx, key string, o Opts) (Binding, bool) {
	for _, ctx := range active {
		for _, i := range km.index[ctx][key] {
			b := km.bindings[i]
			if o.enabled(b.Action) {
				return b.clone(), true
			}
		}
	}
	return Binding{}, false
}

// shown is an action as a surface shows it: the innermost reachable binding,
// with Keys widened to every key that still reaches the action in any active
// context, innermost first.
type shown struct {
	Binding
}

// reachable returns one entry per action reachable in active, innermost
// context first. Keys claimed by an enabled binding in a more inner context
// are dropped; an action left with no key is dropped. Surfaces built from it
// never advertise a key that would do something else.
//
// An action bound in several contexts (quit as q in a list and ctrl+c
// globally) is one thing to the user, so it is one entry: the innermost
// binding's fields, the keys of all of them, Pinned if any is pinned, and
// the lowest non-zero Footer. Keeping only the innermost binding instead
// would lose a pinned outer hint behind an unpinned inner one (ccproxy).
func (km Keymap) reachable(active []Ctx, enabled func(string) bool) []shown {
	on := func(a string) bool { return enabled == nil || enabled(a) }
	claimed := map[string]bool{}
	at := map[string]int{} // action → position in out
	var out []shown
	for _, ctx := range active {
		for _, b := range km.bindings {
			if b.Ctx != ctx || !on(b.Action) {
				continue
			}
			ks := unclaimed(b.Keys, claimed)
			if len(ks) == 0 {
				continue
			}
			i, ok := at[b.Action]
			if !ok {
				b = b.clone()
				b.Keys = ks // only the keys that still reach this binding
				at[b.Action] = len(out)
				out = append(out, shown{b})
				continue
			}
			m := &out[i].Binding
			for _, k := range ks {
				if !slices.Contains(m.Keys, k) {
					m.Keys = append(m.Keys, k)
				}
			}
			m.Pinned = m.Pinned || b.Pinned
			if b.Footer > 0 && (m.Footer == 0 || b.Footer < m.Footer) {
				m.Footer = b.Footer
			}
		}
		// Claim after the whole context is scanned: two bindings in the same
		// context sharing a key are a Conflict, not a shadow.
		for _, b := range km.bindings {
			if b.Ctx == ctx && on(b.Action) {
				for _, k := range b.Keys {
					claimed[k] = true
				}
			}
		}
	}
	return out
}

func unclaimed(ks []string, claimed map[string]bool) []string {
	var out []string
	for _, k := range ks {
		if !claimed[k] {
			out = append(out, k)
		}
	}
	return out
}
