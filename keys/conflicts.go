package keys

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ConflictKind classifies a Conflict.
type ConflictKind int

const (
	// Duplicate: one key bound to two actions in the same context. Only the
	// first can ever fire; the second is dead and its help is a lie.
	Duplicate ConflictKind = iota
	// Shadowed: an inner context binds a key that a Pinned binding (quit,
	// help) uses in an outer context, so the footer promises "q quit" while q
	// does something else there. Ordinary inner-over-outer overriding (esc in
	// a modal) is the point of contexts and is not reported, and neither is
	// a binding that lists the pinned action in its Shadows.
	Shadowed
	// Unreachable: an action none of whose keys in a context a 2-set Korean
	// input source can produce (see ReachableUnderCJK) — G alone is
	// reported, G with an end alias is not. Key is the first unreachable
	// key. A binding meant to be Latin-only declares LatinOnly instead.
	Unreachable
)

func (k ConflictKind) String() string {
	switch k {
	case Duplicate:
		return "duplicate"
	case Shadowed:
		return "shadowed"
	case Unreachable:
		return "unreachable-under-cjk"
	}
	return fmt.Sprintf("ConflictKind(%d)", int(k))
}

// Conflict is one problem in a binding table.
type Conflict struct {
	Kind    ConflictKind
	Key     string
	Ctx     Ctx      // where the key is bound (the inner context for Shadowed)
	Outer   Ctx      // the shadowed context; empty for other kinds
	Actions []string // the actions involved, in declaration order
}

func (c Conflict) String() string {
	switch c.Kind {
	case Shadowed:
		return fmt.Sprintf("%s: %q in %s (%s) hides pinned %s in %s", c.Kind, c.Key, c.Ctx, c.Actions[0], c.Actions[1], c.Outer)
	default:
		return fmt.Sprintf("%s: %q in %s: %s", c.Kind, c.Key, c.Ctx, strings.Join(c.Actions, ", "))
	}
}

// Conflicts lists problems in the table. Apps call it from a test:
//
//	func TestKeymapHasNoConflicts(t *testing.T) {
//		for _, c := range appKeymap().Conflicts(stackList, stackDetail) {
//			t.Error(c)
//		}
//	}
//
// Duplicate and Unreachable need no context information. Shadowing depends on
// which contexts can be active together, which only the app knows, so it is
// checked for each stack passed (innermost first, as for Resolve). A shadow
// the app intends is declared on the inner binding (Binding.Shadows), not
// filtered out of this list: the test then stays the plain loop above.
//
// The spec named this Conflicts() with no arguments; the stacks were added
// because a Keymap has no notion of context nesting otherwise.
func (km Keymap) Conflicts(stacks ...[]Ctx) []Conflict {
	var out []Conflict
	out = append(out, km.duplicates()...)
	out = append(out, km.shadows(stacks)...)
	return append(out, km.unreachable()...)
}

// unreachable judges per action within its context, not per key: an
// uppercase key is fine as long as another key of the same action in the
// same context survives the IME (G and end, Y and y). Six adopters hit the
// per-key rule on exactly that shape. Per context rather than across the
// table, because an alias bound in another pane does not help the user in
// this one. LatinOnly bindings are left out of the judgement entirely.
func (km Keymap) unreachable() []Conflict {
	type id struct {
		ctx    Ctx
		action string
	}
	var order []id
	firstBad := map[id]string{}
	ok := map[id]bool{}
	for _, b := range km.bindings {
		if b.LatinOnly {
			continue
		}
		a := id{b.Ctx, b.Action}
		for _, k := range b.Keys {
			if ReachableUnderCJK(k) {
				ok[a] = true
			} else if _, seen := firstBad[a]; !seen {
				firstBad[a] = k
				order = append(order, a)
			}
		}
	}
	var out []Conflict
	for _, a := range order {
		if !ok[a] {
			out = append(out, Conflict{Kind: Unreachable, Key: firstBad[a], Ctx: a.ctx, Actions: []string{a.action}})
		}
	}
	return out
}

func (km Keymap) duplicates() []Conflict {
	var out []Conflict
	for _, ctx := range sortedCtxs(km.index) {
		keys := km.index[ctx]
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			var actions []string
			for _, i := range keys[k] {
				a := km.bindings[i].Action
				if !slices.Contains(actions, a) {
					actions = append(actions, a)
				}
			}
			if len(actions) > 1 {
				out = append(out, Conflict{Kind: Duplicate, Key: k, Ctx: ctx, Actions: actions})
			}
		}
	}
	return out
}

func (km Keymap) shadows(stacks [][]Ctx) []Conflict {
	var out []Conflict
	seen := map[string]bool{}
	for _, stack := range stacks {
		for outer := range stack {
			for _, b := range km.bindings {
				if b.Ctx != stack[outer] || !b.Pinned {
					continue
				}
				for _, k := range b.Keys {
					for inner := 0; inner < outer; inner++ {
						for _, i := range km.index[stack[inner]][k] {
							hider := km.bindings[i]
							if hider.Action == b.Action || slices.Contains(hider.Shadows, b.Action) {
								continue
							}
							id := fmt.Sprint(k, stack[inner], hider.Action, b.Ctx, b.Action)
							if seen[id] {
								continue
							}
							seen[id] = true
							out = append(out, Conflict{
								Kind: Shadowed, Key: k, Ctx: stack[inner], Outer: b.Ctx,
								Actions: []string{hider.Action, b.Action},
							})
						}
					}
				}
			}
		}
	}
	return out
}

func sortedCtxs(m map[Ctx]map[string][]int) []Ctx {
	out := make([]Ctx, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
