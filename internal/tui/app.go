package tui

import (
	"github.com/charmbracelet/huh"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexj212/athanor/internal/runner"
	"github.com/alexj212/athanor/internal/workflow"
)

type phase int

const (
	phaseSelect phase = iota
	phaseExecute
	phaseDone
)

// SelectedMsg is sent when a workflow is selected.
type SelectedMsg struct {
	Path string
}

// Model is the top-level bubbletea model.
type Model struct {
	phase     phase
	workflows []*workflow.Workflow
	selected  string
	form      *huh.Form
	execution executionModel
	width     int
	height    int
}

// NewModel creates the TUI model with discovered workflows.
func NewModel(workflows []*workflow.Workflow) Model {
	m := Model{
		phase:     phaseSelect,
		workflows: workflows,
		execution: newExecutionModel(),
	}
	m.form = buildSelectForm(m.workflows, &m.selected)
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.form.Init(), m.execution.Init())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.phase == phaseDone && (msg.String() == "q" || msg.String() == "enter") {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.phase == phaseExecute || m.phase == phaseDone {
			var cmd tea.Cmd
			m.execution, cmd = m.execution.Update(msg)
			return m, cmd
		}
		return m, nil

	case RunEventMsg:
		var cmd tea.Cmd
		m.execution, cmd = m.execution.Update(msg)
		if _, ok := msg.Event.(runner.WorkflowFinished); ok {
			m.phase = phaseDone
		}
		return m, cmd
	}

	switch m.phase {
	case phaseSelect:
		return m.updateSelect(msg)
	case phaseExecute, phaseDone:
		return m.updateExecution(msg)
	}

	return m, nil
}

func (m Model) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted && m.selected != "" {
		m.phase = phaseExecute
		return m, func() tea.Msg {
			return SelectedMsg{Path: m.selected}
		}
	}

	return m, cmd
}

func (m Model) updateExecution(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.execution, cmd = m.execution.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	switch m.phase {
	case phaseSelect:
		return m.form.View()
	case phaseExecute, phaseDone:
		return m.execution.View()
	}
	return ""
}
