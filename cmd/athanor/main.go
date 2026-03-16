package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexj212/athanor/internal/runner"
	"github.com/alexj212/athanor/internal/server"
	"github.com/alexj212/athanor/internal/tui"
	"github.com/alexj212/athanor/internal/workflow"
)

// appModel wraps the TUI model and handles workflow selection → runner startup.
type appModel struct {
	tui       tui.Model
	ctx       context.Context
	workflows []*workflow.Workflow
	started   bool
	program   *tea.Program
}

// startRunnerMsg is sent after the runner goroutine is launched.
type startRunnerMsg struct{}

func (m appModel) Init() tea.Cmd {
	return m.tui.Init()
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SelectedMsg:
		if !m.started {
			m.started = true
			wf := m.findWorkflow(msg.Path)
			if wf == nil {
				return m, tea.Quit
			}
			return m, func() tea.Msg {
				r := runner.NewRunner(wf)
				go r.Run(m.ctx)
				go func() {
					for event := range r.Events() {
						if m.program != nil {
							m.program.Send(tui.RunEventMsg{Event: event})
						}
					}
				}()
				return startRunnerMsg{}
			}
		}
	case startRunnerMsg:
		// Runner started, nothing else to do
		return m, nil
	}

	var cmd tea.Cmd
	tuiModel, cmd := m.tui.Update(msg)
	if t, ok := tuiModel.(tui.Model); ok {
		m.tui = t
	}
	return m, cmd
}

func (m appModel) View() string {
	return m.tui.View()
}

func (m appModel) findWorkflow(path string) *workflow.Workflow {
	for _, w := range m.workflows {
		if w.Path == path {
			return w
		}
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Check for subcommands
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runServe()
		return
	}

	workflowDir := flag.String("workflow-dir", ".github/workflows", "directory containing workflow files")
	workflowFile := flag.String("workflow", "", "run a specific workflow file (skip selection)")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *workflowFile != "" {
		runDirect(ctx, *workflowFile)
		return
	}

	workflows, err := workflow.DiscoverWorkflows(*workflowDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering workflows: %v\n", err)
		os.Exit(1)
	}
	if len(workflows) == 0 {
		fmt.Fprintf(os.Stderr, "No workflows found in %s\n", *workflowDir)
		os.Exit(1)
	}

	tuiModel := tui.NewModel(workflows)
	app := &appModel{
		tui:       tuiModel,
		ctx:       ctx,
		workflows: workflows,
	}

	p := tea.NewProgram(app, tea.WithAltScreen())
	app.program = p

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runServe starts the webhook server.
func runServe() {
	cfg, err := server.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := server.New(cfg)
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// runDirect runs a specific workflow file directly (skips TUI selection).
func runDirect(ctx context.Context, path string) {
	wf, err := workflow.ParseFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing workflow: %v\n", err)
		os.Exit(1)
	}

	workflows := []*workflow.Workflow{wf}
	tuiModel := tui.NewModel(workflows)
	app := &appModel{
		tui:       tuiModel,
		ctx:       ctx,
		workflows: workflows,
		started:   true,
	}

	p := tea.NewProgram(app, tea.WithAltScreen())
	app.program = p

	// Start runner immediately
	r := runner.NewRunner(wf)
	go r.Run(ctx)
	go func() {
		for event := range r.Events() {
			p.Send(tui.RunEventMsg{Event: event})
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
