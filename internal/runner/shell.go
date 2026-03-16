package runner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// StepResult holds the result of executing a step.
type StepResult struct {
	ExitCode int
	Outputs  map[string]string
}

// ExecOptions configures step execution.
type ExecOptions struct {
	Shell            string
	WorkingDirectory string
	Env              []string
	OutputPath       string // path to GITHUB_OUTPUT file
	Container        string // if set, run inside this Docker image
	ContainerEnv     map[string]string
	ContainerVolumes []string
	Services         map[string]ServiceConfig
}

// ServiceConfig defines a service container.
type ServiceConfig struct {
	Image   string
	Env     map[string]string
	Ports   []string
	Volumes []string
}

// ExecStep writes the script to a temp file and executes it, streaming output lines.
// If opts.Container is set, the script runs inside a Docker container.
func ExecStep(ctx context.Context, script string, opts ExecOptions, lines chan<- string) (*StepResult, error) {
	// Write script to temp file
	tmp, err := os.CreateTemp("", "athanor-step-*.sh")
	if err != nil {
		return nil, fmt.Errorf("creating temp script: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("writing script: %w", err)
	}
	tmp.Close()

	shell := opts.Shell
	if shell == "" {
		shell = "bash"
	}

	var cmd *exec.Cmd
	if opts.Container != "" {
		// Run inside Docker container
		args := []string{"run", "--rm", "-i"}
		// Mount workspace
		if opts.WorkingDirectory != "" {
			args = append(args, "-v", opts.WorkingDirectory+":"+opts.WorkingDirectory, "-w", opts.WorkingDirectory)
		}
		// Mount the script
		args = append(args, "-v", tmp.Name()+":"+tmp.Name())
		// Mount output file
		if opts.OutputPath != "" {
			args = append(args, "-v", opts.OutputPath+":"+opts.OutputPath)
		}
		// Container env
		for k, v := range opts.ContainerEnv {
			args = append(args, "-e", k+"="+v)
		}
		// Env from opts
		for _, env := range opts.Env {
			args = append(args, "-e", env)
		}
		// Volumes
		for _, vol := range opts.ContainerVolumes {
			args = append(args, "-v", vol)
		}
		args = append(args, opts.Container, shell, tmp.Name())
		cmd = exec.CommandContext(ctx, "docker", args...)
	} else {
		cmd = exec.CommandContext(ctx, shell, tmp.Name())
	}
	cmd.Env = opts.Env
	if opts.WorkingDirectory != "" {
		cmd.Dir = opts.WorkingDirectory
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting command: %w", err)
	}

	// Stream output line by line
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if lines != nil {
				lines <- scanner.Text()
			}
		}
	}()

	wg.Wait()
	err = cmd.Wait()

	result := &StepResult{ExitCode: 0}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("waiting for command: %w", err)
		}
	}

	return result, nil
}

// ExecStepFunc is the function signature for step execution, allowing injection in tests.
type ExecStepFunc func(ctx context.Context, script string, opts ExecOptions, lines chan<- string) (*StepResult, error)

// ExecNode executes a Node.js script, streaming output lines.
func ExecNode(ctx context.Context, scriptPath string, opts ExecOptions, lines chan<- string) (*StepResult, error) {
	cmd := exec.CommandContext(ctx, "node", scriptPath)
	cmd.Env = opts.Env
	if opts.WorkingDirectory != "" {
		cmd.Dir = opts.WorkingDirectory
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting node: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if lines != nil {
				lines <- scanner.Text()
			}
		}
	}()

	wg.Wait()
	err = cmd.Wait()

	result := &StepResult{ExitCode: 0}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("waiting for node: %w", err)
		}
	}

	return result, nil
}
