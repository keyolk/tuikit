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
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
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
	// Footer is the footer priority: 0 keeps the binding out of the footer,
	// and a lower number is kept longer when the terminal is narrow.
	Footer int
	// Pinned bindings (quit, help) are never dropped from the footer: they are
	// how a lost user gets out or finds the keys that did not fit.
	Pinned bool
}

// Key returns the first key, the one surfaces display.
func (b Binding) Key() string {
	if len(b.Keys) == 0 {
		return ""
	}
	return b.Keys[0]
}

// Keymap is an immutable binding table.
//
// It is safe to copy into a Bubble Tea model and copy again on every Update:
// nothing in it is mutated after construction, so the slices and the index map
// shared between copies never change underneath either copy. Override returns
// a new Keymap instead of editing this one.
type Keymap struct {
	bindings []Binding
	// index maps ctx → key → positions in bindings, in declaration order.
	index map[Ctx]map[string][]int
}

// New builds a Keymap. The bindings are copied, so the caller may reuse its
// slice.
func New(bindings ...Binding) Keymap {
	bs := make([]Binding, len(bindings))
	for i, b := range bindings {
		b.Keys = slices.Clone(b.Keys)
		bs[i] = b
	}
	return Keymap{bindings: bs, index: buildIndex(bs)}
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

// Bindings returns a copy of the table, in declaration order.
func (km Keymap) Bindings() []Binding {
	out := make([]Binding, len(km.bindings))
	for i, b := range km.bindings {
		b.Keys = slices.Clone(b.Keys)
		out[i] = b
	}
	return out
}

// Resolve finds the binding for k in the active contexts, innermost first.
// k is normalized with NormalizeCJK first, so shortcuts work under a Korean
// input source.
//
// A context that binds the key but whose binding is disabled (enabled returns
// false) does not swallow the key: resolution falls through to outer contexts.
// That mirrors the engine's "a key no binding in the active context claims
// passes to the parent" — a disabled binding claims nothing. An app that wants
// a disabled key to be inert rather than fall through binds it enabled and
// shows a reason in its handler. A nil enabled treats every action as enabled.
//
// Within one context the first enabled binding in declaration order wins.
func (km Keymap) Resolve(active []Ctx, k tea.KeyMsg, enabled func(action string) bool) (Binding, bool) {
	return km.ResolveRaw(active, NormalizeCJK(k), enabled)
}

// ResolveRaw is Resolve without CJK normalization, for when a text field owns
// the keyboard and a jamo is the intended input.
func (km Keymap) ResolveRaw(active []Ctx, k tea.KeyMsg, enabled func(action string) bool) (Binding, bool) {
	return km.resolveKey(active, k.String(), enabled)
}

func (km Keymap) resolveKey(active []Ctx, key string, enabled func(string) bool) (Binding, bool) {
	for _, ctx := range active {
		for _, i := range km.index[ctx][key] {
			b := km.bindings[i]
			if enabled == nil || enabled(b.Action) {
				return b, true
			}
		}
	}
	return Binding{}, false
}

// Override returns a copy of km whose Keys are replaced per Action, for user
// rebinding. Every binding carrying the action is rebound (an action may be
// declared in several contexts). Unknown actions are an error listing them,
// and km is returned unchanged in that case.
func (km Keymap) Override(o map[string][]string) (Keymap, error) {
	known := map[string]bool{}
	for _, b := range km.bindings {
		known[b.Action] = true
	}
	var unknown []string
	for a := range o {
		if !known[a] {
			unknown = append(unknown, a)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return km, fmt.Errorf("keymap override: unknown actions: %s", strings.Join(unknown, ", "))
	}
	bs := km.Bindings()
	for i := range bs {
		if ks, ok := o[bs[i].Action]; ok {
			bs[i].Keys = slices.Clone(ks)
		}
	}
	return Keymap{bindings: bs, index: buildIndex(bs)}, nil
}

// LoadOverrides reads a YAML file of the form `{action: [keys...]}` and applies
// it with Override. A missing file is not an error — most users never write
// one — and returns km unchanged.
func (km Keymap) LoadOverrides(path string) (Keymap, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return km, nil
	}
	if err != nil {
		return km, fmt.Errorf("keymap override: %w", err)
	}
	var o map[string][]string
	if err := yaml.Unmarshal(data, &o); err != nil {
		return km, fmt.Errorf("keymap override %s: %w", path, err)
	}
	out, err := km.Override(o)
	if err != nil {
		return km, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

// visible returns the bindings reachable in active, innermost context first,
// one per action, with Keys narrowed to those not claimed by an enabled
// binding in a more inner context (dropped entirely when none are left). Surfaces built from it never advertise a
// key that would do something else.
func (km Keymap) visible(active []Ctx, enabled func(string) bool) []Binding {
	claimed := map[string]bool{}
	seen := map[string]bool{}
	var out []Binding
	for _, ctx := range active {
		var here []Binding
		for _, b := range km.bindings {
			if b.Ctx != ctx {
				continue
			}
			on := enabled == nil || enabled(b.Action)
			if ks := unclaimed(b.Keys, claimed); on && !seen[b.Action] && len(ks) > 0 {
				// Only the keys that still reach this binding are shown.
				b.Keys = ks
				here = append(here, b)
				seen[b.Action] = true
			}
		}
		// Claim after the whole context is scanned: two bindings in the same
		// context sharing a key are a Conflict, not a shadow.
		for _, b := range km.bindings {
			if b.Ctx == ctx && (enabled == nil || enabled(b.Action)) {
				for _, k := range b.Keys {
					claimed[k] = true
				}
			}
		}
		out = append(out, here...)
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
