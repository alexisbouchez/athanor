package vmm

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/alexj212/athanor/internal/runner"
)

// VMConfig holds configuration for VM-based job execution.
type VMJobConfig struct {
	KernelPath string
	RootfsPath string
	SSHKeyPath string
	DiskDir    string // directory for per-VM rootfs copies
	CPUs       int
	MemoryMB   int
}

// VMJobLifecycle implements runner.JobLifecycle using CloudHypervisor microVMs.
type VMJobLifecycle struct {
	cfg     VMJobConfig
	network *Network
	logger  *log.Logger
	mu      sync.Mutex
	vms     map[string]*vmState
}

type vmState struct {
	vm       *VM
	virtiofs *VirtioFS
	diskPath string
}

// NewVMJobLifecycle creates a new VM lifecycle manager.
func NewVMJobLifecycle(cfg VMJobConfig, network *Network) *VMJobLifecycle {
	return &VMJobLifecycle{
		cfg:     cfg,
		network: network,
		logger:  log.New(os.Stderr, "[vmm] ", log.LstdFlags),
		vms:     make(map[string]*vmState),
	}
}

// Setup spins up a microVM for the given job.
func (l *VMJobLifecycle) Setup(ctx context.Context, jobID string, hostWorkspace string) (runner.ExecStepFunc, string, error) {
	l.logger.Printf("Setting up VM for job %s", jobID)

	// Sanitize job ID for use in filenames
	safeID := sanitizeID(jobID)

	// 1. Copy rootfs for this VM
	diskPath := filepath.Join(l.cfg.DiskDir, fmt.Sprintf("rootfs-%s.ext4", safeID))
	if err := copyFile(l.cfg.RootfsPath, diskPath); err != nil {
		return nil, "", fmt.Errorf("copying rootfs: %w", err)
	}

	// 2. Allocate network
	tapName, ipAddr, err := l.network.AllocateTap(ctx, safeID)
	if err != nil {
		os.Remove(diskPath)
		return nil, "", fmt.Errorf("allocating network: %w", err)
	}

	// 3. Start virtiofsd
	vfsSockPath := fmt.Sprintf("/tmp/athanor-virtiofs-%s.sock", safeID)
	vfs, err := StartVirtioFS(ctx, vfsSockPath, hostWorkspace)
	if err != nil {
		l.network.FreeTap(ctx, safeID)
		os.Remove(diskPath)
		return nil, "", fmt.Errorf("starting virtiofsd: %w", err)
	}

	// 4. Create and start VM
	vm := NewVM(VMOptions{
		ID:           safeID,
		KernelPath:   l.cfg.KernelPath,
		RootfsPath:   diskPath,
		TapDevice:    tapName,
		IP:           ipAddr,
		VirtioFSSock: vfsSockPath,
		CPUs:         l.cfg.CPUs,
		MemoryMB:     l.cfg.MemoryMB,
	})

	if err := vm.Start(ctx); err != nil {
		l.logger.Printf("Failed to start VM: %v", err)
		vfs.Stop()
		l.network.FreeTap(ctx, safeID)
		os.Remove(diskPath)
		return nil, "", fmt.Errorf("starting VM: %w", err)
	}

	// 5. Wait for SSH
	l.logger.Printf("Waiting for SSH on %s:22...", ipAddr)
	if err := vm.WaitSSH(ctx); err != nil {
		l.logger.Printf("SSH wait failed: %v", err)
		vm.Destroy(ctx)
		vfs.Stop()
		l.network.FreeTap(ctx, safeID)
		os.Remove(diskPath)
		return nil, "", fmt.Errorf("waiting for SSH: %w", err)
	}

	// Store state for teardown
	l.mu.Lock()
	l.vms[jobID] = &vmState{
		vm:       vm,
		virtiofs: vfs,
		diskPath: diskPath,
	}
	l.mu.Unlock()

	l.logger.Printf("VM ready for job %s (IP=%s)", jobID, ipAddr)

	// Return SSH executor and VM-side workspace path
	sshAddr := ipAddr + ":22"
	execFn := NewSSHExec(sshAddr, l.cfg.SSHKeyPath)
	return execFn, "/workspace", nil
}

// Teardown destroys the VM for the given job.
func (l *VMJobLifecycle) Teardown(ctx context.Context, jobID string) error {
	l.mu.Lock()
	state, ok := l.vms[jobID]
	if ok {
		delete(l.vms, jobID)
	}
	l.mu.Unlock()

	if !ok {
		return nil
	}

	safeID := sanitizeID(jobID)
	l.logger.Printf("Tearing down VM for job %s", jobID)

	// Destroy VM
	state.vm.Destroy(ctx)

	// Stop virtiofsd
	state.virtiofs.Stop()

	// Free network
	l.network.FreeTap(ctx, safeID)

	// Remove rootfs copy
	os.Remove(state.diskPath)

	return nil
}

func sanitizeID(id string) string {
	result := make([]byte, 0, len(id))
	for _, b := range []byte(id) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' {
			result = append(result, b)
		}
	}
	if len(result) > 32 {
		result = result[:32]
	}
	return string(result)
}

func copyFile(src, dst string) error {
	cmd := exec.Command("cp", "--reflink=auto", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cp %s %s: %s: %w", src, dst, string(out), err)
	}
	return nil
}
