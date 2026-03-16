package action

import (
	"fmt"
	"strings"
)

// ActionRef describes a parsed uses: reference.
type ActionRef struct {
	Type    string // "github", "local", "docker"
	Owner   string
	Repo    string
	Version string
	Path    string // subdirectory within repo, or local path
}

// ParseRef parses a uses: string into an ActionRef.
func ParseRef(uses string) (ActionRef, error) {
	if uses == "" {
		return ActionRef{}, fmt.Errorf("empty uses: reference")
	}

	// Local action: ./path/to/action
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../") {
		return ActionRef{
			Type: "local",
			Path: uses,
		}, nil
	}

	// Docker action: docker://image:tag
	if strings.HasPrefix(uses, "docker://") {
		return ActionRef{
			Type: "docker",
			Path: strings.TrimPrefix(uses, "docker://"),
		}, nil
	}

	// GitHub action: owner/repo@version or owner/repo/path@version
	ref := ActionRef{Type: "github"}

	// Split on @
	parts := strings.SplitN(uses, "@", 2)
	if len(parts) != 2 || parts[1] == "" {
		return ActionRef{}, fmt.Errorf("github action must have @version: %q", uses)
	}
	ref.Version = parts[1]

	// Split the path part: owner/repo or owner/repo/subdir
	segments := strings.SplitN(parts[0], "/", 3)
	if len(segments) < 2 {
		return ActionRef{}, fmt.Errorf("github action must be owner/repo: %q", uses)
	}
	ref.Owner = segments[0]
	ref.Repo = segments[1]
	if len(segments) == 3 {
		ref.Path = segments[2]
	}

	return ref, nil
}

// String returns a human-readable representation of the ref.
func (r ActionRef) String() string {
	switch r.Type {
	case "local":
		return r.Path
	case "docker":
		return "docker://" + r.Path
	default:
		s := r.Owner + "/" + r.Repo
		if r.Path != "" {
			s += "/" + r.Path
		}
		s += "@" + r.Version
		return s
	}
}
