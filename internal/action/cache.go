package action

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// DefaultCacheDir returns the default cache directory for actions.
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".cache", "athanor", "actions")
}

// Fetch clones and caches a GitHub action, returning the local path to the action directory.
// For local refs, it resolves relative to workspace.
func Fetch(ctx context.Context, ref ActionRef, cacheDir, workspace string) (string, error) {
	switch ref.Type {
	case "local":
		return filepath.Join(workspace, ref.Path), nil

	case "docker":
		return "", fmt.Errorf("docker actions are not supported")

	case "github":
		dir := filepath.Join(cacheDir, ref.Owner, ref.Repo, ref.Version)

		// Already cached?
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if ref.Path != "" {
				return filepath.Join(dir, ref.Path), nil
			}
			return dir, nil
		}

		// Clone
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", fmt.Errorf("creating cache dir: %w", err)
		}

		cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", ref.Owner, ref.Repo)
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", ref.Version, cloneURL, dir)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Clean up partial clone
			os.RemoveAll(dir)
			return "", fmt.Errorf("cloning %s: %w", ref, err)
		}

		if ref.Path != "" {
			return filepath.Join(dir, ref.Path), nil
		}
		return dir, nil

	default:
		return "", fmt.Errorf("unknown action type %q", ref.Type)
	}
}
