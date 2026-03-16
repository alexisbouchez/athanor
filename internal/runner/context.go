package runner

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// RunContext holds all context objects available during workflow execution.
type RunContext struct {
	GitHub  GitHubContext
	Runner  RunnerContext
	Env     map[string]string
	Steps   map[string]StepContext
	Needs   map[string]NeedContext
	Matrix  map[string]any
	Inputs  map[string]string
	Secrets map[string]string
	Job     JobContext
}

// GitHubContext holds the github.* context values.
type GitHubContext struct {
	Workspace       string
	SHA             string
	Ref             string
	RefName         string
	Repository      string
	RepositoryOwner string
	Actor           string
	EventName       string
	RunID           string
	RunNumber       string
}

// RunnerContext holds the runner.* context values.
type RunnerContext struct {
	OS   string
	Arch string
	Temp string
}

// StepContext holds the result of a completed step.
type StepContext struct {
	Outputs    map[string]string
	Outcome    string // "success", "failure", "skipped"
	Conclusion string // same as outcome unless continue-on-error
}

// NeedContext holds the result of a completed job dependency.
type NeedContext struct {
	Outputs map[string]string
	Result  string // "success", "failure", "skipped"
}

// JobContext holds the job.* context values.
type JobContext struct {
	Status string
}

// JobResult holds the result of a completed job.
type JobResult struct {
	Status  string
	Outputs map[string]string
}

// NewRunContextWith creates a RunContext with explicit GitHub context values.
func NewRunContextWith(ghCtx GitHubContext) *RunContext {
	rc := &RunContext{
		Env:     make(map[string]string),
		Steps:   make(map[string]StepContext),
		Needs:   make(map[string]NeedContext),
		Inputs:  make(map[string]string),
		Secrets: make(map[string]string),
	}
	rc.GitHub = ghCtx
	rc.Runner = detectRunnerContext()
	return rc
}

// NewRunContext creates a RunContext with values detected from the environment.
func NewRunContext() *RunContext {
	rc := &RunContext{
		Env:     make(map[string]string),
		Steps:   make(map[string]StepContext),
		Needs:   make(map[string]NeedContext),
		Inputs:  make(map[string]string),
		Secrets: make(map[string]string),
	}
	rc.GitHub = detectGitHubContext()
	rc.Runner = detectRunnerContext()
	return rc
}

// ToMap converts the RunContext to a map[string]any for use with expr.Evaluate.
func (rc *RunContext) ToMap() map[string]any {
	steps := make(map[string]any, len(rc.Steps))
	for id, sc := range rc.Steps {
		outputs := make(map[string]any, len(sc.Outputs))
		for k, v := range sc.Outputs {
			outputs[k] = v
		}
		steps[id] = map[string]any{
			"outputs":    outputs,
			"outcome":    sc.Outcome,
			"conclusion": sc.Conclusion,
		}
	}

	needs := make(map[string]any, len(rc.Needs))
	for id, nc := range rc.Needs {
		outputs := make(map[string]any, len(nc.Outputs))
		for k, v := range nc.Outputs {
			outputs[k] = v
		}
		needs[id] = map[string]any{
			"outputs": outputs,
			"result":  nc.Result,
		}
	}

	env := make(map[string]any, len(rc.Env))
	for k, v := range rc.Env {
		env[k] = v
	}

	matrix := make(map[string]any, len(rc.Matrix))
	for k, v := range rc.Matrix {
		matrix[k] = v
	}

	inputs := make(map[string]any, len(rc.Inputs))
	for k, v := range rc.Inputs {
		inputs[k] = v
	}

	secrets := make(map[string]any, len(rc.Secrets))
	for k, v := range rc.Secrets {
		secrets[k] = v
	}

	return map[string]any{
		"github": map[string]any{
			"workspace":        rc.GitHub.Workspace,
			"sha":              rc.GitHub.SHA,
			"ref":              rc.GitHub.Ref,
			"ref_name":         rc.GitHub.RefName,
			"repository":       rc.GitHub.Repository,
			"repository_owner": rc.GitHub.RepositoryOwner,
			"actor":            rc.GitHub.Actor,
			"event_name":       rc.GitHub.EventName,
			"run_id":           rc.GitHub.RunID,
			"run_number":       rc.GitHub.RunNumber,
		},
		"runner": map[string]any{
			"os":   rc.Runner.OS,
			"arch": rc.Runner.Arch,
			"temp": rc.Runner.Temp,
		},
		"steps":   steps,
		"needs":   needs,
		"env":     env,
		"matrix":  matrix,
		"inputs":  inputs,
		"secrets": secrets,
		"job": map[string]any{
			"status": rc.Job.Status,
		},
	}
}

// DefaultEnvVars returns the environment variables that GitHub Actions sets by default.
func (rc *RunContext) DefaultEnvVars(jobID, actionName string) map[string]string {
	return map[string]string{
		"CI":                 "true",
		"GITHUB_WORKSPACE":  rc.GitHub.Workspace,
		"GITHUB_SHA":        rc.GitHub.SHA,
		"GITHUB_REF":        rc.GitHub.Ref,
		"GITHUB_REF_NAME":   rc.GitHub.RefName,
		"GITHUB_REPOSITORY": rc.GitHub.Repository,
		"GITHUB_ACTOR":      rc.GitHub.Actor,
		"RUNNER_OS":         rc.Runner.OS,
		"RUNNER_ARCH":       rc.Runner.Arch,
		"RUNNER_TEMP":       rc.Runner.Temp,
		"GITHUB_JOB":        jobID,
		"GITHUB_ACTION":     actionName,
		"GITHUB_RUN_ID":     rc.GitHub.RunID,
		"GITHUB_RUN_NUMBER": rc.GitHub.RunNumber,
		"GITHUB_EVENT_NAME": rc.GitHub.EventName,
	}
}

func detectGitHubContext() GitHubContext {
	gc := GitHubContext{
		EventName: "push",
		RunID:     "1",
		RunNumber: "1",
	}

	if user := os.Getenv("USER"); user != "" {
		gc.Actor = user
	} else {
		gc.Actor = "local"
	}

	// Try to detect from git
	gc.SHA = gitOutput("rev-parse", "HEAD")
	gc.Ref = gitOutput("rev-parse", "--symbolic-full-name", "HEAD")
	gc.RefName = gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	gc.Workspace = gitOutput("rev-parse", "--show-toplevel")

	// Parse remote origin URL for repository/owner
	remote := gitOutput("remote", "get-url", "origin")
	gc.Repository, gc.RepositoryOwner = parseRemoteURL(remote)

	return gc
}

func detectRunnerContext() RunnerContext {
	osName := runtime.GOOS
	switch osName {
	case "darwin":
		osName = "macOS"
	case "linux":
		osName = "Linux"
	case "windows":
		osName = "Windows"
	}

	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "X64"
	case "arm64":
		arch = "ARM64"
	}

	return RunnerContext{
		OS:   osName,
		Arch: arch,
		Temp: os.TempDir(),
	}
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func parseRemoteURL(remote string) (repo, owner string) {
	if remote == "" {
		return "", ""
	}
	// Handle SSH: git@github.com:owner/repo.git
	if strings.HasPrefix(remote, "git@") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) == 2 {
			path := strings.TrimSuffix(parts[1], ".git")
			return path, extractOwner(path)
		}
	}
	// Handle HTTPS: https://github.com/owner/repo.git
	remote = strings.TrimSuffix(remote, ".git")
	parts := strings.Split(remote, "/")
	if len(parts) >= 2 {
		owner := parts[len(parts)-2]
		name := parts[len(parts)-1]
		return owner + "/" + name, owner
	}
	return "", ""
}

func extractOwner(path string) string {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}
