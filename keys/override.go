package keys

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Override returns a copy of km with Keys replaced, for user rebinding. Each
// map key is either
//
//   - an action ("quit"): every binding of that action, in every context,
//     gets the keys; or
//   - a context-qualified action ("list.quit", context "list", action
//     "quit"): only that context's binding does.
//
// A qualified entry beats a plain one for its context, so
// `{quit: [x], global.quit: [ctrl+c]}` moves quit to x everywhere but keeps
// ctrl+c globally. A name that reads both ways (action "list.open" and
// context "list" with action "open") is an error rather than a guess.
//
// Override fails, returning km unchanged, when a name matches nothing or the
// result has a conflict the original table did not have: one key bound to
// two actions in one context (Duplicate — the second action becomes
// unreachable), or, in any of stacks, an inner key hiding a Pinned binding
// (Shadowed — `help: [j]` where the list binds j makes help unreachable
// there and drops its pinned hint). Pass the same stacks as to Conflicts;
// without them only Duplicates can be judged. The error lists every problem.
func (km Keymap) Override(o map[string][]string, stacks ...[]Ctx) (Keymap, error) {
	if len(o) == 0 {
		return km, nil
	}
	byAction := map[string][]string{}
	byScope := map[target][]string{}
	var problems []string
	names := km.names()
	for _, name := range sortedNames(o) {
		switch kind, t := names.classify(name); kind {
		case nameAmbiguous:
			problems = append(problems, fmt.Sprintf("ambiguous name %q (an action and a context-qualified action)", name))
		case namePlain:
			byAction[name] = o[name]
		case nameScoped:
			byScope[t] = o[name]
		default:
			problems = append(problems, fmt.Sprintf("unknown action %q", name))
		}
	}
	if len(problems) > 0 {
		return km, fmt.Errorf("keymap override: %s", strings.Join(problems, "; "))
	}

	bs := km.Bindings()
	set := func(i int, ks []string) {
		// A KeyLabel ("1-9") describes the default keys; after a rebind it
		// would name keys that no longer do anything.
		if !slices.Equal(bs[i].Keys, ks) {
			bs[i].KeyLabel = ""
		}
		bs[i].Keys = slices.Clone(ks)
	}
	for i, b := range bs {
		if ks, ok := byScope[target{b.Ctx, b.Action}]; ok {
			set(i, ks)
		} else if ks, ok := byAction[b.Action]; ok {
			set(i, ks)
		}
	}
	out := Keymap{bindings: bs, index: buildIndex(bs)}

	before := map[string]bool{}
	for _, c := range append(km.duplicates(), km.shadows(stacks)...) {
		before[c.String()] = true
	}
	for _, c := range append(out.duplicates(), out.shadows(stacks)...) {
		if !before[c.String()] {
			problems = append(problems, c.String())
		}
	}
	if len(problems) > 0 {
		return km, fmt.Errorf("keymap override creates conflicts: %s", strings.Join(problems, "; "))
	}
	return out, nil
}

type target struct {
	ctx    Ctx
	action string
}

type nameKind int

const (
	nameUnknown nameKind = iota
	namePlain
	nameScoped
	nameAmbiguous
)

// tableNames is what override names can refer to in a table.
type tableNames struct {
	plain  map[string]bool
	scoped map[target]bool
}

func (km Keymap) names() tableNames {
	n := tableNames{plain: map[string]bool{}, scoped: map[target]bool{}}
	for _, b := range km.bindings {
		n.plain[b.Action] = true
		n.scoped[target{b.Ctx, b.Action}] = true
	}
	return n
}

// classify reads name as an action or a context-qualified action.
func (n tableNames) classify(name string) (nameKind, target) {
	var hits []target
	for i := range len(name) {
		if name[i] == '.' {
			if t := (target{Ctx(name[:i]), name[i+1:]}); n.scoped[t] {
				hits = append(hits, t)
			}
		}
	}
	switch {
	case n.plain[name] && len(hits) > 0, len(hits) > 1:
		return nameAmbiguous, target{}
	case n.plain[name]:
		return namePlain, target{}
	case len(hits) == 1:
		return nameScoped, hits[0]
	}
	return nameUnknown, target{}
}

// Known splits overrides into the names this table binds and the ones it
// does not (sorted). Several tables that share one keys.yaml (ghx's main,
// admin and actions maps) each apply only their own entries:
//
//	o, err := keys.ReadOverrides(keys.ConfigPath("ghx"))
//	mine, _ := adminKeys.Known(o)
//	adminKeys, err = adminKeys.Override(mine, adminStacks...)
//
// An app that wants a typo reported checks that every name is known to at
// least one table. Ambiguous names count as known, so Override still
// reports them.
func (km Keymap) Known(o map[string][]string) (known map[string][]string, unknown []string) {
	names := km.names()
	known = map[string][]string{}
	for _, name := range sortedNames(o) {
		if kind, _ := names.classify(name); kind == nameUnknown {
			unknown = append(unknown, name)
		} else {
			known[name] = slices.Clone(o[name])
		}
	}
	return known, unknown
}

// ReadOverrides reads an overrides file (see LoadOverrides) without applying
// it, for apps that split one file between tables (Known). A missing file
// is nil and no error.
func ReadOverrides(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("keymap override: %w", err)
	}
	var o map[string][]string
	if err := yaml.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("keymap override %s: %w", path, err)
	}
	return o, nil
}

func sortedNames(o map[string][]string) []string {
	names := make([]string, 0, len(o))
	for n := range o {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LoadOverrides reads a YAML file mapping actions (or context.action names,
// see Override) to key lists and applies it with Override:
//
//	quit: [x]
//	global.quit: [ctrl+c]
//	list.open: [enter, o]
//
// A missing file is not an error — most users never write one — and returns
// km unchanged. ConfigPath is the conventional location. stacks are passed to
// Override.
func (km Keymap) LoadOverrides(path string, stacks ...[]Ctx) (Keymap, error) {
	o, err := ReadOverrides(path)
	if err != nil {
		return km, err
	}
	out, err := km.Override(o, stacks...)
	if err != nil {
		return km, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}
