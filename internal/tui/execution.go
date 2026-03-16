package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexj212/athanor/internal/runner"
)

// RunEventMsg wraps a runner.RunEvent as a tea.Msg.
type RunEventMsg struct {
	Event runner.RunEvent
}

type jobState struct {
	id     string
	status string // "pending", "running", "success", "failure", "skipped"
	steps  []stepState
}

type stepState struct {
	name   string
	status string // "pending", "running", "success", "failure", "skipped"
	logs   []string
}

// executionModel handles the execution phase display.
type executionModel struct {
	jobs       []jobState
	jobIndex   map[string]int
	viewport   viewport.Model
	spinner    spinner.Model
	width      int
	height     int
	activeJob  string
	activeStep int
	done       bool
	status     string
}

func newExecutionModel() executionModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleRunning

	vp := viewport.New(0, 0)

	return executionModel{
		jobIndex: make(map[string]int),
		spinner:  s,
		viewport: vp,
	}
}

func (m executionModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m executionModel) Update(msg tea.Msg) (executionModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = m.logPanelWidth()
		m.viewport.Height = m.height - 4
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case RunEventMsg:
		return m.handleRunEvent(msg.Event)

	case tea.KeyMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, tea.Batch(cmds...)
}

func (m executionModel) handleRunEvent(event runner.RunEvent) (executionModel, tea.Cmd) {
	switch e := event.(type) {
	case runner.WorkflowStarted:
		// nothing to do

	case runner.JobStarted:
		m.jobIndex[e.JobID] = len(m.jobs)
		m.jobs = append(m.jobs, jobState{id: e.JobID, status: "running"})
		m.activeJob = e.JobID

	case runner.StepStarted:
		if idx, ok := m.jobIndex[e.JobID]; ok {
			m.jobs[idx].steps = append(m.jobs[idx].steps, stepState{
				name:   e.StepName,
				status: "running",
			})
			m.activeJob = e.JobID
			m.activeStep = len(m.jobs[idx].steps) - 1
			m.updateViewport()
		}

	case runner.StepOutput:
		if idx, ok := m.jobIndex[e.JobID]; ok {
			if e.StepIdx < len(m.jobs[idx].steps) {
				m.jobs[idx].steps[e.StepIdx].logs = append(
					m.jobs[idx].steps[e.StepIdx].logs, e.Line,
				)
				// Auto-scroll if viewing active step
				if e.JobID == m.activeJob && e.StepIdx == m.activeStep {
					m.updateViewport()
					m.viewport.GotoBottom()
				}
			}
		}

	case runner.StepFinished:
		if idx, ok := m.jobIndex[e.JobID]; ok {
			if e.StepIdx < len(m.jobs[idx].steps) {
				if e.Skipped {
					m.jobs[idx].steps[e.StepIdx].status = "skipped"
				} else if e.ExitCode == 0 {
					m.jobs[idx].steps[e.StepIdx].status = "success"
				} else {
					m.jobs[idx].steps[e.StepIdx].status = "failure"
				}
			}
		}

	case runner.JobFinished:
		if idx, ok := m.jobIndex[e.JobID]; ok {
			m.jobs[idx].status = e.Status
		}

	case runner.WorkflowFinished:
		m.done = true
		m.status = e.Status
	}

	return m, m.spinner.Tick
}

func (m *executionModel) updateViewport() {
	if idx, ok := m.jobIndex[m.activeJob]; ok {
		if m.activeStep < len(m.jobs[idx].steps) {
			step := m.jobs[idx].steps[m.activeStep]
			content := strings.Join(step.logs, "\n")
			m.viewport.SetContent(content)
		}
	}
}

func (m executionModel) treePanelWidth() int {
	return max(m.width*35/100, 30)
}

func (m executionModel) logPanelWidth() int {
	return m.width - m.treePanelWidth() - 5
}

func (m executionModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	tree := m.renderTree()
	logs := m.renderLogs()

	treePanel := stylePanel.Width(m.treePanelWidth()).Height(m.height - 3).Render(tree)
	logPanel := stylePanel.Width(m.logPanelWidth()).Height(m.height - 3).Render(logs)

	main := lipgloss.JoinHorizontal(lipgloss.Top, treePanel, logPanel)

	statusBar := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, main, statusBar)
}

func (m executionModel) renderTree() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Jobs") + "\n\n")

	for _, job := range m.jobs {
		glyph := m.glyphFor(job.status)
		jobLine := fmt.Sprintf("%s %s", glyph, job.id)
		b.WriteString(jobLine + "\n")

		for _, step := range job.steps {
			glyph := m.glyphFor(step.status)
			stepLine := fmt.Sprintf("  %s %s", glyph, step.name)
			b.WriteString(stepLine + "\n")
		}
	}

	return b.String()
}

func (m executionModel) renderLogs() string {
	var b strings.Builder
	title := "Output"
	if m.activeJob != "" {
		if idx, ok := m.jobIndex[m.activeJob]; ok {
			if m.activeStep < len(m.jobs[idx].steps) {
				title = m.jobs[idx].steps[m.activeStep].name
			}
		}
	}
	b.WriteString(styleTitle.Render(title) + "\n\n")
	b.WriteString(m.viewport.View())
	return b.String()
}

func (m executionModel) renderStatusBar() string {
	if m.done {
		if m.status == "success" {
			return stylePassed.Render("  Workflow completed successfully")
		}
		return styleFailed.Render("  Workflow failed")
	}
	return styleRunning.Render("  " + m.spinner.View() + " Running...")
}

func (m executionModel) glyphFor(status string) string {
	switch status {
	case "running":
		return m.spinner.View()
	case "success":
		return glyphPassed
	case "failure":
		return glyphFailed
	case "skipped":
		return glyphSkipped
	default:
		return glyphPending
	}
}
