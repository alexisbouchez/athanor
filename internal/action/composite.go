package action

import (
	"context"
	"fmt"
	"strings"

	"github.com/alexj212/athanor/internal/expr"
)

// CompositeResult holds the result of running a composite action.
type CompositeResult struct {
	Outputs map[string]string
	Failed  bool
}

// ExecCompositeFunc is the function signature for executing a single composite step.
// It mirrors the runner's step execution but allows the composite runner to stay decoupled.
type ExecCompositeFunc func(ctx context.Context, step CompositeStep, env map[string]string, inputs map[string]string) (outputs map[string]string, exitCode int, err error)

// RunComposite executes a composite action's steps and evaluates its outputs.
func RunComposite(ctx context.Context, meta *ActionMetadata, with map[string]string, execFn ExecCompositeFunc) (*CompositeResult, error) {
	if meta.Runs.Using != "composite" {
		return nil, fmt.Errorf("RunComposite called on non-composite action (using=%q)", meta.Runs.Using)
	}

	// Build inputs from with + defaults
	inputs := make(map[string]string)
	for name, def := range meta.Inputs {
		if val, ok := with[name]; ok {
			inputs[name] = val
		} else if def.Default != "" {
			inputs[name] = def.Default
		}
	}

	// Build INPUT_* env vars
	inputEnv := make(map[string]string, len(inputs))
	for k, v := range inputs {
		inputEnv["INPUT_"+strings.ToUpper(strings.ReplaceAll(k, "-", "_"))] = v
	}

	allOutputs := make(map[string]map[string]string) // stepID -> outputs
	failed := false

	for _, step := range meta.Runs.Steps {
		if ctx.Err() != nil {
			break
		}
		outputs, exitCode, err := execFn(ctx, step, inputEnv, inputs)
		if err != nil {
			return nil, fmt.Errorf("composite step %q: %w", step.Name, err)
		}
		if step.ID != "" && outputs != nil {
			allOutputs[step.ID] = outputs
		}
		if exitCode != 0 {
			failed = true
			break
		}
	}

	// Evaluate action output expressions
	result := &CompositeResult{
		Outputs: make(map[string]string),
		Failed:  failed,
	}

	// Build context for output expressions
	stepsCtx := make(map[string]any, len(allOutputs))
	for id, outs := range allOutputs {
		outsMap := make(map[string]any, len(outs))
		for k, v := range outs {
			outsMap[k] = v
		}
		stepsCtx[id] = map[string]any{"outputs": outsMap}
	}
	exprCtx := map[string]any{
		"steps": stepsCtx,
	}

	for name, def := range meta.Outputs {
		if def.Value != "" {
			val, err := expr.Interpolate(def.Value, exprCtx)
			if err != nil {
				return nil, fmt.Errorf("evaluating output %q: %w", name, err)
			}
			result.Outputs[name] = val
		}
	}

	return result, nil
}
