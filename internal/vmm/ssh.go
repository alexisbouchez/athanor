package vmm

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/alexj212/athanor/internal/runner"
)

// NewSSHExec creates an ExecStepFunc that runs commands over SSH inside a VM.
func NewSSHExec(addr string, keyPath string) runner.ExecStepFunc {
	return func(ctx context.Context, script string, opts runner.ExecOptions, lines chan<- string) (*runner.StepResult, error) {
		client, err := dialSSH(addr, keyPath)
		if err != nil {
			return nil, fmt.Errorf("ssh connect: %w", err)
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("ssh session: %w", err)
		}
		defer session.Close()

		// Set environment variables
		for _, env := range opts.Env {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				session.Setenv(parts[0], parts[1])
			}
		}

		// Build the command to run
		// Write script to a temp file on the remote, then execute it
		shell := opts.Shell
		if shell == "" || shell == "__node__" {
			shell = "bash"
		}

		workDir := opts.WorkingDirectory
		if workDir == "" {
			workDir = "/workspace"
		}

		// Escape the script for heredoc
		remoteScript := fmt.Sprintf(
			"cd %s && cat > /tmp/athanor-step.sh << 'ATHANOR_SCRIPT_EOF'\n%s\nATHANOR_SCRIPT_EOF\n%s /tmp/athanor-step.sh",
			shellQuote(workDir), script, shell,
		)

		// Set up stdout/stderr streaming
		stdout, err := session.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("stdout pipe: %w", err)
		}
		session.Stderr = session.Stdout

		if err := session.Start(remoteScript); err != nil {
			return nil, fmt.Errorf("starting command: %w", err)
		}

		// Stream output
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if lines != nil {
				lines <- scanner.Text()
			}
		}

		err = session.Wait()
		result := &runner.StepResult{ExitCode: 0}
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				result.ExitCode = exitErr.ExitStatus()
			} else {
				return nil, fmt.Errorf("waiting for command: %w", err)
			}
		}

		return result, nil
	}
}

func dialSSH(addr, keyPath string) (*ssh.Client, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("reading SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parsing SSH key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * 1e9, // 10s
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
