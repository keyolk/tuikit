package surface

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// CatalogJSON is the terminal catalog as a JSON Schema document, the form an
// agent puts in its prompt (a2ui_protocol.md, "The component catalog").
//
//go:embed catalog/terminal_v1.json
var CatalogJSON []byte

// Properties of the terminal catalog's components (catalog/terminal_v1.json).
// Dynamic values stay json.RawMessage and are resolved per draw, in the scope
// the component is drawn in.

// common is what any component may carry. Weight is the basic catalog's
// flex weight inside a Row or Column. SelectionOf is a terminal-catalog
// extension: the component and its subtree resolve relative paths against
// the item selected in that List, which is how a detail pane follows a list
// without the agent round-tripping every cursor move.
type common struct {
	Weight      float64 `json:"weight"`
	SelectionOf string  `json:"selectionOf"`
}

// childList is common_types.json ChildList: an array of ids, or a template
// {componentId, path} instantiated once per element of the array at path.
type childList struct {
	IDs      []string
	Template string
	Path     string
}

func (c *childList) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		return json.Unmarshal(b, &c.IDs)
	}
	var t struct {
		ComponentID string `json:"componentId"`
		Path        string `json:"path"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	if t.ComponentID == "" || t.Path == "" {
		return fmt.Errorf("a template child list needs componentId and path")
	}
	c.Template, c.Path = t.ComponentID, t.Path
	return nil
}

type stackProps struct {
	common
	Children childList `json:"children"`
}

type splitProps struct {
	common
	Left  string  `json:"left"`
	Right string  `json:"right"`
	Ratio float64 `json:"ratio"`
}

type tabsProps struct {
	common
	Tabs []struct {
		Title json.RawMessage `json:"title"`
		Child string          `json:"child"`
	} `json:"tabs"`
}

type textProps struct {
	common
	Text    json.RawMessage `json:"text"`
	Style   string          `json:"style"`
	Variant string          `json:"variant"` // basic catalog: h1…h5, caption, body
}

type listProps struct {
	common
	Children childList `json:"children"`
	// ItemKey names the field of each data item that identifies it, so the
	// cursor follows the item, not the row, when the agent reorders the
	// array (E3). Without it the index is the identity.
	ItemKey string `json:"itemKey"`
	// Selected is two-way: the renderer writes the selected item's key there.
	Selected *struct {
		Path string `json:"path"`
	} `json:"selected"`
}

type tableProps struct {
	common
	Rows struct {
		Path string `json:"path"`
	} `json:"rows"`
	ItemKey string `json:"itemKey"`
	Columns []struct {
		Title  string          `json:"title"`
		Value  json.RawMessage `json:"value"`
		Status bool            `json:"status"`
	} `json:"columns"`
}

type detailProps struct {
	common
	Fields []struct {
		Label string          `json:"label"`
		Value json.RawMessage `json:"value"`
	} `json:"fields"`
}

type inputProps struct {
	common
	Label       json.RawMessage `json:"label"`
	Value       json.RawMessage `json:"value"`
	Placeholder string          `json:"placeholder"`
}

// actionSpec is common_types.json Action: an event for the agent, or a
// local functionCall. Only openUrl runs locally.
type actionSpec struct {
	Event *struct {
		Name    string                     `json:"name"`
		Context map[string]json.RawMessage `json:"context"`
	} `json:"event"`
	FunctionCall *struct {
		Call string         `json:"call"`
		Args map[string]any `json:"args"`
	} `json:"functionCall"`
}

type buttonProps struct {
	common
	Label json.RawMessage `json:"label"`
	Child string          `json:"child"` // basic catalog: a Text child holds the label
	// Key is the preferred key. The renderer binds it only when it clashes
	// with nothing; otherwise the action is reachable from the palette.
	Key     string     `json:"key"`
	Confirm bool       `json:"confirm"`
	Danger  bool       `json:"danger"`
	Action  actionSpec `json:"action"`
}

type badgeProps struct {
	common
	Text   json.RawMessage `json:"text"`
	Status json.RawMessage `json:"status"`
}

type linkProps struct {
	common
	Text json.RawMessage `json:"text"`
	URL  json.RawMessage `json:"url"`
}

type progressProps struct {
	common
	Value json.RawMessage `json:"value"`
	Label json.RawMessage `json:"label"`
}

type cardProps struct {
	common
	Child string `json:"child"`
}

func decode[T any](c Component) (T, error) {
	var p T
	if err := json.Unmarshal(c.Raw, &p); err != nil {
		return p, fmt.Errorf("%s %q: %w", c.Type, c.ID, err)
	}
	return p, nil
}

func commonOf(c Component) common {
	var p common
	if err := json.Unmarshal(c.Raw, &p); err != nil {
		return common{} // the component's own draw reports the error
	}
	return p
}

// tone is a semantic style token: the only styling an agent can ask for.
// Raw colors are not part of the catalog, so the theme (and NO_COLOR) stay
// in charge.
type tone string

const (
	toneNone   tone = ""
	toneMuted  tone = "muted"
	toneAccent tone = "accent"
	toneOK     tone = "ok"
	toneWarn   tone = "warn"
	toneErr    tone = "err"
)

// toneOf maps a token, or a status word an agent is likely to send, to a
// tone. Unknown words are muted rather than an error: status text comes from
// data, and data is not validated against the catalog.
func toneOf(s string) tone {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ok", "success", "succeeded", "pass", "passed", "done", "merged", "open", "approved":
		return toneOK
	case "warn", "warning", "pending", "running", "queued", "in_progress", "draft", "changes_requested":
		return toneWarn
	case "err", "error", "fail", "failed", "failure", "cancelled", "closed":
		return toneErr
	case "accent", "h1", "h2", "h3":
		return toneAccent
	case "", "body", "h4", "h5":
		return toneNone
	}
	return toneMuted
}
