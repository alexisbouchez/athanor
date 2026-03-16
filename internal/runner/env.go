package runner

import (
	"fmt"
	"os"
	"strings"
)

// Env manages layered environment variables and step outputs.
type Env struct {
	base    map[string]string
	outputs map[string]map[string]string // stepID -> key -> value
}

// NewEnv creates an Env with the given base variables.
func NewEnv(layers ...map[string]string) *Env {
	merged := make(map[string]string)
	for _, layer := range layers {
		for k, v := range layer {
			merged[k] = v
		}
	}
	return &Env{
		base:    merged,
		outputs: make(map[string]map[string]string),
	}
}

// List returns the environment as a slice of KEY=VALUE strings,
// suitable for exec.Cmd.Env.
func (e *Env) List() []string {
	result := os.Environ()
	for k, v := range e.base {
		result = append(result, k+"="+v)
	}
	return result
}

// Set sets a variable in the base layer.
func (e *Env) Set(key, value string) {
	e.base[key] = value
}

// Get retrieves a variable from the base layer.
func (e *Env) Get(key string) string {
	return e.base[key]
}

// SetOutput records an output for a step.
func (e *Env) SetOutput(stepID, key, value string) {
	if e.outputs[stepID] == nil {
		e.outputs[stepID] = make(map[string]string)
	}
	e.outputs[stepID][key] = value
}

// GetOutput retrieves a step output value.
func (e *Env) GetOutput(stepID, key string) string {
	if m, ok := e.outputs[stepID]; ok {
		return m[key]
	}
	return ""
}

// GetOutputs returns all outputs for a step.
func (e *Env) GetOutputs(stepID string) map[string]string {
	if m, ok := e.outputs[stepID]; ok {
		return m
	}
	return nil
}

// ParseOutputFile reads a GITHUB_OUTPUT file and records the outputs for the given step.
// Supports both KEY=VALUE format and multiline delimiter format:
//
//	KEY<<DELIMITER
//	value line 1
//	value line 2
//	DELIMITER
func (e *Env) ParseOutputFile(stepID, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading output file: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}
		// Check for heredoc format: KEY<<DELIMITER
		if idx := strings.Index(line, "<<"); idx != -1 {
			key := line[:idx]
			delimiter := line[idx+2:]
			i++
			var valueLines []string
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == delimiter {
					break
				}
				valueLines = append(valueLines, lines[i])
				i++
			}
			e.SetOutput(stepID, key, strings.Join(valueLines, "\n"))
			i++ // skip delimiter line
			continue
		}
		// Simple KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			e.SetOutput(stepID, parts[0], parts[1])
		}
		i++
	}
	return nil
}

// ParseEnvFile reads a GITHUB_ENV file and returns the key-value pairs.
// Supports KEY=VALUE and heredoc KEY<<DELIMITER format.
func ParseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading env file: %w", err)
	}
	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}
		// Check for heredoc format
		if idx := strings.Index(line, "<<"); idx != -1 {
			key := line[:idx]
			delimiter := line[idx+2:]
			i++
			var valueLines []string
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == delimiter {
					break
				}
				valueLines = append(valueLines, lines[i])
				i++
			}
			result[key] = strings.Join(valueLines, "\n")
			i++ // skip delimiter line
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
		i++
	}
	return result, nil
}

// ParsePathFile reads a GITHUB_PATH file and returns the path entries.
func ParsePathFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading path file: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}
