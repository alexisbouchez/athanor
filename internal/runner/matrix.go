package runner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexj212/athanor/internal/workflow"
)

// ExpandMatrix computes the Cartesian product of matrix values,
// applies include/exclude rules, and returns the list of combinations.
func ExpandMatrix(def workflow.MatrixDef) []map[string]any {
	if len(def.Values) == 0 {
		// No matrix keys, just includes
		if len(def.Include) > 0 {
			return def.Include
		}
		return nil
	}

	// Compute Cartesian product
	keys := sortedKeys(def.Values)
	combos := cartesian(keys, def.Values)

	// Apply excludes
	combos = applyExcludes(combos, def.Exclude)

	// Apply includes
	combos = applyIncludes(combos, def.Include)

	return combos
}

// MatrixJobID generates a display name for a matrix job.
func MatrixJobID(baseID string, combo map[string]any) string {
	keys := make([]string, 0, len(combo))
	for k := range combo {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	vals := make([]string, 0, len(keys))
	for _, k := range keys {
		vals = append(vals, fmt.Sprintf("%v", combo[k]))
	}

	return fmt.Sprintf("%s (%s)", baseID, strings.Join(vals, ", "))
}

func sortedKeys(m map[string][]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cartesian(keys []string, values map[string][]any) []map[string]any {
	if len(keys) == 0 {
		return []map[string]any{{}}
	}

	result := []map[string]any{{}}
	for _, key := range keys {
		var next []map[string]any
		for _, combo := range result {
			for _, val := range values[key] {
				newCombo := make(map[string]any, len(combo)+1)
				for k, v := range combo {
					newCombo[k] = v
				}
				newCombo[key] = val
				next = append(next, newCombo)
			}
		}
		result = next
	}
	return result
}

func applyExcludes(combos []map[string]any, excludes []map[string]any) []map[string]any {
	if len(excludes) == 0 {
		return combos
	}
	var result []map[string]any
	for _, combo := range combos {
		excluded := false
		for _, exc := range excludes {
			if matchesExclude(combo, exc) {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, combo)
		}
	}
	return result
}

func matchesExclude(combo, exclude map[string]any) bool {
	for k, v := range exclude {
		cv, ok := combo[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", cv) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}

func applyIncludes(combos []map[string]any, includes []map[string]any) []map[string]any {
	for _, inc := range includes {
		// Check if this include matches an existing combo (to add extra keys)
		matched := false
		for i, combo := range combos {
			if includeMatchesCombo(combo, inc) {
				// Merge extra keys
				for k, v := range inc {
					combos[i][k] = v
				}
				matched = true
			}
		}
		if !matched {
			// Add as a new combo
			newCombo := make(map[string]any, len(inc))
			for k, v := range inc {
				newCombo[k] = v
			}
			combos = append(combos, newCombo)
		}
	}
	return combos
}

func includeMatchesCombo(combo, inc map[string]any) bool {
	// An include matches if all keys that exist in both have the same value
	for k, v := range inc {
		cv, ok := combo[k]
		if !ok {
			continue
		}
		if fmt.Sprintf("%v", cv) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}
