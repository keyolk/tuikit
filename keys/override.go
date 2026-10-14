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
	type target struct {
		ctx    Ctx
		action string
	}
	plain := map[string]bool{}
	scoped := map[target]bool{}
	for _, b := range km.bindings {
		plain[b.Action] = true
		scoped[target{b.Ctx, b.Action}] = true
	}
	byAction := map[string][]string{}
	byScope := map[target][]string{}
	var problems []string
	for _, name := range sortedNames(o) {
		var hits []target
		for i := range len(name) {
			if name[i] == '.' {
				if t := (target{Ctx(name[:i]), name[i+1:]}); scoped[t] {
					hits = append(hits, t)
				}
			}
		}
		switch {
		case plain[name] && len(hits) > 0, len(hits) > 1:
			problems = append(problems, fmt.Sprintf("ambiguous name %q (an action and a context-qualified action)", name))
		case plain[name]:
			byAction[name] = o[name]
		case len(hits) == 1:
			byScope[hits[0]] = o[name]
		default:
			problems = append(problems, fmt.Sprintf("unknown action %q", name))
		}
	}
	if len(problems) > 0 {
		return km, fmt.Errorf("keymap override: %s", strings.Join(problems, "; "))
	}

	bs := km.Bindings()
	for i, b := range bs {
		if ks, ok := byScope[target{b.Ctx, b.Action}]; ok {
			bs[i].Keys = slices.Clone(ks)
		} else if ks, ok := byAction[b.Action]; ok {
			bs[i].Keys = slices.Clone(ks)
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
	out, err := km.Override(o, stacks...)
	if err != nil {
		return km, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}
