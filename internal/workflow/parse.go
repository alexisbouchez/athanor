package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ParseFile reads and parses a workflow YAML file.
func ParseFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow: %w", err)
	}
	w, err := ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	w.Path = path
	return w, nil
}

// ParseBytes parses workflow YAML from bytes.
func ParseBytes(data []byte) (*Workflow, error) {
	var w Workflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("unmarshaling workflow: %w", err)
	}
	if err := validate(&w); err != nil {
		return nil, err
	}
	return &w, nil
}

// DiscoverWorkflows finds all .yml and .yaml files in dir and parses them.
func DiscoverWorkflows(dir string) ([]*Workflow, error) {
	var workflows []*Workflow
	for _, ext := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(dir, ext))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			w, err := ParseFile(m)
			if err != nil {
				return nil, err
			}
			workflows = append(workflows, w)
		}
	}
	return workflows, nil
}

// validate checks that a parsed workflow is internally consistent.
func validate(w *Workflow) error {
	if len(w.Jobs) == 0 {
		return fmt.Errorf("workflow has no jobs")
	}
	for jobID, job := range w.Jobs {
		// Jobs using reusable workflows have uses: instead of steps:
		if job.Uses != "" {
			if len(job.Steps) > 0 {
				return fmt.Errorf("job %q cannot have both uses: and steps:", jobID)
			}
			continue
		}
		if len(job.Steps) == 0 {
			return fmt.Errorf("job %q has no steps", jobID)
		}
		for i, step := range job.Steps {
			hasRun := step.Run != ""
			hasUses := step.Uses != ""
			if !hasRun && !hasUses {
				return fmt.Errorf("job %q step %d must have run: or uses:", jobID, i)
			}
			if hasRun && hasUses {
				return fmt.Errorf("job %q step %d cannot have both run: and uses:", jobID, i)
			}
			if hasUses && step.Shell != "" {
				return fmt.Errorf("job %q step %d: uses: steps cannot set shell:", jobID, i)
			}
		}
		for _, need := range job.Needs {
			if _, ok := w.Jobs[need]; !ok {
				return fmt.Errorf("job %q needs %q which does not exist", jobID, need)
			}
		}
	}
	return nil
}
