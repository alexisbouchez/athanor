package workflow

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Workflow represents a GitHub Actions workflow file.
type Workflow struct {
	Name string            `yaml:"name"`
	On   OnTrigger         `yaml:"on"`
	Env  map[string]string `yaml:"env"`
	Jobs map[string]Job    `yaml:"jobs"`

	// Path is the file path this workflow was loaded from (set by ParseFile).
	Path string `yaml:"-"`
}

// OnTrigger represents the "on:" field which can be a string, list, or map.
type OnTrigger struct {
	Events []string
}

func (o *OnTrigger) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		o.Events = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		o.Events = list
		return nil
	case yaml.MappingNode:
		// Map form: keys are event names
		o.Events = make([]string, 0, len(value.Content)/2)
		for i := 0; i < len(value.Content); i += 2 {
			o.Events = append(o.Events, value.Content[i].Value)
		}
		return nil
	default:
		return fmt.Errorf("unexpected on: type %d", value.Kind)
	}
}

// Job represents a single job in a workflow.
type Job struct {
	Name            string            `yaml:"name"`
	RunsOn          string            `yaml:"runs-on"`
	Needs           StringOrSlice     `yaml:"needs"`
	Env             map[string]string `yaml:"env"`
	If              string            `yaml:"if"`
	Defaults        Defaults          `yaml:"defaults"`
	Steps           []Step            `yaml:"steps"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	Outputs         map[string]string `yaml:"outputs"`
	TimeoutMinutes  int               `yaml:"timeout-minutes"`
	Strategy        Strategy          `yaml:"strategy"`
}

// Step represents a single step in a job.
type Step struct {
	ID               string            `yaml:"id"`
	Name             string            `yaml:"name"`
	Run              string            `yaml:"run"`
	Uses             string            `yaml:"uses"`
	With             map[string]string `yaml:"with"`
	Shell            string            `yaml:"shell"`
	Env              map[string]string `yaml:"env"`
	If               string            `yaml:"if"`
	WorkingDirectory string            `yaml:"working-directory"`
	ContinueOnError  bool              `yaml:"continue-on-error"`
	TimeoutMinutes   int               `yaml:"timeout-minutes"`
}

// Strategy defines matrix strategy for a job.
type Strategy struct {
	Matrix      MatrixDef `yaml:"matrix"`
	FailFast    *bool     `yaml:"fail-fast"`
	MaxParallel int       `yaml:"max-parallel"`
}

// MatrixDef defines the matrix values, includes, and excludes.
type MatrixDef struct {
	Values  map[string][]any     // main keys
	Include []map[string]any     `yaml:"include"`
	Exclude []map[string]any     `yaml:"exclude"`
}

func (m *MatrixDef) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("matrix must be a mapping")
	}
	m.Values = make(map[string][]any)
	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i].Value
		val := value.Content[i+1]
		switch key {
		case "include":
			if err := val.Decode(&m.Include); err != nil {
				return fmt.Errorf("decoding matrix include: %w", err)
			}
		case "exclude":
			if err := val.Decode(&m.Exclude); err != nil {
				return fmt.Errorf("decoding matrix exclude: %w", err)
			}
		default:
			var items []any
			if err := val.Decode(&items); err != nil {
				return fmt.Errorf("decoding matrix key %q: %w", key, err)
			}
			m.Values[key] = items
		}
	}
	return nil
}

// Defaults holds default settings for a job.
type Defaults struct {
	Run RunDefaults `yaml:"run"`
}

// RunDefaults holds default run settings.
type RunDefaults struct {
	Shell            string `yaml:"shell"`
	WorkingDirectory string `yaml:"working-directory"`
}

// StringOrSlice is a type that can unmarshal from either a string or a list of strings.
type StringOrSlice []string

func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*s = list
		return nil
	default:
		return fmt.Errorf("needs: must be a string or list, got %d", value.Kind)
	}
}
