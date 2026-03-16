package expr

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Evaluate evaluates an AST node against the given context.
func Evaluate(node Node, ctx map[string]any) (any, error) {
	switch n := node.(type) {
	case Literal:
		return n.Value, nil

	case Context:
		return contextLookup(ctx, n.Parts), nil

	case Index:
		obj, err := Evaluate(n.Object, ctx)
		if err != nil {
			return nil, err
		}
		key, err := Evaluate(n.Key, ctx)
		if err != nil {
			return nil, err
		}
		return indexLookup(obj, key), nil

	case FuncCall:
		return callFunction(n.Name, n.Args, ctx)

	case BinaryOp:
		return evalBinary(n.Op, n.Left, n.Right, ctx)

	case UnaryOp:
		if n.Op == "!" {
			val, err := Evaluate(n.Operand, ctx)
			if err != nil {
				return nil, err
			}
			return !isTruthy(val), nil
		}
		return nil, fmt.Errorf("unknown unary operator %q", n.Op)

	default:
		return nil, fmt.Errorf("unknown node type %T", node)
	}
}

func contextLookup(ctx map[string]any, parts []string) any {
	var current any = ctx
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}
	return current
}

func indexLookup(obj any, key any) any {
	switch o := obj.(type) {
	case map[string]any:
		k := toString(key)
		return o[k]
	case []any:
		idx, ok := toFloat(key)
		if !ok {
			return nil
		}
		i := int(idx)
		if i >= 0 && i < len(o) {
			return o[i]
		}
		return nil
	}
	return nil
}

func evalBinary(op string, left, right Node, ctx map[string]any) (any, error) {
	// Short-circuit for logical operators
	if op == "||" {
		l, err := Evaluate(left, ctx)
		if err != nil {
			return nil, err
		}
		if isTruthy(l) {
			return l, nil
		}
		return Evaluate(right, ctx)
	}
	if op == "&&" {
		l, err := Evaluate(left, ctx)
		if err != nil {
			return nil, err
		}
		if !isTruthy(l) {
			return l, nil
		}
		return Evaluate(right, ctx)
	}

	l, err := Evaluate(left, ctx)
	if err != nil {
		return nil, err
	}
	r, err := Evaluate(right, ctx)
	if err != nil {
		return nil, err
	}

	switch op {
	case "==":
		return equals(l, r), nil
	case "!=":
		return !equals(l, r), nil
	case "<":
		return compare(l, r) < 0, nil
	case ">":
		return compare(l, r) > 0, nil
	case "<=":
		return compare(l, r) <= 0, nil
	case ">=":
		return compare(l, r) >= 0, nil
	default:
		return nil, fmt.Errorf("unknown operator %q", op)
	}
}

// isTruthy matches GitHub Actions truthiness: "", 0, false, nil are falsy.
func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	case float64:
		return val != 0
	case int:
		return val != 0
	}
	return true
}

func equals(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Try numeric comparison
	af, aOk := toFloat(a)
	bf, bOk := toFloat(b)
	if aOk && bOk {
		return af == bf
	}
	// String comparison
	return toString(a) == toString(b)
}

func compare(a, b any) int {
	af, aOk := toFloat(a)
	bf, bOk := toFloat(b)
	if aOk && bOk {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	as := toString(a)
	bs := toString(b)
	switch {
	case as < bs:
		return -1
	case as > bs:
		return 1
	default:
		return 0
	}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func callFunction(name string, args []Node, ctx map[string]any) (any, error) {
	switch name {
	case "contains":
		if len(args) != 2 {
			return nil, fmt.Errorf("contains() requires 2 arguments")
		}
		a, err := Evaluate(args[0], ctx)
		if err != nil {
			return nil, err
		}
		b, err := Evaluate(args[1], ctx)
		if err != nil {
			return nil, err
		}
		// If a is an array, check membership
		if arr, ok := a.([]any); ok {
			bs := toString(b)
			for _, item := range arr {
				if strings.EqualFold(toString(item), bs) {
					return true, nil
				}
			}
			return false, nil
		}
		return strings.Contains(
			strings.ToLower(toString(a)),
			strings.ToLower(toString(b)),
		), nil

	case "startsWith":
		if len(args) != 2 {
			return nil, fmt.Errorf("startsWith() requires 2 arguments")
		}
		a, err := Evaluate(args[0], ctx)
		if err != nil {
			return nil, err
		}
		b, err := Evaluate(args[1], ctx)
		if err != nil {
			return nil, err
		}
		return strings.HasPrefix(
			strings.ToLower(toString(a)),
			strings.ToLower(toString(b)),
		), nil

	case "endsWith":
		if len(args) != 2 {
			return nil, fmt.Errorf("endsWith() requires 2 arguments")
		}
		a, err := Evaluate(args[0], ctx)
		if err != nil {
			return nil, err
		}
		b, err := Evaluate(args[1], ctx)
		if err != nil {
			return nil, err
		}
		return strings.HasSuffix(
			strings.ToLower(toString(a)),
			strings.ToLower(toString(b)),
		), nil

	case "format":
		if len(args) < 1 {
			return nil, fmt.Errorf("format() requires at least 1 argument")
		}
		fmtVal, err := Evaluate(args[0], ctx)
		if err != nil {
			return nil, err
		}
		fmtStr := toString(fmtVal)
		vals := make([]string, len(args)-1)
		for i := 1; i < len(args); i++ {
			v, err := Evaluate(args[i], ctx)
			if err != nil {
				return nil, err
			}
			vals[i-1] = toString(v)
		}
		// Replace {0}, {1}, etc.
		result := fmtStr
		for i, v := range vals {
			result = strings.ReplaceAll(result, fmt.Sprintf("{%d}", i), v)
		}
		return result, nil

	case "join":
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("join() requires 1 or 2 arguments")
		}
		a, err := Evaluate(args[0], ctx)
		if err != nil {
			return nil, err
		}
		sep := ","
		if len(args) == 2 {
			s, err := Evaluate(args[1], ctx)
			if err != nil {
				return nil, err
			}
			sep = toString(s)
		}
		arr, ok := a.([]any)
		if !ok {
			return toString(a), nil
		}
		parts := make([]string, len(arr))
		for i, v := range arr {
			parts[i] = toString(v)
		}
		return strings.Join(parts, sep), nil

	case "toJSON":
		if len(args) != 1 {
			return nil, fmt.Errorf("toJSON() requires 1 argument")
		}
		v, err := Evaluate(args[0], ctx)
		if err != nil {
			return nil, err
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, err
		}
		return string(b), nil

	case "fromJSON":
		if len(args) != 1 {
			return nil, fmt.Errorf("fromJSON() requires 1 argument")
		}
		v, err := Evaluate(args[0], ctx)
		if err != nil {
			return nil, err
		}
		var result any
		if err := json.Unmarshal([]byte(toString(v)), &result); err != nil {
			return nil, fmt.Errorf("fromJSON: %w", err)
		}
		return result, nil

	case "hashFiles":
		// Stub: not meaningful outside a real workspace
		return "", nil

	case "success":
		status, _ := ctx["__job_status"].(string)
		if status == "" {
			status = "success"
		}
		return status == "success", nil

	case "failure":
		status, _ := ctx["__job_status"].(string)
		return status == "failure", nil

	case "always":
		return true, nil

	case "cancelled":
		cancelled, _ := ctx["__cancelled"].(bool)
		return cancelled, nil

	default:
		return nil, fmt.Errorf("unknown function %q", name)
	}
}
