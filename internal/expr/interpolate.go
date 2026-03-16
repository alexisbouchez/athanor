package expr

import (
	"fmt"
	"strings"
)

// Interpolate finds ${{ ... }} delimiters in s, evaluates the expressions,
// and returns the string with results spliced in.
func Interpolate(s string, ctx map[string]any) (string, error) {
	var result strings.Builder
	i := 0
	for i < len(s) {
		// Look for ${{
		idx := strings.Index(s[i:], "${{")
		if idx == -1 {
			result.WriteString(s[i:])
			break
		}
		result.WriteString(s[i : i+idx])
		i += idx + 3

		// Find matching }}
		end := strings.Index(s[i:], "}}")
		if end == -1 {
			return "", fmt.Errorf("unterminated expression at position %d", i-3)
		}
		exprStr := strings.TrimSpace(s[i : i+end])
		i += end + 2

		// Tokenize and parse
		tokens, err := Lex(exprStr)
		if err != nil {
			return "", fmt.Errorf("lexing expression %q: %w", exprStr, err)
		}
		node, err := Parse(tokens)
		if err != nil {
			return "", fmt.Errorf("parsing expression %q: %w", exprStr, err)
		}
		val, err := Evaluate(node, ctx)
		if err != nil {
			return "", fmt.Errorf("evaluating expression %q: %w", exprStr, err)
		}
		result.WriteString(exprToString(val))
	}
	return result.String(), nil
}

// EvaluateExpression is a convenience that lexes, parses, and evaluates an expression string.
func EvaluateExpression(exprStr string, ctx map[string]any) (any, error) {
	tokens, err := Lex(exprStr)
	if err != nil {
		return nil, err
	}
	node, err := Parse(tokens)
	if err != nil {
		return nil, err
	}
	return Evaluate(node, ctx)
}

// exprToString converts an evaluation result to a string suitable for interpolation.
func exprToString(v any) string {
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
