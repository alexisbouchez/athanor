package workflow

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Workflow represents a GitHub Actions workflow file.
type Workflow struct {
	Name        string            `yaml:"name"`
	On          OnTrigger         `yaml:"on"`
	Env         map[string]string `yaml:"env"`
	Jobs        map[string]Job    `yaml:"jobs"`
	Concurrency *Concurrency      `yaml:"concurrency"`
	Permissions Permissions       `yaml:"permissions"`

	// Path is the file path this workflow was loaded from (set by ParseFile).
	Path string `yaml:"-"`
}

// OnTrigger represents the "on:" field which can be a string, list, or map.
type OnTrigger struct {
	Events  []string
	Filters map[string]EventFilter // event name -> filter (branches, tags, paths)
}

// EventFilter holds branch/tag/path filters for an event trigger.
type EventFilter struct {
	Branches        []string `yaml:"branches"`
	BranchesIgnore  []string `yaml:"branches-ignore"`
	Tags            []string `yaml:"tags"`
	TagsIgnore      []string `yaml:"tags-ignore"`
	Paths           []string `yaml:"paths"`
	PathsIgnore     []string `yaml:"paths-ignore"`
	Types           []string `yaml:"types"`
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
		o.Events = make([]string, 0, len(value.Content)/2)
		o.Filters = make(map[string]EventFilter)
		for i := 0; i < len(value.Content); i += 2 {
			eventName := value.Content[i].Value
			o.Events = append(o.Events, eventName)
			if value.Content[i+1].Kind == yaml.MappingNode {
				var f EventFilter
				if err := value.Content[i+1].Decode(&f); err == nil {
					o.Filters[eventName] = f
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("unexpected on: type %d", value.Kind)
	}
}

// MatchesRef checks if a trigger event matches the given ref (e.g. "refs/heads/main").
func (o *OnTrigger) MatchesRef(event, ref string) bool {
	f, ok := o.Filters[event]
	if !ok {
		return true // no filter = matches all
	}

	// Extract branch name from ref
	branch := strings.TrimPrefix(ref, "refs/heads/")
	tag := strings.TrimPrefix(ref, "refs/tags/")

	if len(f.Branches) > 0 {
		if !matchesAny(branch, f.Branches) {
			return false
		}
	}
	if len(f.BranchesIgnore) > 0 {
		if matchesAny(branch, f.BranchesIgnore) {
			return false
		}
	}
	if len(f.Tags) > 0 {
		if !matchesAny(tag, f.Tags) {
			return false
		}
	}
	if len(f.TagsIgnore) > 0 {
		if matchesAny(tag, f.TagsIgnore) {
			return false
		}
	}
	return true
}

func matchesAny(value string, patterns []string) bool {
	for _, p := range patterns {
		if matchGlob(value, p) {
			return true
		}
	}
	return false
}

func matchGlob(value, pattern string) bool {
	// Simple glob: only supports * as wildcard and ** for recursive
	if pattern == value {
		return true
	}
	if pattern == "*" || pattern == "**" {
		return true
	}
	// Prefix match with *
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		return strings.HasPrefix(value, pattern[:len(pattern)-1])
	}
	// Suffix match with *
	if len(pattern) > 0 && pattern[0] == '*' {
		return strings.HasSuffix(value, pattern[1:])
	}
	return false
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
	Uses            string            `yaml:"uses"` // reusable workflow reference
	With            map[string]string `yaml:"with"` // inputs for reusable workflow
	Secrets         StringOrMap       `yaml:"secrets"` // "inherit" or map of secret mappings
	Container       *Container        `yaml:"container"`
	Services        map[string]Service `yaml:"services"`
	Concurrency     *Concurrency      `yaml:"concurrency"`
	Permissions     Permissions        `yaml:"permissions"`
}

// Container defines a Docker container to run job steps in.
type Container struct {
	Image   string            `yaml:"image"`
	Env     map[string]string `yaml:"env"`
	Ports   []string          `yaml:"ports"`
	Volumes []string          `yaml:"volumes"`
	Options string            `yaml:"options"`
}

// UnmarshalYAML allows container: to be a string (image name) or a mapping.
func (c *Container) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		c.Image = value.Value
		return nil
	}
	type containerAlias Container
	var alias containerAlias
	if err := value.Decode(&alias); err != nil {
		return err
	}
	*c = Container(alias)
	return nil
}

// Service defines a service container that runs alongside the job.
type Service struct {
	Image   string            `yaml:"image"`
	Env     map[string]string `yaml:"env"`
	Ports   []string          `yaml:"ports"`
	Volumes []string          `yaml:"volumes"`
	Options string            `yaml:"options"`
}

// Concurrency defines concurrency control for a workflow or job.
type Concurrency struct {
	Group            string `yaml:"group"`
	CancelInProgress bool   `yaml:"cancel-in-progress"`
}

// UnmarshalYAML allows concurrency: to be a string (group name) or a mapping.
func (c *Concurrency) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		c.Group = value.Value
		return nil
	}
	type concurrencyAlias Concurrency
	var alias concurrencyAlias
	if err := value.Decode(&alias); err != nil {
		return err
	}
	*c = Concurrency(alias)
	return nil
}

// Permissions defines the permissions granted to the GITHUB_TOKEN.
type Permissions map[string]string

// UnmarshalYAML allows permissions: to be a string ("read-all", "write-all") or a mapping.
func (p *Permissions) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*p = Permissions{"_all": value.Value}
		return nil
	}
	var m map[string]string
	if err := value.Decode(&m); err != nil {
		return err
	}
	*p = Permissions(m)
	return nil
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

// StringOrMap can unmarshal from a string (e.g. "inherit") or a map of strings.
type StringOrMap struct {
	Value string            // non-empty if scalar (e.g. "inherit")
	Map   map[string]string // non-nil if mapping
}

func (s *StringOrMap) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		s.Value = value.Value
		return nil
	}
	if value.Kind == yaml.MappingNode {
		s.Map = make(map[string]string)
		return value.Decode(&s.Map)
	}
	return fmt.Errorf("expected string or map, got %d", value.Kind)
}

// IsInherit returns true if the value is "inherit".
func (s *StringOrMap) IsInherit() bool {
	return s.Value == "inherit"
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
