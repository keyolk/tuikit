package surface

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// The data model is plain decoded JSON: map[string]any, []any, string,
// float64, bool, nil. Writes copy every container on the path they touch, so
// a model value kept by an older Bubble Tea copy never sees a later write.

// tokens splits an RFC 6901 pointer. "" and "/" both name the root, as
// updateDataModel's path does.
func tokens(p string) []string {
	if p == "" || p == "/" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
	}
	return parts
}

func escapeToken(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// join resolves p against scope, the absolute pointer of the data item a
// template is instantiating. A2UI extends RFC 6901: a path without a leading
// slash is relative to the collection scope (a2ui_protocol.md, "Collection
// scopes"). Outside any template, scope is "" and a relative path is
// resolved from the root.
func join(scope, p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	if p == "" {
		if scope == "" {
			return "/"
		}
		return scope
	}
	return strings.TrimSuffix(scope, "/") + "/" + p
}

func lookup(root any, p string) (any, bool) {
	cur := root
	for _, t := range tokens(p) {
		switch c := cur.(type) {
		case map[string]any:
			v, ok := c[t]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(t)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// setPath returns root with v at p, creating objects along the way. On an
// array a token must be an index, or len to append; anything else is an
// error, as the spec says a non-numeric index on an array is.
func setPath(root any, p string, v any) (any, error) {
	return setAt(root, tokens(p), v, false)
}

// removePath returns root without the key at p. An array element is set to
// nil rather than cut, preserving the length (a2ui_protocol.md, "Server to
// client updates").
func removePath(root any, p string) (any, error) {
	if len(tokens(p)) == 0 {
		return nil, nil
	}
	return setAt(root, tokens(p), nil, true)
}

func setAt(cur any, toks []string, v any, remove bool) (any, error) {
	if len(toks) == 0 {
		return v, nil
	}
	t, rest := toks[0], toks[1:]
	switch c := cur.(type) {
	case nil:
		if remove {
			return nil, nil
		}
		return setAt(map[string]any{}, toks, v, remove)
	case map[string]any:
		out := maps.Clone(c)
		if remove && len(rest) == 0 {
			delete(out, t)
			return out, nil
		}
		child, err := setAt(c[t], rest, v, remove)
		if err != nil {
			return nil, err
		}
		out[t] = child
		return out, nil
	case []any:
		i, err := strconv.Atoi(t)
		if err != nil || i < 0 || i > len(c) {
			return nil, fmt.Errorf("index %q is not valid for an array of %d", t, len(c))
		}
		out := slices.Clone(c)
		if i == len(c) {
			if remove {
				return out, nil
			}
			out = append(out, nil)
		}
		child, err := setAt(out[i], rest, v, remove)
		if err != nil {
			return nil, err
		}
		out[i] = child
		return out, nil
	default:
		if remove {
			return cur, nil
		}
		return nil, fmt.Errorf("cannot descend into %T at %q", cur, t)
	}
}
