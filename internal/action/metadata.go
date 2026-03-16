package action

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ActionMetadata represents a parsed action.yml/action.yaml file.
type ActionMetadata struct {
	Name        string                `yaml:"name"`
	Description string                `yaml:"description"`
	Inputs      map[string]InputDef   `yaml:"inputs"`
	Outputs     map[string]OutputDef  `yaml:"outputs"`
	Runs        RunsDef               `yaml:"runs"`
}

// InputDef defines an action input.
type InputDef struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

// OutputDef defines an action output.
type OutputDef struct {
	Description string `yaml:"description"`
	Value       string `yaml:"value"` // expression for composite actions
}

// RunsDef defines how the action runs.
type RunsDef struct {
	Using string         `yaml:"using"` // "composite", "node12", "node16", "node20", "docker"
	Steps []CompositeStep `yaml:"steps"` // for composite
	Main  string         `yaml:"main"`  // for node
	Image string         `yaml:"image"` // for docker
}

// CompositeStep is a step in a composite action.
type CompositeStep struct {
	Name             string            `yaml:"name"`
	ID               string            `yaml:"id"`
	Run              string            `yaml:"run"`
	Uses             string            `yaml:"uses"`
	With             map[string]string `yaml:"with"`
	Shell            string            `yaml:"shell"`
	Env              map[string]string `yaml:"env"`
	If               string            `yaml:"if"`
	WorkingDirectory string            `yaml:"working-directory"`
}

// LoadMetadata reads and parses an action.yml or action.yaml from the given directory.
func LoadMetadata(actionDir string) (*ActionMetadata, error) {
	for _, name := range []string{"action.yml", "action.yaml"} {
		path := filepath.Join(actionDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta ActionMetadata
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		return &meta, nil
	}
	return nil, fmt.Errorf("no action.yml or action.yaml found in %s", actionDir)
}
