package vmm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// VirtioFS manages a virtiofsd process for sharing a directory with a VM.
type VirtioFS struct {
	SocketPath string
	SharedDir  string
	process    *exec.Cmd
}

// StartVirtioFS launches virtiofsd to share a directory.
func StartVirtioFS(ctx context.Context, socketPath, sharedDir string) (*VirtioFS, error) {
	// Clean up any stale socket
	os.Remove(socketPath)

	cmd := exec.CommandContext(ctx, "virtiofsd",
		"--socket-path", socketPath,
		"--shared-dir", sharedDir,
		"--sandbox", "none",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting virtiofsd: %w", err)
	}

	vfs := &VirtioFS{
		SocketPath: socketPath,
		SharedDir:  sharedDir,
		process:    cmd,
	}

	return vfs, nil
}

// Stop kills the virtiofsd process and cleans up the socket.
func (vfs *VirtioFS) Stop() {
	if vfs.process != nil && vfs.process.Process != nil {
		vfs.process.Process.Kill()
		vfs.process.Wait()
	}
	os.Remove(vfs.SocketPath)
}
