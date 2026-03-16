package vmm

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// Network manages TAP devices and IP allocation for VMs.
type Network struct {
	mu      sync.Mutex
	nextIP  int
	bridge  string
	allocs  map[string]string // vmID -> tapName
}

// NewNetwork creates a network manager. The bridge must already exist.
func NewNetwork(bridge string) *Network {
	return &Network{
		nextIP: 2, // .1 is the bridge itself
		bridge: bridge,
		allocs: make(map[string]string),
	}
}

// AllocateTap creates a TAP device and attaches it to the bridge.
// Returns the TAP name and the assigned IP address.
func (n *Network) AllocateTap(ctx context.Context, vmID string) (tapName, ipAddr string, err error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	ip := n.nextIP
	n.nextIP++
	if ip > 254 {
		return "", "", fmt.Errorf("no more IPs available")
	}

	tapName = fmt.Sprintf("tap-%s", vmID)
	if len(tapName) > 15 {
		tapName = tapName[:15] // IFNAMSIZ limit
	}
	ipAddr = fmt.Sprintf("192.168.100.%d", ip)

	// Create TAP device
	if err := run(ctx, "ip", "tuntap", "add", "dev", tapName, "mode", "tap"); err != nil {
		return "", "", fmt.Errorf("creating tap %s: %w", tapName, err)
	}

	// Attach to bridge
	if err := run(ctx, "ip", "link", "set", tapName, "master", n.bridge); err != nil {
		run(ctx, "ip", "link", "delete", tapName)
		return "", "", fmt.Errorf("attaching tap to bridge: %w", err)
	}

	// Bring up
	if err := run(ctx, "ip", "link", "set", tapName, "up"); err != nil {
		run(ctx, "ip", "link", "delete", tapName)
		return "", "", fmt.Errorf("bringing up tap: %w", err)
	}

	n.allocs[vmID] = tapName
	return tapName, ipAddr, nil
}

// FreeTap removes the TAP device.
func (n *Network) FreeTap(ctx context.Context, vmID string) error {
	n.mu.Lock()
	tapName, ok := n.allocs[vmID]
	if ok {
		delete(n.allocs, vmID)
	}
	n.mu.Unlock()

	if !ok {
		return nil
	}

	return run(ctx, "ip", "link", "delete", tapName)
}

// SetupBridge creates and configures the network bridge with NAT.
// This is idempotent — safe to call if bridge already exists.
func SetupBridge(ctx context.Context, bridge string) error {
	// Create bridge (ignore error if exists)
	run(ctx, "ip", "link", "add", bridge, "type", "bridge")

	// Set IP
	run(ctx, "ip", "addr", "flush", "dev", bridge)
	if err := run(ctx, "ip", "addr", "add", "192.168.100.1/24", "dev", bridge); err != nil {
		return fmt.Errorf("setting bridge IP: %w", err)
	}

	// Bring up
	if err := run(ctx, "ip", "link", "set", bridge, "up"); err != nil {
		return fmt.Errorf("bringing up bridge: %w", err)
	}

	// Enable IP forwarding
	if err := run(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("enabling ip_forward: %w", err)
	}

	// Add NAT rule (ignore error if already exists)
	run(ctx, "iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "192.168.100.0/24", "!", "-o", bridge, "-j", "MASQUERADE")

	return nil
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s: %w", name, args, string(out), err)
	}
	return nil
}
