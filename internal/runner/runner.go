package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alexj212/athanor/internal/action"
	"github.com/alexj212/athanor/internal/expr"
	"github.com/alexj212/athanor/internal/workflow"
)

// RunEvent types sent from the runner to observers.
type RunEvent interface {
	runEvent()
}

type WorkflowStarted struct{ Name string }
type JobStarted struct{ JobID string }
type StepStarted struct {
	JobID    string
	StepIdx  int
	StepName string
}
type StepOutput struct {
	JobID   string
	StepIdx int
	Line    string
}
type StepFinished struct {
	JobID    string
	StepIdx  int
	ExitCode int
	Skipped  bool
	Error    string
}
type JobFinished struct {
	JobID  string
	Status string // "success", "failure", "skipped"
}
type WorkflowFinished struct {
	Status string // "success", "failure"
}

func (WorkflowStarted) runEvent()  {}
func (JobStarted) runEvent()       {}
func (StepStarted) runEvent()      {}
func (StepOutput) runEvent()       {}
func (StepFinished) runEvent()     {}
func (JobFinished) runEvent()      {}
func (WorkflowFinished) runEvent() {}

// JobLifecycle manages per-job setup and teardown (e.g., spinning up a microVM).
type JobLifecycle interface {
	// Setup prepares the execution environment for a job.
	// Returns the exec function to use, the workspace path inside the environment, and an error.
	Setup(ctx context.Context, jobID string, hostWorkspace string) (ExecStepFunc, string, error)
	// Teardown cleans up after a job completes.
	Teardown(ctx context.Context, jobID string) error
}

// Runner executes a workflow.
type Runner struct {
	wf        *workflow.Workflow
	events    chan RunEvent
	exec      ExecStepFunc
	runCtx    *RunContext
	lifecycle JobLifecycle
}

// NewRunner creates a runner for the given workflow.
func NewRunner(wf *workflow.Workflow) *Runner {
	return &Runner{
		wf:     wf,
		events: make(chan RunEvent, 256),
		exec:   ExecStep,
		runCtx: NewRunContext(),
	}
}

// NewRunnerWithContext creates a runner with a pre-built RunContext.
func NewRunnerWithContext(wf *workflow.Workflow, runCtx *RunContext) *Runner {
	return &Runner{
		wf:     wf,
		events: make(chan RunEvent, 256),
		exec:   ExecStep,
		runCtx: runCtx,
	}
}

// NewRunnerWithLifecycle creates a runner with a VM lifecycle manager.
func NewRunnerWithLifecycle(wf *workflow.Workflow, runCtx *RunContext, lc JobLifecycle) *Runner {
	return &Runner{
		wf:        wf,
		events:    make(chan RunEvent, 256),
		exec:      ExecStep,
		runCtx:    runCtx,
		lifecycle: lc,
	}
}

// NewRunnerWithExec creates a runner with a custom exec function (for testing).
func NewRunnerWithExec(wf *workflow.Workflow, execFn ExecStepFunc) *Runner {
	return &Runner{
		wf:     wf,
		events: make(chan RunEvent, 256),
		exec:   execFn,
		runCtx: NewRunContext(),
	}
}

// Events returns the channel on which run events are sent.
func (r *Runner) Events() <-chan RunEvent {
	return r.events
}

// Run executes the workflow. It closes the events channel when done.
func (r *Runner) Run(ctx context.Context) {
	defer close(r.events)

	r.events <- WorkflowStarted{Name: r.wf.Name}

	// Expand matrix jobs into virtual jobs
	expandedJobs, matrixContexts := r.expandMatrixJobs()

	levels, err := topoSort(expandedJobs)
	if err != nil {
		r.events <- WorkflowFinished{Status: "failure"}
		return
	}

	jobResults := make(map[string]*JobResult)
	overallStatus := "success"

	for _, level := range levels {
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Semaphore for max-parallel within matrix jobs
		// (we use the first job in the level to determine max-parallel)
		var sem chan struct{}
		if len(level) > 0 {
			if baseID := matrixBaseID(level[0]); baseID != "" {
				if origJob, ok := r.wf.Jobs[baseID]; ok && origJob.Strategy.MaxParallel > 0 {
					sem = make(chan struct{}, origJob.Strategy.MaxParallel)
				}
			}
		}

		for _, jobID := range level {
			job := expandedJobs[jobID]

			// Check if dependencies succeeded
			skip := false
			for _, need := range job.Needs {
				if jr, ok := jobResults[need]; ok && jr.Status != "success" {
					skip = true
					break
				}
			}

			if skip {
				r.events <- JobStarted{JobID: jobID}
				r.events <- JobFinished{JobID: jobID, Status: "skipped"}
				mu.Lock()
				jobResults[jobID] = &JobResult{Status: "failure"}
				overallStatus = "failure"
				mu.Unlock()
				continue
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				if sem != nil {
					sem <- struct{}{}
					defer func() { <-sem }()
				}

				// Set up matrix context for this job
				mc := matrixContexts[jobID]

				// Set up needs context
				needsCtx := make(map[string]NeedContext)
				mu.Lock()
				for _, need := range job.Needs {
					if jr, ok := jobResults[need]; ok {
						needsCtx[need] = NeedContext{
							Outputs: jr.Outputs,
							Result:  jr.Status,
						}
					}
				}
				mu.Unlock()

				result := r.runJob(ctx, jobID, job, mc, needsCtx)
				mu.Lock()
				jobResults[jobID] = result
				if result.Status == "failure" {
					overallStatus = "failure"
				}
				mu.Unlock()
			}()
		}

		wg.Wait()

		// fail-fast: check if any matrix job in this level failed
		// and cancel siblings (already done since wg.Wait completed)
	}

	r.events <- WorkflowFinished{Status: overallStatus}
}

// runJob executes a single job and returns its result.
func (r *Runner) runJob(ctx context.Context, jobID string, job workflow.Job, matrixCtx map[string]any, needsCtx map[string]NeedContext) *JobResult {
	// Apply job timeout
	if job.TimeoutMinutes > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutMinutes)*time.Minute)
		defer cancel()
	} else {
		// Default: 360 minutes (GitHub default)
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 360*time.Minute)
		defer cancel()
	}

	r.events <- JobStarted{JobID: jobID}

	// Set up per-job lifecycle (e.g., spin up a microVM)
	execFn := r.exec
	if r.lifecycle != nil {
		fn, vmWorkspace, err := r.lifecycle.Setup(ctx, jobID, r.runCtx.GitHub.Workspace)
		if err != nil {
			r.events <- StepStarted{JobID: jobID, StepIdx: 0, StepName: "VM Setup"}
			r.events <- StepOutput{JobID: jobID, StepIdx: 0, Line: fmt.Sprintf("Failed to set up VM: %v", err)}
			r.events <- StepFinished{JobID: jobID, StepIdx: 0, ExitCode: 1, Error: err.Error()}
			r.events <- JobFinished{JobID: jobID, Status: "failure"}
			return &JobResult{Status: "failure"}
		}
		execFn = fn
		defer r.lifecycle.Teardown(ctx, jobID)
		// Override workspace to the VM-side path
		r.runCtx.GitHub.Workspace = vmWorkspace
	}
	// Use lifecycle-provided exec function for this job
	origExec := r.exec
	r.exec = execFn
	defer func() { r.exec = origExec }()

	// Set up run context for this job
	rc := *r.runCtx // copy
	rc.Matrix = matrixCtx
	rc.Needs = needsCtx
	rc.Steps = make(map[string]StepContext)
	rc.Job = JobContext{Status: "success"}

	// Merge workflow + job env into context env
	rc.Env = make(map[string]string)
	for k, v := range r.wf.Env {
		rc.Env[k] = v
	}
	for k, v := range job.Env {
		rc.Env[k] = v
	}

	// Build job-level env
	env := NewEnv(rc.DefaultEnvVars(jobID, ""), r.wf.Env, job.Env)

	// Track extra env from GITHUB_ENV and extra PATH from GITHUB_PATH
	var extraEnv map[string]string
	var extraPath []string

	jobFailed := false
	for i, step := range job.Steps {
		// Check context cancellation
		if ctx.Err() != nil {
			rc.Job.Status = "failure"
			jobFailed = true
			break
		}

		// Build expression context for condition evaluation
		exprCtx := rc.ToMap()
		if jobFailed {
			exprCtx["__job_status"] = "failure"
		} else {
			exprCtx["__job_status"] = "success"
		}
		if ctx.Err() == context.Canceled {
			exprCtx["__cancelled"] = true
		}

		// Evaluate condition
		if !shouldRun(step.If, jobFailed, exprCtx) {
			name := stepName(step, i)
			// Interpolate step name
			if n, err := expr.Interpolate(name, exprCtx); err == nil {
				name = n
			}
			r.events <- StepStarted{JobID: jobID, StepIdx: i, StepName: name}
			r.events <- StepFinished{JobID: jobID, StepIdx: i, Skipped: true}
			if step.ID != "" {
				rc.Steps[step.ID] = StepContext{
					Outcome:    "skipped",
					Conclusion: "skipped",
					Outputs:    make(map[string]string),
				}
			}
			continue
		}

		// Interpolate step name
		name := stepName(step, i)
		if n, err := expr.Interpolate(name, exprCtx); err == nil {
			name = n
		}

		r.events <- StepStarted{JobID: jobID, StepIdx: i, StepName: name}

		// Apply step timeout
		stepCtx := ctx
		if step.TimeoutMinutes > 0 {
			var cancel context.CancelFunc
			stepCtx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutMinutes)*time.Minute)
			defer cancel()
		}

		// Dispatch: run: step vs uses: step
		var exitCode int
		var stepErr error
		var stepOutputs map[string]string

		if step.Uses != "" {
			exitCode, stepOutputs, stepErr = r.runUsesStep(stepCtx, jobID, i, step, env, exprCtx, extraEnv, extraPath)
		} else {
			exitCode, stepOutputs, stepErr = r.runRunStep(stepCtx, jobID, i, step, job, env, exprCtx, extraEnv, extraPath)
		}

		// Determine outcome
		outcome := "success"
		if stepErr != nil || exitCode != 0 {
			outcome = "failure"
		}
		conclusion := outcome
		if step.ContinueOnError && outcome == "failure" {
			conclusion = "success"
		}

		// Record step context
		if step.ID != "" {
			if stepOutputs == nil {
				stepOutputs = make(map[string]string)
			}
			rc.Steps[step.ID] = StepContext{
				Outputs:    stepOutputs,
				Outcome:    outcome,
				Conclusion: conclusion,
			}
		}

		if stepErr != nil {
			r.events <- StepFinished{JobID: jobID, StepIdx: i, ExitCode: 1, Error: stepErr.Error()}
			if !step.ContinueOnError {
				jobFailed = true
				rc.Job.Status = "failure"
				break
			}
			continue
		}

		r.events <- StepFinished{JobID: jobID, StepIdx: i, ExitCode: exitCode}

		if exitCode != 0 && !step.ContinueOnError {
			jobFailed = true
			rc.Job.Status = "failure"
			break
		}
	}

	status := "success"
	if jobFailed {
		status = "failure"
	}
	rc.Job.Status = status

	// Evaluate job outputs
	jobOutputs := make(map[string]string)
	if len(job.Outputs) > 0 {
		exprCtx := rc.ToMap()
		for name, exprStr := range job.Outputs {
			val, err := expr.Interpolate(exprStr, exprCtx)
			if err == nil {
				jobOutputs[name] = val
			}
		}
	}

	r.events <- JobFinished{JobID: jobID, Status: status}
	return &JobResult{Status: status, Outputs: jobOutputs}
}

// runRunStep executes a run: step.
func (r *Runner) runRunStep(ctx context.Context, jobID string, stepIdx int, step workflow.Step, job workflow.Job, env *Env, exprCtx map[string]any, extraEnv map[string]string, extraPath []string) (int, map[string]string, error) {
	// Build step env
	stepEnv := NewEnv(env.base, step.Env)
	stepEnv.outputs = env.outputs // share outputs

	// Apply extra env from GITHUB_ENV
	for k, v := range extraEnv {
		stepEnv.Set(k, v)
	}

	// Determine shell and working directory
	shell := step.Shell
	if shell == "" {
		shell = job.Defaults.Run.Shell
	}
	workDir := step.WorkingDirectory
	if workDir == "" {
		workDir = job.Defaults.Run.WorkingDirectory
	}
	if workDir != "" && !filepath.IsAbs(workDir) && r.runCtx.GitHub.Workspace != "" {
		workDir = filepath.Join(r.runCtx.GitHub.Workspace, workDir)
	}
	// Default to workspace if no working directory specified
	if workDir == "" && r.runCtx.GitHub.Workspace != "" {
		workDir = r.runCtx.GitHub.Workspace
	}

	// Create temp files for GITHUB_OUTPUT, GITHUB_ENV, GITHUB_PATH, GITHUB_STEP_SUMMARY
	outputFile, err := os.CreateTemp("", "athanor-output-*")
	if err != nil {
		return 1, nil, err
	}
	outputPath := outputFile.Name()
	outputFile.Close()
	defer os.Remove(outputPath)

	envFile, err := os.CreateTemp("", "athanor-env-*")
	if err != nil {
		return 1, nil, err
	}
	envPath := envFile.Name()
	envFile.Close()
	defer os.Remove(envPath)

	pathFile, err := os.CreateTemp("", "athanor-path-*")
	if err != nil {
		return 1, nil, err
	}
	pathPath := pathFile.Name()
	pathFile.Close()
	defer os.Remove(pathPath)

	summaryFile, err := os.CreateTemp("", "athanor-summary-*")
	if err != nil {
		return 1, nil, err
	}
	summaryPath := summaryFile.Name()
	summaryFile.Close()
	defer os.Remove(summaryPath)

	// When running in a VM, use VM-local paths for temp files since the host paths
	// are inaccessible from inside the VM. Output parsing is skipped for VM jobs.
	if r.lifecycle != nil {
		stepEnv.Set("GITHUB_OUTPUT", "/tmp/gh-output")
		stepEnv.Set("GITHUB_ENV", "/tmp/gh-env")
		stepEnv.Set("GITHUB_PATH", "/tmp/gh-path")
		stepEnv.Set("GITHUB_STEP_SUMMARY", "/tmp/gh-summary")
	} else {
		stepEnv.Set("GITHUB_OUTPUT", outputPath)
		stepEnv.Set("GITHUB_ENV", envPath)
		stepEnv.Set("GITHUB_PATH", pathPath)
		stepEnv.Set("GITHUB_STEP_SUMMARY", summaryPath)
	}

	// Apply extra PATH
	if len(extraPath) > 0 {
		currentPath := os.Getenv("PATH")
		stepEnv.Set("PATH", strings.Join(extraPath, string(os.PathListSeparator))+string(os.PathListSeparator)+currentPath)
	}

	// Interpolate the script
	script, err := expr.Interpolate(step.Run, exprCtx)
	if err != nil {
		return 1, nil, fmt.Errorf("interpolating script: %w", err)
	}

	// Stream output
	lines := make(chan string, 64)
	var lineWg sync.WaitGroup
	lineWg.Add(1)
	go func() {
		defer lineWg.Done()
		for line := range lines {
			r.events <- StepOutput{JobID: jobID, StepIdx: stepIdx, Line: line}
		}
	}()

	opts := ExecOptions{
		Shell:            shell,
		WorkingDirectory: workDir,
		Env:              stepEnv.List(),
		OutputPath:       outputPath,
	}

	result, execErr := r.exec(ctx, script, opts, lines)
	close(lines)
	lineWg.Wait()

	if execErr != nil {
		return 1, nil, fmt.Errorf("exec error: %w", execErr)
	}

	// Parse outputs
	var stepOutputs map[string]string
	if step.ID != "" {
		if err := env.ParseOutputFile(step.ID, outputPath); err != nil {
			return result.ExitCode, nil, err
		}
		stepOutputs = env.GetOutputs(step.ID)
	}

	// Parse GITHUB_ENV → apply to extraEnv for subsequent steps
	newEnvVars, err := ParseEnvFile(envPath)
	if err == nil && len(newEnvVars) > 0 {
		if extraEnv == nil {
			// Can't modify the caller's map reference, so we merge into env directly
			for k, v := range newEnvVars {
				env.Set(k, v)
			}
		} else {
			for k, v := range newEnvVars {
				env.Set(k, v)
			}
		}
	}

	// Parse GITHUB_PATH → prepend to extraPath for subsequent steps
	newPaths, err := ParsePathFile(pathPath)
	if err == nil && len(newPaths) > 0 {
		// Prepend to the env PATH for future steps
		for _, p := range newPaths {
			env.Set("PATH", p+string(os.PathListSeparator)+env.Get("PATH"))
		}
	}

	return result.ExitCode, stepOutputs, nil
}

// runUsesStep executes a uses: step.
func (r *Runner) runUsesStep(ctx context.Context, jobID string, stepIdx int, step workflow.Step, env *Env, exprCtx map[string]any, extraEnv map[string]string, extraPath []string) (int, map[string]string, error) {
	// Interpolate uses: value
	uses, err := expr.Interpolate(step.Uses, exprCtx)
	if err != nil {
		return 1, nil, fmt.Errorf("interpolating uses: %w", err)
	}

	ref, err := action.ParseRef(uses)
	if err != nil {
		return 1, nil, fmt.Errorf("parsing uses: %w", err)
	}

	// Interpolate with: values
	with := make(map[string]string, len(step.With))
	for k, v := range step.With {
		val, err := expr.Interpolate(v, exprCtx)
		if err != nil {
			return 1, nil, fmt.Errorf("interpolating with.%s: %w", k, err)
		}
		with[k] = val
	}

	// Check builtins first
	if builtin, ok := action.LookupBuiltin(ref); ok {
		r.events <- StepOutput{JobID: jobID, StepIdx: stepIdx, Line: fmt.Sprintf("Run %s (built-in)", ref)}
		outputs, err := builtin(ctx, with, r.runCtx.GitHub.Workspace)
		if err != nil {
			return 1, nil, err
		}
		return 0, outputs, nil
	}

	// Fetch/cache the action
	cacheDir := action.DefaultCacheDir()
	actionDir, err := action.Fetch(ctx, ref, cacheDir, r.runCtx.GitHub.Workspace)
	if err != nil {
		return 1, nil, err
	}

	// Load metadata
	meta, err := action.LoadMetadata(actionDir)
	if err != nil {
		return 1, nil, err
	}

	r.events <- StepOutput{JobID: jobID, StepIdx: stepIdx, Line: fmt.Sprintf("Run %s", ref)}

	switch {
	case meta.Runs.Using == "composite":
		return r.runCompositeAction(ctx, jobID, stepIdx, meta, with, env, exprCtx, actionDir)
	case strings.HasPrefix(meta.Runs.Using, "node"):
		return r.runNodeAction(ctx, jobID, stepIdx, meta, with, env, actionDir)
	case meta.Runs.Using == "docker":
		return 1, nil, fmt.Errorf("docker actions are not yet supported")
	default:
		return 1, nil, fmt.Errorf("unsupported action type: %q", meta.Runs.Using)
	}
}

// runCompositeAction runs a composite action's steps inline.
func (r *Runner) runCompositeAction(ctx context.Context, jobID string, stepIdx int, meta *action.ActionMetadata, with map[string]string, env *Env, exprCtx map[string]any, actionDir string) (int, map[string]string, error) {
	// Build inputs from with + defaults
	inputs := make(map[string]string)
	for name, def := range meta.Inputs {
		if val, ok := with[name]; ok {
			inputs[name] = val
		} else if def.Default != "" {
			inputs[name] = def.Default
		}
	}

	// Build INPUT_* env vars
	inputEnv := make(map[string]string, len(inputs))
	for k, v := range inputs {
		inputEnv["INPUT_"+strings.ToUpper(strings.ReplaceAll(k, "-", "_"))] = v
	}

	// Build inputs context for expressions
	inputsCtx := make(map[string]any, len(inputs))
	for k, v := range inputs {
		inputsCtx[k] = v
	}

	compositeOutputs := make(map[string]map[string]string) // stepID -> outputs
	failed := false

	for si, cstep := range meta.Runs.Steps {
		if ctx.Err() != nil {
			failed = true
			break
		}

		if cstep.Run == "" && cstep.Uses == "" {
			continue
		}

		// Build composite step expression context
		cExprCtx := make(map[string]any)
		for k, v := range exprCtx {
			cExprCtx[k] = v
		}
		cExprCtx["inputs"] = inputsCtx

		subName := cstep.Name
		if subName == "" {
			subName = fmt.Sprintf("Sub-step %d", si+1)
		}
		if n, err := expr.Interpolate(subName, cExprCtx); err == nil {
			subName = n
		}

		r.events <- StepOutput{JobID: jobID, StepIdx: stepIdx, Line: fmt.Sprintf("  > %s", subName)}

		if cstep.Run != "" {
			script, err := expr.Interpolate(cstep.Run, cExprCtx)
			if err != nil {
				return 1, nil, fmt.Errorf("interpolating composite step: %w", err)
			}

			// Build env for composite step
			stepEnv := NewEnv(env.base, inputEnv, cstep.Env)
			stepEnv.outputs = env.outputs

			outputFile, err := os.CreateTemp("", "athanor-comp-output-*")
			if err != nil {
				return 1, nil, err
			}
			outputPath := outputFile.Name()
			outputFile.Close()
			defer os.Remove(outputPath)

			stepEnv.Set("GITHUB_OUTPUT", outputPath)

			shell := cstep.Shell
			if shell == "" {
				shell = "bash"
			}

			workDir := cstep.WorkingDirectory
			if workDir != "" && !filepath.IsAbs(workDir) {
				workDir = filepath.Join(actionDir, workDir)
			}

			lines := make(chan string, 64)
			var lineWg sync.WaitGroup
			lineWg.Add(1)
			go func() {
				defer lineWg.Done()
				for line := range lines {
					r.events <- StepOutput{JobID: jobID, StepIdx: stepIdx, Line: "    " + line}
				}
			}()

			opts := ExecOptions{
				Shell:            shell,
				WorkingDirectory: workDir,
				Env:              stepEnv.List(),
				OutputPath:       outputPath,
			}

			result, execErr := r.exec(ctx, script, opts, lines)
			close(lines)
			lineWg.Wait()

			if execErr != nil {
				return 1, nil, execErr
			}

			// Parse outputs
			if cstep.ID != "" {
				if err := env.ParseOutputFile(cstep.ID, outputPath); err != nil {
					return result.ExitCode, nil, err
				}
				compositeOutputs[cstep.ID] = env.GetOutputs(cstep.ID)
			}

			if result.ExitCode != 0 {
				failed = true
				break
			}
		}
		// TODO: nested uses: in composite actions
	}

	if failed {
		return 1, nil, nil
	}

	// Evaluate action output expressions
	stepsCtx := make(map[string]any, len(compositeOutputs))
	for id, outs := range compositeOutputs {
		outsMap := make(map[string]any, len(outs))
		for k, v := range outs {
			outsMap[k] = v
		}
		stepsCtx[id] = map[string]any{"outputs": outsMap}
	}
	outExprCtx := map[string]any{"steps": stepsCtx}

	outputs := make(map[string]string)
	for name, def := range meta.Outputs {
		if def.Value != "" {
			val, err := expr.Interpolate(def.Value, outExprCtx)
			if err == nil {
				outputs[name] = val
			}
		}
	}

	return 0, outputs, nil
}

// runNodeAction runs a Node.js action.
func (r *Runner) runNodeAction(ctx context.Context, jobID string, stepIdx int, meta *action.ActionMetadata, with map[string]string, env *Env, actionDir string) (int, map[string]string, error) {
	if meta.Runs.Main == "" {
		return 1, nil, fmt.Errorf("node action has no main entry point")
	}

	scriptPath := filepath.Join(actionDir, meta.Runs.Main)

	// Build inputs as INPUT_* env vars
	stepEnv := NewEnv(env.base)
	stepEnv.outputs = env.outputs
	for k, v := range with {
		stepEnv.Set("INPUT_"+strings.ToUpper(strings.ReplaceAll(k, "-", "_")), v)
	}

	// Apply defaults from action inputs
	for name, def := range meta.Inputs {
		envKey := "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		if _, ok := with[name]; !ok && def.Default != "" {
			stepEnv.Set(envKey, def.Default)
		}
	}

	outputFile, err := os.CreateTemp("", "athanor-node-output-*")
	if err != nil {
		return 1, nil, err
	}
	outputPath := outputFile.Name()
	outputFile.Close()
	defer os.Remove(outputPath)

	stepEnv.Set("GITHUB_OUTPUT", outputPath)

	lines := make(chan string, 64)
	var lineWg sync.WaitGroup
	lineWg.Add(1)
	go func() {
		defer lineWg.Done()
		for line := range lines {
			r.events <- StepOutput{JobID: jobID, StepIdx: stepIdx, Line: line}
		}
	}()

	opts := ExecOptions{
		Shell:            "__node__",
		WorkingDirectory: actionDir,
		Env:              stepEnv.List(),
		OutputPath:       outputPath,
	}

	result, execErr := ExecNode(ctx, scriptPath, opts, lines)
	close(lines)
	lineWg.Wait()

	if execErr != nil {
		return 1, nil, execErr
	}

	return result.ExitCode, nil, nil
}

// shouldRun evaluates whether a step should execute based on its if: condition.
func shouldRun(condition string, jobFailed bool, exprCtx map[string]any) bool {
	if condition == "" {
		return !jobFailed
	}

	// Try to evaluate as an expression
	val, err := expr.EvaluateExpression(condition, exprCtx)
	if err != nil {
		// Fallback: if evaluation fails, run on success
		return !jobFailed
	}

	return isTruthy(val)
}

// isTruthy checks GitHub Actions truthiness.
func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	case float64:
		return val != 0
	case int:
		return val != 0
	}
	return true
}

func stepName(step workflow.Step, idx int) string {
	if step.Name != "" {
		return step.Name
	}
	if step.Uses != "" {
		return step.Uses
	}
	return fmt.Sprintf("Step %d", idx+1)
}

// expandMatrixJobs expands jobs with matrix strategy into virtual jobs.
func (r *Runner) expandMatrixJobs() (map[string]workflow.Job, map[string]map[string]any) {
	expanded := make(map[string]workflow.Job)
	matrixContexts := make(map[string]map[string]any)

	for id, job := range r.wf.Jobs {
		if len(job.Strategy.Matrix.Values) == 0 && len(job.Strategy.Matrix.Include) == 0 {
			expanded[id] = job
			matrixContexts[id] = nil
			continue
		}

		combos := ExpandMatrix(job.Strategy.Matrix)
		if len(combos) == 0 {
			expanded[id] = job
			matrixContexts[id] = nil
			continue
		}

		for _, combo := range combos {
			virtualID := MatrixJobID(id, combo)
			virtualJob := job
			// Store the base job ID in the name for reference
			if virtualJob.Name == "" {
				virtualJob.Name = virtualID
			}
			expanded[virtualID] = virtualJob
			matrixContexts[virtualID] = combo
		}
	}

	return expanded, matrixContexts
}

// matrixBaseID extracts the base job ID from a matrix job ID.
func matrixBaseID(jobID string) string {
	if idx := strings.Index(jobID, " ("); idx != -1 {
		return jobID[:idx]
	}
	return ""
}

// topoSort does Kahn's algorithm topological sort, returning levels of parallelizable jobs.
func topoSort(jobs map[string]workflow.Job) ([][]string, error) {
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for id := range jobs {
		if _, ok := inDegree[id]; !ok {
			inDegree[id] = 0
		}
	}

	for id, job := range jobs {
		inDegree[id] += len(job.Needs)
		for _, need := range job.Needs {
			// For matrix jobs, the need might reference a base job ID
			// that was expanded into multiple virtual jobs
			found := false
			for depID := range jobs {
				if depID == need {
					dependents[need] = append(dependents[need], id)
					found = true
					break
				}
			}
			if !found {
				// Try matching base ID of matrix jobs
				for depID := range jobs {
					if matrixBaseID(depID) == need {
						dependents[depID] = append(dependents[depID], id)
					}
				}
			}
		}
	}

	var levels [][]string
	for {
		var level []string
		for id, deg := range inDegree {
			if deg == 0 {
				level = append(level, id)
			}
		}
		if len(level) == 0 {
			break
		}
		for _, id := range level {
			delete(inDegree, id)
			for _, dep := range dependents[id] {
				inDegree[dep]--
			}
		}
		levels = append(levels, level)
	}

	if len(inDegree) > 0 {
		return nil, fmt.Errorf("cycle detected in job dependencies")
	}

	return levels, nil
}
