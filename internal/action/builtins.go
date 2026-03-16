package action

import (
	"context"
	"fmt"
	"os/exec"
)

// BuiltinFunc is a function that implements a built-in action.
// It receives the action inputs (with:) and workspace directory,
// and returns step outputs and an error.
type BuiltinFunc func(ctx context.Context, inputs map[string]string, workspace string) (outputs map[string]string, err error)

// Builtins maps action names (owner/repo) to built-in implementations.
var Builtins = map[string]BuiltinFunc{
	"actions/checkout":   builtinCheckout,
	"actions/setup-node": builtinSetupNode,
	"actions/setup-go":   builtinSetupGo,
}

// LookupBuiltin checks if a GitHub action has a built-in implementation.
func LookupBuiltin(ref ActionRef) (BuiltinFunc, bool) {
	if ref.Type != "github" {
		return nil, false
	}
	key := ref.Owner + "/" + ref.Repo
	fn, ok := Builtins[key]
	return fn, ok
}

// builtinCheckout implements a minimal actions/checkout.
// For local runs, the workspace is typically already checked out.
// It handles the ref: input to switch branches.
func builtinCheckout(ctx context.Context, inputs map[string]string, workspace string) (map[string]string, error) {
	ref := inputs["ref"]
	fetchDepth := inputs["fetch-depth"]

	if fetchDepth != "" && fetchDepth != "0" && fetchDepth != "1" {
		// Fetch more history if requested
		cmd := exec.CommandContext(ctx, "git", "fetch", "--depth", fetchDepth)
		cmd.Dir = workspace
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("git fetch: %w", err)
		}
	} else if fetchDepth == "0" {
		// Unshallow
		cmd := exec.CommandContext(ctx, "git", "fetch", "--unshallow")
		cmd.Dir = workspace
		// Ignore error — may already be complete
		cmd.Run()
	}

	if ref != "" {
		cmd := exec.CommandContext(ctx, "git", "checkout", ref)
		cmd.Dir = workspace
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("git checkout %s: %w", ref, err)
		}
	}

	return nil, nil
}

// builtinSetupNode is a no-op shim for actions/setup-node.
// Node.js is pre-installed in the VM rootfs.
func builtinSetupNode(_ context.Context, _ map[string]string, _ string) (map[string]string, error) {
	return nil, nil
}

// builtinSetupGo is a no-op shim for actions/setup-go.
// Go is pre-installed in the VM rootfs.
func builtinSetupGo(_ context.Context, _ map[string]string, _ string) (map[string]string, error) {
	return nil, nil
}
