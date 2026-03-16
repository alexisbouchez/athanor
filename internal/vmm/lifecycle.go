package vmm

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alexj212/athanor/internal/runner"
)

// VMJobConfig holds configuration for VM-based job execution.
type VMJobConfig struct {
	KernelPath  string
	RootfsPath  string
	SSHKeyPath  string
	DiskDir     string // directory for per-VM rootfs copies
	CPUs        int
	MemoryMB    int
	MaxParallel int // max concurrent VMs (0 = auto)
}

// VMJobLifecycle implements runner.JobLifecycle using CloudHypervisor microVMs.
type VMJobLifecycle struct {
	cfg     VMJobConfig
	network *Network
	logger  *log.Logger
	mu      sync.Mutex
	vms     map[string]*vmState
	sem     chan struct{} // limits concurrent VMs
}

type vmState struct {
	vm       *VM
	virtiofs *VirtioFS
	diskPath string
}

// NewVMJobLifecycle creates a new VM lifecycle manager.
func NewVMJobLifecycle(cfg VMJobConfig, network *Network) *VMJobLifecycle {
	maxVMs := cfg.MaxParallel
	if maxVMs <= 0 {
		// Auto: estimate from host memory (~1.5GB overhead for OS + athanor)
		hostMemMB := getHostMemoryMB()
		availableMB := hostMemMB - 1536
		if availableMB < cfg.MemoryMB {
			availableMB = cfg.MemoryMB
		}
		maxVMs = availableMB / cfg.MemoryMB
	}
	if maxVMs < 1 {
		maxVMs = 1
	}
	logger := log.New(os.Stderr, "[vmm] ", log.LstdFlags)
	logger.Printf("Max parallel VMs: %d", maxVMs)

	// Startup cleanup: kill orphaned cloud-hypervisor/virtiofsd, delete stale TAPs and disks
	exec.Command("pkill", "-f", "cloud-hypervisor.*athanor").Run()
	exec.Command("pkill", "-f", "virtiofsd.*athanor").Run()
	// Clean stale TAP devices
	if out, err := exec.Command("ip", "-o", "link", "show").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if idx := strings.Index(line, "tap-"); idx >= 0 {
				end := strings.IndexAny(line[idx:], " :@")
				if end > 0 {
					tap := line[idx : idx+end]
					logger.Printf("Cleaning stale TAP: %s", tap)
					exec.Command("ip", "link", "delete", tap).Run()
				}
			}
		}
	}
	// Clean stale disk copies
	if entries, err := os.ReadDir(cfg.DiskDir); err == nil {
		for _, e := range entries {
			os.Remove(filepath.Join(cfg.DiskDir, e.Name()))
		}
	}

	return &VMJobLifecycle{
		cfg:     cfg,
		network: network,
		logger:  log.New(os.Stderr, "[vmm] ", log.LstdFlags),
		vms:     make(map[string]*vmState),
		sem:     make(chan struct{}, maxVMs),
	}
}

// Setup spins up a microVM for the given job.
func (l *VMJobLifecycle) Setup(ctx context.Context, jobID string, hostWorkspace string) (runner.ExecStepFunc, string, error) {
	l.logger.Printf("Waiting for VM slot for job %s", jobID)
	select {
	case l.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	l.logger.Printf("Setting up VM for job %s", jobID)

	// Sanitize job ID for use in filenames
	safeID := sanitizeID(jobID)

	// On any setup failure, release the semaphore slot
	setupFailed := true
	defer func() {
		if setupFailed {
			<-l.sem
		}
	}()

	// 1. Copy rootfs for this VM
	diskPath := filepath.Join(l.cfg.DiskDir, fmt.Sprintf("rootfs-%s.ext4", safeID))
	if err := copyFile(l.cfg.RootfsPath, diskPath); err != nil {
		return nil, "", fmt.Errorf("copying rootfs: %w", err)
	}

	// 2. Allocate network — delete stale TAP if it exists from a previous crash
	l.network.FreeTap(ctx, safeID) // ignore error
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
	setupFailed = false

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
		<-l.sem // release slot
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

	// Release VM slot
	<-l.sem

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

func getHostMemoryMB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 4096 // fallback 4GB
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb int
			fmt.Sscanf(line, "MemTotal: %d kB", &kb)
			return kb / 1024
		}
	}
	return 4096
}

func copyFile(src, dst string) error {
	cmd := exec.Command("cp", "--reflink=auto", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cp %s %s: %s: %w", src, dst, string(out), err)
	}
	return nil
}
