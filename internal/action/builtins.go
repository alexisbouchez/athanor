package action

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuiltinFunc is a function that implements a built-in action.
// It receives the action inputs (with:) and workspace directory,
// and returns step outputs and an error.
type BuiltinFunc func(ctx context.Context, inputs map[string]string, workspace string) (outputs map[string]string, err error)

// Builtins maps action names (owner/repo) to built-in implementations.
var Builtins = map[string]BuiltinFunc{
	"actions/checkout":          builtinCheckout,
	"actions/setup-node":        builtinSetupNode,
	"actions/setup-go":          builtinSetupGo,
	"actions/upload-artifact":   builtinUploadArtifact,
	"actions/download-artifact": builtinDownloadArtifact,
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

// ArtifactDir is the shared directory for artifacts between jobs.
// Set by the server before running workflows.
var ArtifactDir string

// builtinUploadArtifact copies files to the artifact store.
func builtinUploadArtifact(_ context.Context, inputs map[string]string, workspace string) (map[string]string, error) {
	name := inputs["name"]
	if name == "" {
		name = "artifact"
	}
	path := inputs["path"]
	if path == "" {
		return nil, fmt.Errorf("upload-artifact: path is required")
	}

	destDir := filepath.Join(ArtifactDir, name)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating artifact dir: %w", err)
	}

	// Expand glob patterns
	var files []string
	for _, pattern := range strings.Split(path, "\n") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		absPattern := pattern
		if !filepath.IsAbs(pattern) {
			absPattern = filepath.Join(workspace, pattern)
		}
		matches, err := filepath.Glob(absPattern)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", pattern, err)
		}
		files = append(files, matches...)
	}

	for _, src := range files {
		rel, _ := filepath.Rel(workspace, src)
		dst := filepath.Join(destDir, rel)
		os.MkdirAll(filepath.Dir(dst), 0o755)
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", dst, err)
		}
	}

	return map[string]string{"artifact-id": name}, nil
}

// builtinDownloadArtifact copies files from the artifact store to the workspace.
func builtinDownloadArtifact(_ context.Context, inputs map[string]string, workspace string) (map[string]string, error) {
	name := inputs["name"]
	if name == "" {
		return nil, fmt.Errorf("download-artifact: name is required")
	}

	srcDir := filepath.Join(ArtifactDir, name)
	destPath := inputs["path"]
	if destPath == "" {
		destPath = workspace
	} else if !filepath.IsAbs(destPath) {
		destPath = filepath.Join(workspace, destPath)
	}

	return nil, copyDir(srcDir, destPath)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
