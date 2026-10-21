package surface

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// value resolves a Dynamic* property (common_types.json): a literal, a
// {"path": …} binding, or a {"call": …} function call, evaluated in scope.
// A path that does not resolve yet is nil, which draws as "": data may
// arrive after the components (progressive rendering).
func value(data any, raw json.RawMessage, scope string) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return eval(data, v, scope)
}

func eval(data, v any, scope string) (any, error) {
	obj, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	if p, ok := obj["path"].(string); ok && len(obj) == 1 {
		got, _ := lookup(data, join(scope, p)) // missing is nil: not loaded yet
		return got, nil
	}
	if name, ok := obj["call"].(string); ok {
		args, _ := obj["args"].(map[string]any) // absent args are an empty set
		return call(data, name, args, scope)
	}
	return v, nil
}

// call runs a client-side function. Only functions that are pure and need no
// locale tables are implemented; the others are reported, so the component
// that used one shows why instead of a silently wrong value.
func call(data any, name string, args map[string]any, scope string) (any, error) {
	arg := func(k string) (any, error) { return eval(data, args[k], scope) }
	switch name {
	case "formatString":
		v, err := arg("value")
		if err != nil {
			return nil, err
		}
		s, _ := v.(string)
		return formatString(data, s, scope), nil
	case "required":
		v, err := arg("value")
		if err != nil {
			return nil, err
		}
		return v != nil && v != "" && !isEmptyList(v), nil
	case "not":
		v, err := arg("value")
		if err != nil {
			return nil, err
		}
		b, _ := v.(bool)
		return !b, nil
	case "and", "or":
		list, _ := args["values"].([]any)
		acc := name == "and"
		for _, x := range list {
			v, err := eval(data, x, scope)
			if err != nil {
				return nil, err
			}
			b, _ := v.(bool)
			if name == "and" {
				acc = acc && b
			} else {
				acc = acc || b
			}
		}
		return acc, nil
	}
	return nil, fmt.Errorf("function %q is not supported by the terminal renderer", name)
}

func isEmptyList(v any) bool {
	l, ok := v.([]any)
	return ok && len(l) == 0
}

// formatString interpolates ${/abs/path} and ${relative} from the data
// model; \${ is a literal "${". Function calls inside ${…} (they contain
// "(") are left as written: nesting them needs the expression parser this
// PoC does not have.
func formatString(data any, s, scope string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		if i > 0 && s[i-1] == '\\' {
			b.WriteString(s[:i-1] + "${")
			s = s[i+2:]
			continue
		}
		end := strings.Index(s[i:], "}")
		if end < 0 {
			b.WriteString(s)
			return b.String()
		}
		expr := s[i+2 : i+end]
		b.WriteString(s[:i])
		if strings.Contains(expr, "(") {
			b.WriteString(s[i : i+end+1])
		} else {
			v, _ := lookup(data, join(scope, expr))
			b.WriteString(stringify(v))
		}
		s = s[i+end+1:]
	}
}

// stringify follows the spec's type conversion: numbers and booleans in
// their usual form, null as "", objects and arrays as JSON.
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// text resolves a DynamicString property to its display text.
func text(data any, raw json.RawMessage, scope string) (string, error) {
	v, err := value(data, raw, scope)
	return stringify(v), err
}

// number resolves a DynamicNumber; a numeric string is accepted, since
// agents often quote numbers.
func number(data any, raw json.RawMessage, scope string) (float64, bool, error) {
	v, err := value(data, raw, scope)
	if err != nil {
		return 0, false, err
	}
	switch x := v.(type) {
	case float64:
		return x, true, nil
	case string:
		f, perr := strconv.ParseFloat(x, 64)
		return f, perr == nil, nil
	}
	return 0, false, nil
}

// resolveContext resolves an action's context map: every value may be a
// literal, a path or a call, all evaluated in the action's scope.
func resolveContext(data any, ctx map[string]json.RawMessage, scope string) (map[string]any, error) {
	out := make(map[string]any, len(ctx))
	for k, raw := range ctx {
		v, err := value(data, raw, scope)
		if err != nil {
			return nil, fmt.Errorf("context %q: %w", k, err)
		}
		out[k] = v
	}
	return out, nil
}
