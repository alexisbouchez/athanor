package expr

import (
	"testing"
)

func TestLexBasic(t *testing.T) {
	tokens, err := Lex("github.sha == 'abc123'")
	if err != nil {
		t.Fatal(err)
	}
	// github, ., sha, ==, 'abc123', EOF
	if len(tokens) != 6 {
		t.Fatalf("got %d tokens, want 6: %v", len(tokens), tokens)
	}
	if tokens[0].Type != TokenIdent || tokens[0].Value != "github" {
		t.Errorf("token 0 = %v", tokens[0])
	}
	if tokens[3].Type != TokenEq {
		t.Errorf("token 3 = %v", tokens[3])
	}
	if tokens[4].Type != TokenString || tokens[4].Value != "abc123" {
		t.Errorf("token 4 = %v", tokens[4])
	}
}

func TestLexOperators(t *testing.T) {
	tokens, err := Lex("a != b && c || !d <= 5 >= 3 < 2 > 1")
	if err != nil {
		t.Fatal(err)
	}
	expected := []TokenType{
		TokenIdent, TokenNeq, TokenIdent, TokenAnd, TokenIdent,
		TokenOr, TokenNot, TokenIdent, TokenLe, TokenNumber,
		TokenGe, TokenNumber, TokenLt, TokenNumber, TokenGt, TokenNumber, TokenEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, exp := range expected {
		if tokens[i].Type != exp {
			t.Errorf("token %d: got type %d, want %d (value=%q)", i, tokens[i].Type, exp, tokens[i].Value)
		}
	}
}

func TestParseAndEvalContext(t *testing.T) {
	ctx := map[string]any{
		"github": map[string]any{
			"sha": "abc123",
			"ref": "refs/heads/main",
		},
	}

	val, err := EvaluateExpression("github.sha", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != "abc123" {
		t.Errorf("got %v, want abc123", val)
	}
}

func TestEvalComparison(t *testing.T) {
	ctx := map[string]any{
		"github": map[string]any{
			"ref": "refs/heads/main",
		},
	}

	val, err := EvaluateExpression("github.ref == 'refs/heads/main'", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("got %v, want true", val)
	}

	val, err = EvaluateExpression("github.ref != 'refs/heads/main'", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("got %v, want false", val)
	}
}

func TestEvalLogical(t *testing.T) {
	ctx := map[string]any{}

	val, err := EvaluateExpression("true && false", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if isTruthy(val) {
		t.Errorf("true && false should be falsy, got %v", val)
	}

	val, err = EvaluateExpression("true || false", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !isTruthy(val) {
		t.Errorf("true || false should be truthy, got %v", val)
	}
}

func TestEvalNot(t *testing.T) {
	ctx := map[string]any{}
	val, err := EvaluateExpression("!true", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("!true should be false, got %v", val)
	}

	val, err = EvaluateExpression("!false", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("!false should be true, got %v", val)
	}
}

func TestEvalTruthiness(t *testing.T) {
	ctx := map[string]any{}

	tests := []struct {
		expr string
		want bool
	}{
		{"''", false},
		{"'hello'", true},
		{"0", false},
		{"1", true},
		{"true", true},
		{"false", false},
		{"null", false},
	}
	for _, tt := range tests {
		val, err := EvaluateExpression(tt.expr, ctx)
		if err != nil {
			t.Errorf("evaluating %q: %v", tt.expr, err)
			continue
		}
		if isTruthy(val) != tt.want {
			t.Errorf("isTruthy(%q) = %v, want %v (val=%v)", tt.expr, isTruthy(val), tt.want, val)
		}
	}
}

func TestEvalContains(t *testing.T) {
	ctx := map[string]any{}

	val, err := EvaluateExpression("contains('Hello World', 'hello')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("contains should be case-insensitive, got %v", val)
	}

	// Array form
	ctx["arr"] = []any{"apple", "banana", "cherry"}
	val, err = EvaluateExpression("contains(arr, 'banana')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("contains(arr, 'banana') should be true")
	}
}

func TestEvalStartsEndsWith(t *testing.T) {
	ctx := map[string]any{}

	val, err := EvaluateExpression("startsWith('Hello World', 'hello')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("startsWith should be case-insensitive, got %v", val)
	}

	val, err = EvaluateExpression("endsWith('Hello World', 'WORLD')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("endsWith should be case-insensitive, got %v", val)
	}
}

func TestEvalFormat(t *testing.T) {
	ctx := map[string]any{}
	val, err := EvaluateExpression("format('Hello {0}, you are {1}!', 'World', 'great')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != "Hello World, you are great!" {
		t.Errorf("format = %q", val)
	}
}

func TestEvalJoin(t *testing.T) {
	ctx := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	val, err := EvaluateExpression("join(items, ', ')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != "a, b, c" {
		t.Errorf("join = %q", val)
	}
}

func TestEvalToFromJSON(t *testing.T) {
	ctx := map[string]any{
		"data": map[string]any{"key": "value"},
	}
	val, err := EvaluateExpression("toJSON(data)", ctx)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("toJSON should return string, got %T", val)
	}
	if s != "{\n  \"key\": \"value\"\n}" {
		t.Errorf("toJSON = %q", s)
	}

	val, err = EvaluateExpression("fromJSON('{\"x\": 42}')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("fromJSON should return map, got %T", val)
	}
	if m["x"] != float64(42) {
		t.Errorf("fromJSON x = %v", m["x"])
	}
}

func TestEvalStatusFunctions(t *testing.T) {
	ctx := map[string]any{
		"__job_status": "success",
	}
	val, err := EvaluateExpression("success()", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("success() should be true when status is success")
	}

	val, err = EvaluateExpression("failure()", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Error("failure() should be false when status is success")
	}

	val, err = EvaluateExpression("always()", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("always() should always be true")
	}

	ctx["__job_status"] = "failure"
	val, err = EvaluateExpression("failure()", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("failure() should be true when status is failure")
	}
}

func TestEvalIndexAccess(t *testing.T) {
	ctx := map[string]any{
		"matrix": map[string]any{
			"os": "ubuntu-latest",
		},
	}
	val, err := EvaluateExpression("matrix['os']", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != "ubuntu-latest" {
		t.Errorf("matrix['os'] = %v", val)
	}
}

func TestEvalNestedMap(t *testing.T) {
	ctx := map[string]any{
		"steps": map[string]any{
			"build": map[string]any{
				"outputs": map[string]any{
					"version": "1.2.3",
				},
			},
		},
	}
	val, err := EvaluateExpression("steps.build.outputs.version", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != "1.2.3" {
		t.Errorf("got %v, want 1.2.3", val)
	}
}

func TestEvalComparisons(t *testing.T) {
	ctx := map[string]any{}

	tests := []struct {
		expr string
		want bool
	}{
		{"1 < 2", true},
		{"2 < 1", false},
		{"1 <= 1", true},
		{"2 > 1", true},
		{"1 >= 1", true},
		{"1 >= 2", false},
	}
	for _, tt := range tests {
		val, err := EvaluateExpression(tt.expr, ctx)
		if err != nil {
			t.Errorf("eval %q: %v", tt.expr, err)
			continue
		}
		b, ok := val.(bool)
		if !ok || b != tt.want {
			t.Errorf("%q = %v, want %v", tt.expr, val, tt.want)
		}
	}
}

func TestInterpolate(t *testing.T) {
	ctx := map[string]any{
		"github": map[string]any{
			"sha":     "abc123",
			"ref":     "refs/heads/main",
			"run_id":  "42",
		},
	}

	tests := []struct {
		input string
		want  string
	}{
		{"no expressions", "no expressions"},
		{"sha=${{ github.sha }}", "sha=abc123"},
		{"${{ github.sha }} and ${{ github.ref }}", "abc123 and refs/heads/main"},
		{"id-${{ github.run_id }}-done", "id-42-done"},
	}
	for _, tt := range tests {
		got, err := Interpolate(tt.input, ctx)
		if err != nil {
			t.Errorf("Interpolate(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Interpolate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInterpolateMixedContent(t *testing.T) {
	ctx := map[string]any{
		"env": map[string]any{
			"NAME": "world",
		},
	}
	got, err := Interpolate("echo Hello ${{ env.NAME }}!", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo Hello world!" {
		t.Errorf("got %q", got)
	}
}

func TestEvalNullContext(t *testing.T) {
	ctx := map[string]any{}
	val, err := EvaluateExpression("github.sha", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Errorf("missing context should return nil, got %v", val)
	}
}

func TestEvalCancelled(t *testing.T) {
	ctx := map[string]any{
		"__cancelled": true,
	}
	val, err := EvaluateExpression("cancelled()", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("cancelled() should be true")
	}
}

func TestEvalNegativeNumber(t *testing.T) {
	ctx := map[string]any{}
	val, err := EvaluateExpression("-1", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val != float64(-1) {
		t.Errorf("got %v, want -1", val)
	}
}

func TestEvalGrouping(t *testing.T) {
	ctx := map[string]any{}
	val, err := EvaluateExpression("(true || false) && false", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if isTruthy(val) {
		t.Error("(true || false) && false should be falsy")
	}
}
