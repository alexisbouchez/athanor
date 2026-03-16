package vmm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// VMCreateConfig is the full VM configuration for cloud-hypervisor's create API.
type VMCreateConfig struct {
	CPUs    CPUsConfig    `json:"cpus"`
	Memory  MemoryConfig  `json:"memory"`
	Payload PayloadConfig `json:"payload"`
	Disks   []DiskConfig  `json:"disks"`
	Net     []NetConfig   `json:"net,omitempty"`
	Fs      []FsConfig    `json:"fs,omitempty"`
	Serial  ConsoleConfig `json:"serial"`
	Console ConsoleConfig `json:"console"`
}

// DiskConfig describes a disk attachment.
type DiskConfig struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly,omitempty"`
}

// NetConfig describes a network interface.
type NetConfig struct {
	Tap string `json:"tap,omitempty"`
}

// FsConfig describes a virtiofs shared directory.
type FsConfig struct {
	Tag    string `json:"tag"`
	Socket string `json:"socket"`
}

// ConsoleConfig describes a console output mode.
type ConsoleConfig struct {
	Mode string `json:"mode"` // "Off", "Pty", "Tty", "File", "Null"
}

// VM represents a single CloudHypervisor microVM lifecycle.
type VM struct {
	ID         string
	SocketPath string
	IP         string
	Config     VMCreateConfig

	process    *exec.Cmd
	client     *http.Client
	logger     *log.Logger
}

// VMOptions configures a new VM.
type VMOptions struct {
	ID           string
	KernelPath   string
	RootfsPath   string
	TapDevice    string
	IP           string
	VirtioFSSock string
	CPUs         int
	MemoryMB     int
	SocketDir    string // directory for API sockets, default /tmp
}

// NewVM creates a new VM instance (does not start it).
func NewVM(opts VMOptions) *VM {
	if opts.CPUs == 0 {
		opts.CPUs = 1
	}
	if opts.MemoryMB == 0 {
		opts.MemoryMB = 1024
	}
	if opts.SocketDir == "" {
		opts.SocketDir = "/tmp"
	}

	socketPath := filepath.Join(opts.SocketDir, fmt.Sprintf("athanor-vm-%s.sock", opts.ID))

	// Kernel cmdline with static IP configuration
	cmdline := fmt.Sprintf(
		"console=ttyS0 root=/dev/vda rw ip=%s::192.168.100.1:255.255.255.0::eth0:off nameserver=8.8.8.8",
		opts.IP,
	)

	config := VMCreateConfig{
		CPUs:   CPUsConfig{BootVCPUs: uint32(opts.CPUs), MaxVCPUs: uint32(opts.CPUs)},
		Memory: MemoryConfig{Size: uint64(opts.MemoryMB) * 1024 * 1024},
		Payload: PayloadConfig{
			Kernel:  opts.KernelPath,
			Cmdline: cmdline,
		},
		Disks: []DiskConfig{
			{Path: opts.RootfsPath},
		},
		Serial:  ConsoleConfig{Mode: "Null"},
		Console: ConsoleConfig{Mode: "Off"},
	}

	if opts.TapDevice != "" {
		config.Net = []NetConfig{{Tap: opts.TapDevice}}
	}
	if opts.VirtioFSSock != "" {
		config.Fs = []FsConfig{{Tag: "workspace", Socket: opts.VirtioFSSock}}
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	return &VM{
		ID:         opts.ID,
		SocketPath: socketPath,
		IP:         opts.IP,
		Config:     config,
		client: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		logger: log.New(os.Stderr, fmt.Sprintf("[vm:%s] ", opts.ID), log.LstdFlags),
	}
}

// Start launches the cloud-hypervisor process, creates the VM, and boots it.
func (vm *VM) Start(ctx context.Context) error {
	// Clean up any stale socket
	os.Remove(vm.SocketPath)

	// Launch cloud-hypervisor process
	vm.process = exec.CommandContext(ctx, "cloud-hypervisor", "--api-socket", vm.SocketPath)
	vm.process.Stdout = os.Stderr
	vm.process.Stderr = os.Stderr
	if err := vm.process.Start(); err != nil {
		return fmt.Errorf("starting cloud-hypervisor: %w", err)
	}

	vm.logger.Printf("Started cloud-hypervisor (PID %d)", vm.process.Process.Pid)

	// Wait for API socket to be ready
	if err := vm.waitSocket(ctx); err != nil {
		vm.process.Process.Kill()
		return fmt.Errorf("waiting for API socket: %w", err)
	}

	// Create VM
	if err := vm.apiPut(ctx, "/api/v1/vm.create", vm.Config); err != nil {
		vm.process.Process.Kill()
		return fmt.Errorf("creating VM: %w", err)
	}

	// Boot VM
	if err := vm.apiPut(ctx, "/api/v1/vm.boot", nil); err != nil {
		vm.process.Process.Kill()
		return fmt.Errorf("booting VM: %w", err)
	}

	vm.logger.Printf("VM booted, IP=%s", vm.IP)
	return nil
}

// WaitSSH waits for SSH to become reachable on the VM.
func (vm *VM) WaitSSH(ctx context.Context) error {
	addr := vm.IP + ":22"
	deadline := time.After(60 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for SSH on %s", addr)
		default:
			conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
			if err == nil {
				conn.Close()
				vm.logger.Printf("SSH reachable")
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
	}
}

// Destroy shuts down and cleans up the VM.
func (vm *VM) Destroy(ctx context.Context) error {
	vm.logger.Printf("Destroying VM")

	// Try graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	vm.apiPut(shutdownCtx, "/api/v1/vm.shutdown", nil) //nolint: ignore error

	// Kill the process
	if vm.process != nil && vm.process.Process != nil {
		vm.process.Process.Kill()
		vm.process.Wait()
	}

	// Clean up socket
	os.Remove(vm.SocketPath)

	return nil
}

func (vm *VM) waitSocket(ctx context.Context) error {
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for socket %s", vm.SocketPath)
		default:
			if _, err := os.Stat(vm.SocketPath); err == nil {
				// Socket file exists, try connecting
				conn, err := net.Dial("unix", vm.SocketPath)
				if err == nil {
					conn.Close()
					return nil
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (vm *VM) apiPut(ctx context.Context, path string, body any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := "http://localhost" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := vm.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}
	return nil
}
