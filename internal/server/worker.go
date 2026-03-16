package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexj212/athanor/internal/action"
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
	queue       chan Job
	cfg         *Config
	gh          *GitHubClient
	logger      *log.Logger
	lifecycle   runner.JobLifecycle
	store       *RunStore
	concurrency *ConcurrencyManager
}

// NewWorker creates a new job worker.
func NewWorker(cfg *Config, gh *GitHubClient, logger *log.Logger) *Worker {
	w := &Worker{
		queue:       make(chan Job, 32),
		cfg:         cfg,
		gh:          gh,
		logger:      logger,
		concurrency: NewConcurrencyManager(),
	}

	// Set up VM lifecycle if configured
	if cfg.UseVMs() {
		os.MkdirAll(cfg.VMDiskDir, 0o755)
		network := vmm.NewNetwork("br0")
		w.lifecycle = vmm.NewVMJobLifecycle(vmm.VMJobConfig{
			KernelPath:  cfg.KernelPath,
			RootfsPath:  cfg.RootfsPath,
			SSHKeyPath:  cfg.SSHKeyPath,
			DiskDir:     cfg.VMDiskDir,
			CPUs:        cfg.VMCPUs,
			MemoryMB:    cfg.VMMemoryMB,
			MaxParallel: cfg.VMMaxParallel,
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

	// Record run in store
	run := &Run{
		ID:        job.SHA[:8],
		Repo:      job.RepoFullName,
		SHA:       job.SHA,
		Ref:       job.Ref,
		Actor:     job.Actor,
		Event:     job.EventName,
		Status:    "running",
		StartedAt: time.Now(),
	}
	w.store.Add(run)

	// Set pending status
	if err := w.gh.SetCommitStatus(ctx, job.RepoFullName, job.SHA, "pending", "Build started", "athanor"); err != nil {
		w.logger.Printf("Warning: failed to set pending status: %v", err)
	}

	// Set up artifact directory for this run
	artifactDir := filepath.Join(w.cfg.WorkspaceDir, ".artifacts", job.SHA[:8])
	os.MkdirAll(artifactDir, 0o755)
	action.ArtifactDir = artifactDir
	defer os.RemoveAll(artifactDir)

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

	// Filter to matching event and branch
	var matching []*workflow.Workflow
	for _, wf := range workflows {
		for _, event := range wf.On.Events {
			if event == job.EventName && wf.On.MatchesRef(event, job.Ref) {
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

		// Apply concurrency control
		runCtxForWf := ctx
		var releaseConcurrency func()
		if wf.Concurrency != nil && wf.Concurrency.Group != "" {
			group := wf.Concurrency.Group
			// Interpolate the group name (may contain ${{ github.ref }} etc.)
			runCtxForWf, releaseConcurrency = w.concurrency.Acquire(ctx, group, wf.Concurrency.CancelInProgress)
			w.logger.Printf("Acquired concurrency group %q", group)
		}

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
		runCtx.Secrets = w.cfg.Secrets
		var r *runner.Runner
		if w.lifecycle != nil {
			r = runner.NewRunnerWithLifecycle(wf, runCtx, w.lifecycle)
		} else {
			r = runner.NewRunnerWithContext(wf, runCtx)
		}
		go r.Run(runCtxForWf)

		// Create a GitHub Check Run for this workflow
		now := time.Now()
		checkRunID, checkErr := w.gh.CreateCheckRun(ctx, job.RepoFullName, CheckRun{
			Name:      wf.Name,
			HeadSHA:   job.SHA,
			Status:    "in_progress",
			StartedAt: &now,
		})
		if checkErr != nil {
			w.logger.Printf("Warning: failed to create check run: %v", checkErr)
		}

		// Track this workflow in the run
		wfIdx := len(run.Workflows)
		run.Workflows = append(run.Workflows, WorkflowRun{Name: wf.Name, Status: "running"})
		w.store.Update(run.ID, func(r *Run) {})

		// Drain events, capture final status and logs
		status := "success"
		jobIndices := make(map[string]int) // jobID -> index in workflow's Jobs
		for event := range r.Events() {
			switch e := event.(type) {
			case runner.JobStarted:
				idx := len(run.Workflows[wfIdx].Jobs)
				jobIndices[e.JobID] = idx
				run.Workflows[wfIdx].Jobs = append(run.Workflows[wfIdx].Jobs, JobRun{
					ID: e.JobID, Status: "running",
				})
				w.store.Update(run.ID, func(r *Run) {})
			case runner.StepStarted:
				if ji, ok := jobIndices[e.JobID]; ok {
					run.Workflows[wfIdx].Jobs[ji].Steps = append(
						run.Workflows[wfIdx].Jobs[ji].Steps,
						StepRun{Name: e.StepName, Status: "running"},
					)
					w.store.Update(run.ID, func(r *Run) {})
				}
			case runner.StepOutput:
				w.logger.Printf("  [%s] %s", e.JobID, e.Line)
				// Collect log lines per step
				if ji, ok := jobIndices[e.JobID]; ok {
					if e.StepIdx < len(run.Workflows[wfIdx].Jobs[ji].Steps) {
						step := &run.Workflows[wfIdx].Jobs[ji].Steps[e.StepIdx]
						step.Lines = append(step.Lines, e.Line)
						w.store.Update(run.ID, func(r *Run) {})
					}
				}
			case runner.StepFinished:
				if e.Error != "" {
					w.logger.Printf("  [%s] Step %d error: %s", e.JobID, e.StepIdx, e.Error)
				}
				if ji, ok := jobIndices[e.JobID]; ok {
					if e.StepIdx < len(run.Workflows[wfIdx].Jobs[ji].Steps) {
						s := "success"
						if e.Skipped {
							s = "skipped"
						} else if e.ExitCode != 0 || e.Error != "" {
							s = "failure"
						}
						step := &run.Workflows[wfIdx].Jobs[ji].Steps[e.StepIdx]
						step.Status = s
						if e.Error != "" {
							step.Lines = append(step.Lines, "Error: "+e.Error)
						}
						w.store.Update(run.ID, func(r *Run) {})
					}
				}
			case runner.JobFinished:
				w.logger.Printf("  Job %s: %s", e.JobID, e.Status)
				if ji, ok := jobIndices[e.JobID]; ok {
					run.Workflows[wfIdx].Jobs[ji].Status = e.Status
					w.store.Update(run.ID, func(r *Run) {})
				}
			case runner.WorkflowFinished:
				status = e.Status
			}
		}

		run.Workflows[wfIdx].Status = status
		w.store.Update(run.ID, func(r *Run) {})

		// Update the GitHub Check Run with conclusion + log output
		if checkRunID != 0 {
			conclusion := "success"
			if status != "success" {
				conclusion = "failure"
			}
			completedAt := time.Now()
			logText := buildCheckRunLog(run.Workflows[wfIdx])
			w.gh.UpdateCheckRun(ctx, job.RepoFullName, checkRunID, CheckRun{
				Status:      "completed",
				Conclusion:  conclusion,
				CompletedAt: &completedAt,
				Output: &CheckOutput{
					Title:   fmt.Sprintf("%s: %s", wf.Name, conclusion),
					Summary: fmt.Sprintf("Workflow **%s** finished with status **%s**", wf.Name, conclusion),
					Text:    logText,
				},
			})
		}

		// Release concurrency slot
		if releaseConcurrency != nil {
			releaseConcurrency()
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

	// Update run in store
	w.store.Update(run.ID, func(r *Run) {
		r.Status = finalState
		r.Duration = time.Since(r.StartedAt).Seconds()
	})
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

// buildCheckRunLog formats a workflow run as Markdown for the GitHub Checks API output.
func buildCheckRunLog(wf WorkflowRun) string {
	var b strings.Builder
	for _, job := range wf.Jobs {
		fmt.Fprintf(&b, "## Job: %s (%s)\n\n", job.ID, job.Status)
		for _, step := range job.Steps {
			icon := "white_check_mark"
			switch step.Status {
			case "failure":
				icon = "x"
			case "skipped":
				icon = "fast_forward"
			case "running":
				icon = "hourglass"
			}
			fmt.Fprintf(&b, "### :%s: %s\n\n", icon, step.Name)
			if len(step.Lines) > 0 {
				b.WriteString("```\n")
				for _, line := range step.Lines {
					b.WriteString(line)
					b.WriteByte('\n')
				}
				b.WriteString("```\n\n")
			}
		}
	}
	text := b.String()
	// GitHub Checks API text field is limited to 65535 chars
	if len(text) > 65000 {
		text = text[:65000] + "\n\n... (truncated)"
	}
	return text
}

// injectToken rewrites a clone URL to include authentication.
// https://github.com/owner/repo.git → https://x-access-token:TOKEN@github.com/owner/repo.git
func injectToken(cloneURL, token string) string {
	if token == "" {
		return cloneURL
	}
	return strings.Replace(cloneURL, "https://github.com/", fmt.Sprintf("https://x-access-token:%s@github.com/", token), 1)
}
