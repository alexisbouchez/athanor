package vmm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// Monitor polls the cloud-hypervisor HTTP API over a Unix socket
// and reports VM status.
type Monitor struct {
	socketPath string
	interval   time.Duration
	client     *http.Client
}

// NewMonitor creates a monitor that polls the given cloud-hypervisor API socket.
func NewMonitor(socketPath string, interval time.Duration) *Monitor {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	return &Monitor{
		socketPath: socketPath,
		interval:   interval,
		client: &http.Client{
			Transport: transport,
			Timeout:   3 * time.Second,
		},
	}
}

// VMInfo holds the response from the cloud-hypervisor /api/v1/vm.info endpoint.
type VMInfo struct {
	State       string      `json:"state"`
	Config      *VMConfig   `json:"config,omitempty"`
	MemorySize  uint64      `json:"memory_actual_size,omitempty"`
	DeviceTree  any         `json:"device_tree,omitempty"`
}

// VMConfig holds a subset of the VM configuration returned by cloud-hypervisor.
type VMConfig struct {
	CPUs    *CPUsConfig    `json:"cpus,omitempty"`
	Memory  *MemoryConfig  `json:"memory,omitempty"`
	Payload *PayloadConfig `json:"payload,omitempty"`
}

// CPUsConfig describes the CPU topology.
type CPUsConfig struct {
	BootVCPUs uint32 `json:"boot_vcpus"`
	MaxVCPUs  uint32 `json:"max_vcpus"`
}

// MemoryConfig describes memory allocation.
type MemoryConfig struct {
	Size   uint64 `json:"size"`
	Shared bool   `json:"shared,omitempty"`
}

// PayloadConfig describes the kernel/initramfs payload.
type PayloadConfig struct {
	Kernel  string `json:"kernel,omitempty"`
	Initrd  string `json:"initramfs,omitempty"`
	Cmdline string `json:"cmdline,omitempty"`
}

// VMCounters holds the response from /api/v1/vm.counters.
type VMCounters struct {
	Counters map[string]map[string]uint64 `json:"counters,omitempty"`
}

// Run polls the cloud-hypervisor API until the context is cancelled.
func (m *Monitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *Monitor) poll(ctx context.Context) {
	info, err := m.getVMInfo(ctx)
	if err != nil {
		log.Printf("poll: vm.info: %v", err)
		return
	}

	log.Printf("vm state=%s", info.State)

	if info.Config != nil {
		if info.Config.CPUs != nil {
			log.Printf("  cpus: boot=%d max=%d", info.Config.CPUs.BootVCPUs, info.Config.CPUs.MaxVCPUs)
		}
		if info.Config.Memory != nil {
			log.Printf("  memory: %d MiB", info.Config.Memory.Size/(1024*1024))
		}
	}
}

// GetVMInfo retrieves VM information from the cloud-hypervisor API.
func (m *Monitor) GetVMInfo(ctx context.Context) (*VMInfo, error) {
	return m.getVMInfo(ctx)
}

func (m *Monitor) getVMInfo(ctx context.Context) (*VMInfo, error) {
	return apiGet[VMInfo](ctx, m.client, "http://localhost/api/v1/vm.info")
}

// GetVMCounters retrieves VM counters from the cloud-hypervisor API.
func (m *Monitor) GetVMCounters(ctx context.Context) (*VMCounters, error) {
	return apiGet[VMCounters](ctx, m.client, "http://localhost/api/v1/vm.counters")
}

func apiGet[T any](ctx context.Context, client *http.Client, url string) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
