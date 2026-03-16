package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alexj212/athanor/internal/runner"
	"github.com/alexj212/athanor/internal/vmm"
	"github.com/alexj212/athanor/internal/workflow"
)

// Job represents a webhook-triggered CI job.
type Job struct {
	RepoFullName string // e.g. "alexisbouchez/athanor"
	CloneURL     string
	SHA          string
	Ref          string // e.g. "refs/heads/main"
	EventName    string // "push" or "pull_request"
	Actor        string
}

// Worker processes jobs sequentially.
type Worker struct {
	queue     chan Job
	cfg       *Config
	gh        *GitHubClient
	logger    *log.Logger
	lifecycle runner.JobLifecycle
}

// NewWorker creates a new job worker.
func NewWorker(cfg *Config, gh *GitHubClient, logger *log.Logger) *Worker {
	w := &Worker{
		queue:  make(chan Job, 32),
		cfg:    cfg,
		gh:     gh,
		logger: logger,
	}

	// Set up VM lifecycle if configured
	if cfg.UseVMs() {
		os.MkdirAll(cfg.VMDiskDir, 0o755)
		network := vmm.NewNetwork("br0")
		w.lifecycle = vmm.NewVMJobLifecycle(vmm.VMJobConfig{
			KernelPath: cfg.KernelPath,
			RootfsPath: cfg.RootfsPath,
			SSHKeyPath: cfg.SSHKeyPath,
			DiskDir:    cfg.VMDiskDir,
			CPUs:       cfg.VMCPUs,
			MemoryMB:   cfg.VMMemoryMB,
		}, network)
		logger.Printf("VM mode enabled (kernel=%s, rootfs=%s)", cfg.KernelPath, cfg.RootfsPath)
	} else {
		logger.Printf("Running in direct mode (no VMs)")
	}

	return w
}

// Start begins processing jobs. Blocks until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.queue:
			w.processJob(ctx, job)
		}
	}
}

// Enqueue adds a job to the queue. Returns false if queue is full.
func (w *Worker) Enqueue(job Job) bool {
	select {
	case w.queue <- job:
		return true
	default:
		return false
	}
}

func (w *Worker) processJob(ctx context.Context, job Job) {
	w.logger.Printf("Processing %s @ %s (%s)", job.RepoFullName, job.SHA[:8], job.Ref)

	// Set pending status
	if err := w.gh.SetCommitStatus(ctx, job.RepoFullName, job.SHA, "pending", "Build started", "athanor"); err != nil {
		w.logger.Printf("Warning: failed to set pending status: %v", err)
	}

	// Prepare workspace
	workspace, err := w.prepareWorkspace(ctx, job)
	if err != nil {
		w.logger.Printf("Error preparing workspace: %v", err)
		w.gh.SetCommitStatus(ctx, job.RepoFullName, job.SHA, "error", err.Error(), "athanor")
		return
	}

	// Discover workflows
	workflowDir := filepath.Join(workspace, ".github", "workflows")
	workflows, err := workflow.DiscoverWorkflows(workflowDir)
	if err != nil {
		w.logger.Printf("Error discovering workflows: %v", err)
		w.gh.SetCommitStatus(ctx, job.RepoFullName, job.SHA, "error", "Failed to parse workflows", "athanor")
		return
	}

	// Filter to matching event
	var matching []*workflow.Workflow
	for _, wf := range workflows {
		for _, event := range wf.On.Events {
			if event == job.EventName {
				matching = append(matching, wf)
				break
			}
		}
	}

	if len(matching) == 0 {
		w.logger.Printf("No workflows match event %q", job.EventName)
		w.gh.SetCommitStatus(ctx, job.RepoFullName, job.SHA, "success", "No matching workflows", "athanor")
		return
	}

	// Run matching workflows
	overallSuccess := true
	for _, wf := range matching {
		w.logger.Printf("Running workflow %q", wf.Name)

		// Build run context
		refName := job.Ref
		if strings.HasPrefix(refName, "refs/heads/") {
			refName = strings.TrimPrefix(refName, "refs/heads/")
		}

		parts := strings.SplitN(job.RepoFullName, "/", 2)
		owner := ""
		if len(parts) == 2 {
			owner = parts[0]
		}

		ghCtx := runner.GitHubContext{
			Workspace:       workspace,
			SHA:             job.SHA,
			Ref:             job.Ref,
			RefName:         refName,
			Repository:      job.RepoFullName,
			RepositoryOwner: owner,
			Actor:           job.Actor,
			EventName:       job.EventName,
			RunID:           job.SHA[:8],
			RunNumber:       "1",
		}

		runCtx := runner.NewRunContextWith(ghCtx)
		var r *runner.Runner
		if w.lifecycle != nil {
			r = runner.NewRunnerWithLifecycle(wf, runCtx, w.lifecycle)
		} else {
			r = runner.NewRunnerWithContext(wf, runCtx)
		}
		go r.Run(ctx)

		// Drain events, capture final status
		status := "success"
		for event := range r.Events() {
			switch e := event.(type) {
			case runner.StepOutput:
				w.logger.Printf("  [%s] %s", e.JobID, e.Line)
			case runner.StepFinished:
				if e.Error != "" {
					w.logger.Printf("  [%s] Step %d error: %s", e.JobID, e.StepIdx, e.Error)
				}
			case runner.JobFinished:
				w.logger.Printf("  Job %s: %s", e.JobID, e.Status)
			case runner.WorkflowFinished:
				status = e.Status
			}
		}

		w.logger.Printf("Workflow %q finished: %s", wf.Name, status)
		if status != "success" {
			overallSuccess = false
		}
	}

	// Set final status
	finalState := "success"
	description := "All workflows passed"
	if !overallSuccess {
		finalState = "failure"
		description = "One or more workflows failed"
	}
	if err := w.gh.SetCommitStatus(ctx, job.RepoFullName, job.SHA, finalState, description, "athanor"); err != nil {
		w.logger.Printf("Warning: failed to set final status: %v", err)
	}
}

func (w *Worker) prepareWorkspace(ctx context.Context, job Job) (string, error) {
	// Workspace per repo: {WorkspaceDir}/{owner}/{repo}
	dir := filepath.Join(w.cfg.WorkspaceDir, job.RepoFullName)

	// Inject token into clone URL for auth
	authCloneURL := injectToken(job.CloneURL, w.cfg.GitHubToken)

	gitDir := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		// Existing clone — fetch and checkout
		cmd := exec.CommandContext(ctx, "git", "remote", "set-url", "origin", authCloneURL)
		cmd.Dir = dir
		cmd.Run() // ignore error

		cmd = exec.CommandContext(ctx, "git", "fetch", "origin")
		cmd.Dir = dir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git fetch: %w", err)
		}

		cmd = exec.CommandContext(ctx, "git", "checkout", "--force", job.SHA)
		cmd.Dir = dir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git checkout %s: %w", job.SHA, err)
		}

		// Clean untracked files
		cmd = exec.CommandContext(ctx, "git", "clean", "-fdx")
		cmd.Dir = dir
		cmd.Run()
	} else {
		// Fresh clone
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", fmt.Errorf("creating workspace dir: %w", err)
		}

		cmd := exec.CommandContext(ctx, "git", "clone", authCloneURL, dir)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone: %w", err)
		}

		cmd = exec.CommandContext(ctx, "git", "checkout", job.SHA)
		cmd.Dir = dir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git checkout %s: %w", job.SHA, err)
		}
	}

	return dir, nil
}

// injectToken rewrites a clone URL to include authentication.
// https://github.com/owner/repo.git → https://x-access-token:TOKEN@github.com/owner/repo.git
func injectToken(cloneURL, token string) string {
	if token == "" {
		return cloneURL
	}
	return strings.Replace(cloneURL, "https://github.com/", fmt.Sprintf("https://x-access-token:%s@github.com/", token), 1)
}
