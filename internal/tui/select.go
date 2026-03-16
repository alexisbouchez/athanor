package tui

import (
	"path/filepath"

	"github.com/charmbracelet/huh"

	"github.com/alexj212/athanor/internal/workflow"
)

// buildSelectForm creates a huh form for selecting a workflow.
func buildSelectForm(workflows []*workflow.Workflow, selected *string) *huh.Form {
	options := make([]huh.Option[string], 0, len(workflows))
	for _, w := range workflows {
		label := filepath.Base(w.Path)
		if w.Name != "" {
			label = w.Name + " (" + filepath.Base(w.Path) + ")"
		}
		options = append(options, huh.NewOption(label, w.Path))
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a workflow to run").
				Options(options...).
				Value(selected),
		),
	)
}
