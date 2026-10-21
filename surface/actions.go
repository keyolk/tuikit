package surface

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/keyolk/tuikit/keys"
	"github.com/keyolk/tuikit/link"
	"github.com/keyolk/tuikit/split"
)

// agentAction is a Button the agent declared: what the renderer binds into
// its key table and the palette.
type agentAction struct {
	id      string // the Button's component id; also the binding's action suffix
	label   string
	key     string // the preferred key, as the agent asked
	confirm bool
	danger  bool
	spec    actionSpec
	scope   string
}

func (a agentAction) action() string { return agentPrefix + a.id }

const agentPrefix = "agent."

// Contexts. Every built-in and every agent action lives in ctxMain, so a
// clash is a Duplicate that keys.Conflicts reports, whatever key it is.
// Separate contexts would hide it: an inner context silently wins, and
// Conflicts reports cross-context clashes only for pinned keys.
const (
	ctxMain    keys.Ctx = "main"
	ctxInput   keys.Ctx = "input"
	ctxConfirm keys.Ctx = "confirm"
	ctxHelp    keys.Ctx = "help"
	ctxPalette keys.Ctx = "palette"
)

var (
	stackMain    = []keys.Ctx{ctxMain}
	stackInput   = []keys.Ctx{ctxInput}
	stackConfirm = []keys.Ctx{ctxConfirm}
	stackHelp    = []keys.Ctx{ctxHelp}
	stackPalette = []keys.Ctx{ctxPalette}
	allStacks    = [][]keys.Ctx{stackMain, stackInput, stackConfirm, stackHelp, stackPalette}
)

// builtins is what the renderer guarantees whatever the agent sends. They
// come first in the table, so even a clash that slipped past Conflicts
// resolves to the built-in (Resolve takes the first match).
func builtins() []keys.Binding {
	mv := func(action string, ks []string, help string, footer int) keys.Binding {
		return keys.Binding{Action: action, Keys: ks, Ctx: ctxMain, Group: "Move", Help: help, Footer: footer, FooterGroup: "move"}
	}
	bs := []keys.Binding{
		{Action: "quit", Keys: []string{"q", "ctrl+c"}, Ctx: ctxMain, Group: "General", Help: "quit", Pinned: true, Footer: 1},
		{Action: "help", Keys: []string{"?"}, Ctx: ctxMain, Group: "General", Help: "help", Pinned: true, Footer: 1},
		{Action: "palette", Keys: []string{":"}, Ctx: ctxMain, Group: "General", Help: "actions", Footer: 2, NoPalette: true},
		mv("down", []string{"j", "down"}, "down", 3),
		mv("up", []string{"k", "up"}, "up", 3),
		{Action: "top", Keys: []string{"g", "home"}, Ctx: ctxMain, Group: "Move", Help: "first row", NoPalette: true},
		{Action: "bottom", Keys: []string{"G", "end"}, Ctx: ctxMain, Group: "Move", Help: "last row", NoPalette: true},
		{Action: "focus.next", Keys: []string{"tab"}, Ctx: ctxMain, Group: "Move", Help: "next pane", Footer: 5},
		{Action: "focus.prev", Keys: []string{"shift+tab"}, Ctx: ctxMain, Group: "Move", Help: "previous pane"},
		{Action: "tab.next", Keys: []string{"]"}, Ctx: ctxMain, Group: "View", Help: "next tab", Footer: 6, FooterGroup: "tab"},
		{Action: "tab.prev", Keys: []string{"["}, Ctx: ctxMain, Group: "View", Help: "previous tab", Footer: 6, FooterGroup: "tab"},
		{Action: "activate", Keys: []string{"enter"}, Ctx: ctxMain, Group: "View", Help: "edit field / press button", NoPalette: true},
		link.BindingWith(ctxMain, "View", 4),
	}
	// < > = resize the Split under focus (or the first one on screen).
	bs = append(bs, split.Bindings(ctxMain)...)
	return append(bs, []keys.Binding{
		{Action: "input.done", Keys: []string{"esc", "enter"}, Ctx: ctxInput, Help: "done", Footer: 1},
		{Action: "focus.next", Keys: []string{"tab"}, Ctx: ctxInput, Help: "next pane", Footer: 2},
		{Action: "focus.prev", Keys: []string{"shift+tab"}, Ctx: ctxInput, Help: "previous pane"},
		{Action: "quit", Keys: []string{"ctrl+c"}, Ctx: ctxInput, Help: "quit", Pinned: true, Footer: 3},

		{Action: "confirm.yes", Keys: []string{"y"}, Ctx: ctxConfirm, Help: "confirm", Footer: 1},
		{Action: "confirm.no", Keys: []string{"n", "esc"}, Ctx: ctxConfirm, Help: "cancel", Footer: 1},
		{Action: "quit", Keys: []string{"ctrl+c"}, Ctx: ctxConfirm, Help: "quit", Pinned: true, Footer: 2},

		{Action: "help.close", Keys: []string{"esc", "q", "?"}, Ctx: ctxHelp, Help: "close help", Footer: 1},
		{Action: "help.down", Keys: []string{"j", "down"}, Ctx: ctxHelp, Help: "scroll down", Footer: 2, FooterGroup: "scroll"},
		{Action: "help.up", Keys: []string{"k", "up"}, Ctx: ctxHelp, Help: "scroll up", Footer: 2, FooterGroup: "scroll"},
		{Action: "quit", Keys: []string{"ctrl+c"}, Ctx: ctxHelp, Help: "quit", Pinned: true, Footer: 3},

		{Action: "palette.run", Keys: []string{"enter"}, Ctx: ctxPalette, Help: "run", Footer: 1},
		{Action: "palette.close", Keys: []string{"esc"}, Ctx: ctxPalette, Help: "close", Footer: 1},
		{Action: "palette.down", Keys: []string{"down", "ctrl+n"}, Ctx: ctxPalette, Help: "next", Footer: 2, FooterGroup: "pick"},
		{Action: "palette.up", Keys: []string{"up", "ctrl+p"}, Ctx: ctxPalette, Help: "previous", Footer: 2, FooterGroup: "pick"},
		{Action: "quit", Keys: []string{"ctrl+c"}, Ctx: ctxPalette, Help: "quit", Pinned: true, Footer: 3},
	}...)
}

// agentKey is the shape of a key an agent may ask for: one letter or digit,
// ctrl+letter, or a function key. Anything else (a jamo, "space", a
// multi-key chord) is not bound, since Resolve would never produce it or the
// user could not see what to press.
var agentKey = regexp.MustCompile(`^([a-zA-Z0-9]|ctrl\+[a-z]|f([1-9]|1[0-2]))$`)

// bindResult is the keymap for a set of agent actions plus what had to be
// refused, one line per problem, for the toast.
type bindResult struct {
	km       keys.Keymap
	bound    map[string]string // action id -> key; "" means palette only
	problems map[string]string // action id -> why its key was not bound
}

// bindActions builds the one key table (E2) for the built-ins and actions.
// Each action is first bound with its preferred key; keys.Conflicts then
// judges the whole table, and every action involved in a Duplicate or a
// Shadowed conflict loses its key and stays reachable from the palette. The
// built-ins never lose theirs.
func bindActions(actions []agentAction) bindResult {
	res := bindResult{bound: map[string]string{}, problems: map[string]string{}}
	bs := builtins()
	idx := map[string]int{} // action name -> position in bs
	for _, a := range actions {
		b := keys.Binding{Action: a.action(), Ctx: ctxMain, Group: "Actions", Help: a.label, Footer: 7}
		switch {
		case a.key == "":
		case !agentKey.MatchString(a.key):
			res.problems[a.id] = fmt.Sprintf("key %q is not a single letter, digit, ctrl+letter or f-key", a.key)
		default:
			b.Keys = []string{a.key}
			// An uppercase key is meant to be Shift+letter; Korean layouts
			// cannot type it, but the palette still can, so it is declared.
			b.LatinOnly = true
		}
		idx[b.Action] = len(bs)
		bs = append(bs, b)
	}
	for _, c := range keys.New(bs...).Conflicts(allStacks...) {
		if c.Kind == keys.Unreachable {
			continue
		}
		for n, name := range c.Actions {
			i, ok := idx[name]
			if !ok || len(bs[i].Keys) == 0 {
				continue
			}
			if n == 0 && c.Kind == keys.Duplicate {
				// Two agent actions on one key: the first declared keeps it,
				// as Resolve would. A built-in is always declared first, so
				// this never lets an agent keep a built-in's key.
				continue
			}
			id := name[len(agentPrefix):]
			res.problems[id] = fmt.Sprintf("key %q is taken (%s); use the palette", bs[i].Keys[0], otherThan(c.Actions, name))
			bs[i].Keys = nil
		}
	}
	for _, a := range actions {
		res.bound[a.id] = ""
		if k := bs[idx[a.action()]].Keys; len(k) > 0 {
			res.bound[a.id] = k[0]
		}
	}
	res.km = keys.New(bs...)
	return res
}

func otherThan(names []string, self string) string {
	rest := slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == self })
	if len(rest) == 0 {
		return "another action"
	}
	return rest[0]
}
