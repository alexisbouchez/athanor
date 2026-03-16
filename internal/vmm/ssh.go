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

		// Build the command to run
		shell := opts.Shell
		if shell == "" || shell == "__node__" {
			shell = "bash"
		}

		workDir := opts.WorkingDirectory
		if workDir == "" {
			workDir = "/workspace"
		}

		// Build env export block (SSH Setenv is usually disabled)
		// Only export CI-relevant vars, not the entire host environment
		var envExports strings.Builder
		for _, env := range opts.Env {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			// Skip host-only vars and large vars that break SSH
			switch {
			case key == "PATH":
				// Use VM's PATH, don't override
				continue
			case strings.HasPrefix(key, "GITHUB_"):
				// Always export GITHUB_* vars
			case strings.HasPrefix(key, "CI"):
			case strings.HasPrefix(key, "RUNNER_"):
			case strings.HasPrefix(key, "INPUT_"):
			case key == "HOME":
			case key == "GOPATH":
			case key == "GOMODCACHE":
			default:
				continue
			}
			envExports.WriteString(fmt.Sprintf("export %s=%s\n", key, shellQuote(parts[1])))
		}

		// Preamble: mount workspace, set PATH (including GITHUB_PATH additions), set HOME
		remoteScript := fmt.Sprintf(
			"export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH; export HOME=/root; "+
				"modprobe virtiofs 2>/dev/null; mount -t virtiofs workspace /workspace 2>/dev/null; "+
				"git config --global safe.directory '*' 2>/dev/null; "+
				"if [ -f /tmp/gh-path ]; then while IFS= read -r p; do export PATH=\"$p:$PATH\"; done < /tmp/gh-path; fi; "+
				"if [ -f /tmp/gh-env ]; then set -a; . /tmp/gh-env 2>/dev/null; set +a; fi; "+
				"%scd %s && cat > /tmp/athanor-step.sh << 'ATHANOR_SCRIPT_EOF'\n%s\nATHANOR_SCRIPT_EOF\n%s /tmp/athanor-step.sh",
			envExports.String(), shellQuote(workDir), script, shell,
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
